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

	"example.com/go-api-template/internal/resources/notes"
	"example.com/go-api-template/internal/resources/tasks"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestNoteAPI(t *testing.T) {
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

	// Only this test's schema is created and removed; existing data is untouched.
	schema := pgx.Identifier{fmt.Sprintf("test_notes_%d_%d", os.Getpid(), time.Now().UnixNano())}.Sanitize()
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
	t.Cleanup(pool.Close)
	migrate := func(filename string) {
		t.Helper()
		migration, err := os.ReadFile("../../migrations/" + filename)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, string(migration)); err != nil {
			t.Fatalf("apply %s: %v", filename, err)
		}
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

	// Add notes to a database that already contains tasks and data.
	migrate("000001_create_tasks.up.sql")
	var task tasks.Task
	request(http.MethodPost, "/api/v1/tasks", `{"title":"既存のタスク"}`, http.StatusCreated, &task)
	migrate("000002_create_notes.up.sql")

	var list []notes.Note
	request(http.MethodGet, "/api/v1/notes", "", http.StatusOK, &list)
	if list == nil || len(list) != 0 {
		t.Fatalf("initial list = %#v, want []", list)
	}

	content := strings.Repeat("あ", 2000)
	body, err := json.Marshal(notes.CreateNote{Content: " " + content + " "})
	if err != nil {
		t.Fatal(err)
	}
	var created notes.Note
	request(http.MethodPost, "/api/v1/notes", string(body), http.StatusCreated, &created)
	if created.ID <= 0 || created.Content != content || created.CreatedAt.IsZero() {
		t.Fatalf("created note = %+v, want trimmed content with ID and timestamp", created)
	}
	var second notes.Note
	request(http.MethodPost, "/api/v1/notes", `{"content":"次のメモ"}`, http.StatusCreated, &second)

	path := "/api/v1/notes/" + strconv.FormatInt(created.ID, 10)
	var fetched notes.Note
	request(http.MethodGet, path, "", http.StatusOK, &fetched)
	if fetched.ID != created.ID || fetched.Content != content || !fetched.CreatedAt.Equal(created.CreatedAt) {
		t.Fatalf("fetched note = %+v, want created note", fetched)
	}
	request(http.MethodGet, "/api/v1/notes", "", http.StatusOK, &list)
	if len(list) != 2 || list[0].ID != created.ID || list[1].ID != second.ID || list[0].ID >= list[1].ID {
		t.Fatalf("list = %+v, want both notes in ID order", list)
	}

	request(http.MethodDelete, path, "", http.StatusNoContent, nil)
	request(http.MethodGet, path, "", http.StatusNotFound, nil)
	request(http.MethodDelete, path, "", http.StatusNotFound, nil)
	request(http.MethodGet, "/api/v1/notes", "", http.StatusOK, &list)
	if len(list) != 1 || list[0].ID != second.ID {
		t.Fatalf("list after deletion = %+v, want second note only", list)
	}

	// Reverting the new migration removes notes only; existing tasks survive.
	migrate("000002_create_notes.down.sql")
	var notesRemoved bool
	if err := pool.QueryRow(ctx, "SELECT to_regclass('notes') IS NULL").Scan(&notesRemoved); err != nil || !notesRemoved {
		t.Fatalf("notes table removed = %t, error = %v", notesRemoved, err)
	}
	var preserved tasks.Task
	request(http.MethodGet, "/api/v1/tasks/"+strconv.FormatInt(task.ID, 10), "", http.StatusOK, &preserved)
	if preserved.ID != task.ID || preserved.Title != task.Title || !preserved.CreatedAt.Equal(task.CreatedAt) {
		t.Fatalf("task after notes rollback = %+v, want original task", preserved)
	}
	migrate("000002_create_notes.up.sql")
	request(http.MethodGet, "/api/v1/notes", "", http.StatusOK, &list)
	if list == nil || len(list) != 0 {
		t.Fatalf("list after reapplying migration = %#v, want []", list)
	}
}
