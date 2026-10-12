package notes

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestHandlerValidation(t *testing.T) {
	// A closed pool distinguishes valid input from requests rejected before DB access.
	pool, err := pgxpool.New(context.Background(), "postgres://postgres:postgres@127.0.0.1:1/postgres")
	if err != nil {
		t.Fatal(err)
	}
	pool.Close()

	for _, tt := range []struct {
		name   string
		method string
		path   string
		body   string
		status int
	}{
		{"one character", http.MethodPost, "/", `{"content":"a"}`, http.StatusInternalServerError},
		{"2000 Japanese characters", http.MethodPost, "/", `{"content":" ` + strings.Repeat("あ", 2000) + ` "}`, http.StatusInternalServerError},
		{"2001 Japanese characters", http.MethodPost, "/", `{"content":"` + strings.Repeat("あ", 2001) + `"}`, http.StatusBadRequest},
		{"blank content", http.MethodPost, "/", `{"content":" \t "}`, http.StatusBadRequest},
		{"missing content", http.MethodPost, "/", `{}`, http.StatusBadRequest},
		{"null content", http.MethodPost, "/", `{"content":null}`, http.StatusBadRequest},
		{"wrong content type", http.MethodPost, "/", `{"content":true}`, http.StatusBadRequest},
		{"unknown field", http.MethodPost, "/", `{"content":"note","title":"unknown"}`, http.StatusBadRequest},
		{"multiple JSON values", http.MethodPost, "/", `{"content":"note"} {}`, http.StatusBadRequest},
		{"invalid JSON", http.MethodPost, "/", `{"content":`, http.StatusBadRequest},
		{"empty body", http.MethodPost, "/", ``, http.StatusBadRequest},
		{"list DB error", http.MethodGet, "/", ``, http.StatusInternalServerError},
		{"get DB error", http.MethodGet, "/1", ``, http.StatusInternalServerError},
		{"delete DB error", http.MethodDelete, "/1", ``, http.StatusInternalServerError},
		{"zero id", http.MethodGet, "/0", ``, http.StatusBadRequest},
		{"negative id", http.MethodDelete, "/-1", ``, http.StatusBadRequest},
		{"noninteger id", http.MethodGet, "/abc", ``, http.StatusBadRequest},
		{"overflow id", http.MethodDelete, "/9223372036854775808", ``, http.StatusBadRequest},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var logs bytes.Buffer
			handler := NewHandler(NewRepository(pool), slog.New(slog.NewJSONHandler(&logs, nil)))
			request := httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("X-Request-Id", "notes-test")
			response := httptest.NewRecorder()
			middleware.RequestID(handler.Routes()).ServeHTTP(response, request)
			if response.Code != tt.status {
				t.Fatalf("status = %d, want %d; body = %s", response.Code, tt.status, response.Body.String())
			}
			if tt.status == http.StatusInternalServerError {
				var entry struct {
					Level     string `json:"level"`
					Error     string `json:"error"`
					RequestID string `json:"request_id"`
				}
				if err := json.Unmarshal(logs.Bytes(), &entry); err != nil {
					t.Fatalf("decode error log: %v", err)
				}
				if entry.Level != "ERROR" || entry.Error == "" || entry.RequestID != "notes-test" {
					t.Fatalf("missing error details or request ID: %+v", entry)
				}
				if strings.Contains(response.Body.String(), entry.Error) {
					t.Fatalf("internal error leaked to client: %s", response.Body.String())
				}
			}
		})
	}
}
