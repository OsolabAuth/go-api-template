package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"
	"time"

	"example.com/go-api-template/internal/cache"
	"example.com/go-api-template/internal/resources/tasks"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

func TestTaskCacheAPI(t *testing.T) {
	databaseURL, redisURL := os.Getenv("TEST_DATABASE_URL"), os.Getenv("TEST_REDIS_URL")
	if databaseURL == "" || redisURL == "" {
		t.Skip("set TEST_DATABASE_URL and TEST_REDIS_URL to run the task cache integration test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	admin, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Close(context.Background()) })
	schema := pgx.Identifier{fmt.Sprintf("test_task_cache_%d_%d", os.Getpid(), time.Now().UnixNano())}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		if _, err := admin.Exec(cleanupCtx, "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Errorf("remove test schema: %v", err)
		}
	})
	options, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	options.ConnConfig.RuntimeParams["search_path"] = schema
	db, err := pgxpool.NewWithConfig(ctx, options)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	migration, err := os.ReadFile("../../migrations/000001_create_tasks.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, string(migration)); err != nil {
		t.Fatal(err)
	}
	client, err := cache.Open(ctx, redisURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	var keys []string
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		if len(keys) > 0 {
			if err := client.Del(cleanupCtx, keys...).Err(); err != nil {
				t.Errorf("remove test cache keys: %v", err)
			}
		}
	})
	key := func(id int64) string { return "go-api-template:tasks:v1:" + strconv.FormatInt(id, 10) }
	// High, unique IDs isolate keys from other schemas and existing API data.
	nextID := time.Now().UnixNano()
	create := func(t *testing.T) tasks.Task {
		t.Helper()
		nextID++
		var task tasks.Task
		err := db.QueryRow(ctx, `INSERT INTO tasks (id, title) OVERRIDING SYSTEM VALUE VALUES ($1, 'cache example') RETURNING id, title, done, created_at`, nextID).Scan(&task.ID, &task.Title, &task.Done, &task.CreatedAt)
		if err != nil {
			t.Fatal(err)
		}
		keys = append(keys, key(task.ID))
		return task
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	router := Router(db, client, logger)
	request := func(t *testing.T, handler http.Handler, method string, id int64, status int) tasks.Task {
		t.Helper()
		requestCtx, requestCancel := context.WithTimeout(ctx, 2*time.Second)
		defer requestCancel()
		req := httptest.NewRequest(method, "/api/v1/tasks/"+strconv.FormatInt(id, 10), nil).WithContext(requestCtx)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, req)
		if response.Code != status {
			t.Fatalf("%s: status = %d, want %d; body = %s", method, response.Code, status, response.Body.String())
		}
		if requestCtx.Err() != nil {
			t.Fatalf("cache operations consumed the request deadline: %v", requestCtx.Err())
		}
		var task tasks.Task
		if status == http.StatusOK {
			if err := json.Unmarshal(response.Body.Bytes(), &task); err != nil {
				t.Fatal(err)
			}
			if task.ID != id || task.Title != "cache example" || task.Done || task.CreatedAt.IsZero() {
				t.Fatalf("unexpected task response: %+v", task)
			}
		}
		return task
	}

	t.Run("miss populates cache with TTL and hit avoids DB", func(t *testing.T) {
		task := create(t)
		request(t, router, http.MethodGet, task.ID, http.StatusOK)
		data, err := client.Get(ctx, key(task.ID)).Bytes()
		if err != nil {
			t.Fatal(err)
		}
		var cached tasks.Task
		if err := json.Unmarshal(data, &cached); err != nil || cached.ID != task.ID || cached.Title != task.Title {
			t.Fatalf("cached task = %+v, %v", cached, err)
		}
		if ttl, err := client.PTTL(ctx, key(task.ID)).Result(); err != nil || ttl <= 0 || ttl > 10*time.Second {
			t.Fatalf("cache TTL = %v, %v; want 0 < TTL <= 10s", ttl, err)
		}
		closedDB, err := pgxpool.NewWithConfig(ctx, options.Copy())
		if err != nil {
			t.Fatal(err)
		}
		closedDB.Close()
		request(t, Router(closedDB, client, logger), http.MethodGet, task.ID, http.StatusOK)
	})

	t.Run("corrupt or expired cache is a miss", func(t *testing.T) {
		task := create(t)
		if err := client.Set(ctx, key(task.ID), "{invalid JSON", time.Minute).Err(); err != nil {
			t.Fatal(err)
		}
		request(t, router, http.MethodGet, task.ID, http.StatusOK)
		if data, err := client.Get(ctx, key(task.ID)).Bytes(); err != nil || !json.Valid(data) {
			t.Fatalf("invalid cache was not replaced: %q, %v", data, err)
		}
		// Ask Redis to expire this key immediately instead of slowing the test by 10s.
		if err := client.PExpire(ctx, key(task.ID), -time.Second).Err(); err != nil {
			t.Fatal(err)
		}
		request(t, router, http.MethodGet, task.ID, http.StatusOK)
	})

	t.Run("delete invalidates cache and missing records are not cached", func(t *testing.T) {
		task := create(t)
		request(t, router, http.MethodGet, task.ID, http.StatusOK)
		request(t, router, http.MethodDelete, task.ID, http.StatusNoContent)
		if count, err := client.Exists(ctx, key(task.ID)).Result(); err != nil || count != 0 {
			t.Fatalf("cache remains after delete: count=%d, err=%v", count, err)
		}
		request(t, router, http.MethodGet, task.ID, http.StatusNotFound)
		if count, err := client.Exists(ctx, key(task.ID)).Result(); err != nil || count != 0 {
			t.Fatalf("missing task was cached: count=%d, err=%v", count, err)
		}
		// A retry also clears any stale value for a task already deleted from DB.
		if err := client.Set(ctx, key(task.ID), "stale", time.Minute).Err(); err != nil {
			t.Fatal(err)
		}
		request(t, router, http.MethodDelete, task.ID, http.StatusNotFound)
		if count, err := client.Exists(ctx, key(task.ID)).Result(); err != nil || count != 0 {
			t.Fatalf("retry did not invalidate cache: count=%d, err=%v", count, err)
		}
	})

	t.Run("Redis failure preserves DB reads and deletes", func(t *testing.T) {
		task := create(t)
		request(t, router, http.MethodGet, task.ID, http.StatusOK)
		closedClient := redis.NewClient(client.Options())
		if err := closedClient.Close(); err != nil {
			t.Fatal(err)
		}
		fallbackRouter := Router(db, closedClient, logger)
		request(t, fallbackRouter, http.MethodGet, task.ID, http.StatusOK)
		request(t, fallbackRouter, http.MethodDelete, task.ID, http.StatusNoContent)
		request(t, fallbackRouter, http.MethodGet, task.ID, http.StatusNotFound)
		// Invalidation can fail during an outage; stale data survives until its TTL.
		if count, err := client.Exists(ctx, key(task.ID)).Result(); err != nil || count != 1 {
			t.Fatalf("expected old cache after failed invalidation: count=%d, err=%v", count, err)
		}
		if err := client.PExpire(ctx, key(task.ID), -time.Second).Err(); err != nil {
			t.Fatal(err)
		}
		request(t, router, http.MethodGet, task.ID, http.StatusNotFound)
	})

	t.Run("slow Redis uses its own short deadline", func(t *testing.T) {
		task := create(t)
		slowClient := redis.NewClient(&redis.Options{
			ContextTimeoutEnabled: true,
			MaxRetries:            -1,
			Dialer: func(ctx context.Context, _, _ string) (net.Conn, error) {
				<-ctx.Done()
				return nil, ctx.Err()
			},
		})
		t.Cleanup(func() { _ = slowClient.Close() })
		request(t, Router(db, slowClient, logger), http.MethodGet, task.ID, http.StatusOK)
	})
}
