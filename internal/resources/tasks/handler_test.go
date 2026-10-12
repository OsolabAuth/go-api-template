package tasks

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

func TestCreateTitleCharacterLimit(t *testing.T) {
	// A closed pool lets valid requests reach the repository without a running DB.
	pool, err := pgxpool.New(context.Background(), "postgres://postgres:postgres@127.0.0.1:1/postgres")
	if err != nil {
		t.Fatal(err)
	}
	pool.Close()

	for _, tt := range []struct {
		name   string
		title  string
		status int
	}{
		{"blank", " \t ", http.StatusBadRequest},
		{"200 ASCII characters", strings.Repeat("a", 200), http.StatusInternalServerError},
		{"200 Japanese characters", " " + strings.Repeat("あ", 200) + " ", http.StatusInternalServerError},
		{"201 Japanese characters", strings.Repeat("あ", 201), http.StatusBadRequest},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var logs bytes.Buffer
			handler := NewHandler(NewRepository(pool), slog.New(slog.NewJSONHandler(&logs, nil)))
			body, err := json.Marshal(CreateTask{Title: tt.title})
			if err != nil {
				t.Fatal(err)
			}
			request := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
			request.Header.Set("X-Request-Id", "create-test")
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
				if entry.Level != "ERROR" || entry.Error == "" || entry.RequestID != "create-test" {
					t.Fatalf("missing error details or request ID: %+v", entry)
				}
				if strings.Contains(response.Body.String(), entry.Error) {
					t.Fatalf("internal error leaked to client: %s", response.Body.String())
				}
			}
		})
	}
}
