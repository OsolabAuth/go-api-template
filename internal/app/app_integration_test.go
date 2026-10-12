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

	router := Router(pool, slog.New(slog.NewTextHandler(io.Discard, nil)))
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

	var seeded []tasks.Task
	for i := 0; i < 5; i++ {
		var task tasks.Task
		request(http.MethodPost, "/api/v1/tasks/", fmt.Sprintf(`{"title":"Page task %d"}`, i), http.StatusCreated, &task)
		seeded = append(seeded, task)
	}

	// Each page starts strictly after the last ID of the previous page.
	var afterID int64
	for offset := 0; offset < len(seeded); offset += 2 {
		pagePath := fmt.Sprintf("/api/v1/tasks/?limit=2&after_id=%d", afterID)
		request(http.MethodGet, pagePath, "", http.StatusOK, &list)
		wantCount := min(2, len(seeded)-offset)
		if len(list) != wantCount {
			t.Fatalf("page at offset %d = %+v, want %d tasks", offset, list, wantCount)
		}
		for i, task := range list {
			if task.ID != seeded[offset+i].ID {
				t.Fatalf("page at offset %d: ID = %d, want %d", offset, task.ID, seeded[offset+i].ID)
			}
		}
		afterID = list[len(list)-1].ID
	}
	request(http.MethodGet, fmt.Sprintf("/api/v1/tasks/?limit=2&after_id=%d", afterID), "", http.StatusOK, &list)
	if list == nil || len(list) != 0 {
		t.Fatalf("final page = %+v, want an empty JSON array", list)
	}

	// A cursor remains usable even when the task with that ID is deleted.
	cursorID := seeded[1].ID
	request(http.MethodDelete, fmt.Sprintf("/api/v1/tasks/%d", cursorID), "", http.StatusNoContent, nil)
	request(http.MethodGet, fmt.Sprintf("/api/v1/tasks/?limit=2&after_id=%d", cursorID), "", http.StatusOK, &list)
	if len(list) != 2 || list[0].ID != seeded[2].ID || list[1].ID != seeded[3].ID {
		t.Fatalf("page after deleted cursor = %+v, want the next two tasks", list)
	}

	request(http.MethodGet, "/api/v1/tasks/?limit=1&after_id=0", "", http.StatusOK, &list)
	if len(list) != 1 || list[0].ID != seeded[0].ID {
		t.Fatalf("first page with limit=1 = %+v, want the first task", list)
	}
	request(http.MethodGet, "/api/v1/tasks/?limit=100&after_id=9223372036854775807", "", http.StatusOK, &list)
	if list == nil || len(list) != 0 {
		t.Fatalf("page after maximum ID = %+v, want an empty JSON array", list)
	}
}
