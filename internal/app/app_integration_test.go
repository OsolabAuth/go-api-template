package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"example.com/go-api-template/internal/resources/tasks"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestTaskAPI(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set TEST_DATABASE_URL to run the PostgreSQL integration test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	admin, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Close(context.Background()) })

	// Each run owns a schema; existing schemas and tables are never modified.
	schema := pgx.Identifier{fmt.Sprintf("test_tasks_%d_%d", os.Getpid(), time.Now().UnixNano())}.Sanitize()
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
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	// Cleanups run in reverse order, closing the pool before dropping its schema.
	t.Cleanup(pool.Close)
	migration, err := os.ReadFile("../../migrations/000001_create_tasks.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, string(migration)); err != nil {
		t.Fatal(err)
	}

	router := Router(pool, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	request := func(method, path, body string, wantStatus int, result any) {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body)).WithContext(ctx)
		req.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		if response.Code != wantStatus {
			t.Fatalf("%s %s: status = %d, want %d; body = %s", method, path, response.Code, wantStatus, response.Body.String())
		}
		if result != nil {
			if err := json.Unmarshal(response.Body.Bytes(), result); err != nil {
				t.Fatalf("%s %s: decode response: %v", method, path, err)
			}
		}
	}

	var list []tasks.Task
	request(http.MethodGet, "/api/v1/tasks/", "", http.StatusOK, &list)
	if len(list) != 0 {
		t.Fatalf("initial list = %+v, want empty", list)
	}

	title := strings.Repeat("あ", 200)
	body, err := json.Marshal(tasks.CreateTask{Title: " " + title + " "})
	if err != nil {
		t.Fatal(err)
	}
	var created tasks.Task
	request(http.MethodPost, "/api/v1/tasks/", string(body), http.StatusCreated, &created)
	if created.ID <= 0 || created.Title != title || created.Done {
		t.Fatalf("created task = %+v, want a positive ID, trimmed title, and done=false", created)
	}

	path := "/api/v1/tasks/" + strconv.FormatInt(created.ID, 10)
	var fetched tasks.Task
	request(http.MethodGet, path, "", http.StatusOK, &fetched)
	if fetched.ID != created.ID || fetched.Title != title || fetched.Done {
		t.Fatalf("fetched task = %+v, want created task", fetched)
	}

	request(http.MethodGet, "/api/v1/tasks/", "", http.StatusOK, &list)
	if len(list) != 1 || list[0].ID != created.ID || list[0].Title != title || list[0].Done {
		t.Fatalf("list = %+v, want only the created task", list)
	}

	request(http.MethodDelete, path, "", http.StatusNoContent, nil)
	request(http.MethodGet, path, "", http.StatusNotFound, nil)
}
