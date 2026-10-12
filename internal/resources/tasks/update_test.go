package tasks

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestUpdateValidation(t *testing.T) {
	// A closed pool distinguishes valid input from requests rejected before DB access.
	pool, err := pgxpool.New(context.Background(), "postgres://postgres:postgres@127.0.0.1:1/postgres")
	if err != nil {
		t.Fatal(err)
	}
	pool.Close()
	handler := NewHandler(NewRepository(pool), slog.New(slog.NewTextHandler(io.Discard, nil))).Routes()

	for _, tt := range []struct {
		name   string
		id     string
		body   string
		status int
	}{
		{"done true", "1", `{"done":true}`, http.StatusInternalServerError},
		{"done false", "1", `{"done":false}`, http.StatusInternalServerError},
		{"missing done", "1", `{}`, http.StatusBadRequest},
		{"null done", "1", `{"done":null}`, http.StatusBadRequest},
		{"string done", "1", `{"done":"true"}`, http.StatusBadRequest},
		{"number done", "1", `{"done":1}`, http.StatusBadRequest},
		{"unknown field", "1", `{"done":true,"title":"new title"}`, http.StatusBadRequest},
		{"multiple JSON values", "1", `{"done":true} {"done":false}`, http.StatusBadRequest},
		{"empty body", "1", ``, http.StatusBadRequest},
		{"null body", "1", `null`, http.StatusBadRequest},
		{"invalid JSON", "1", `{"done":`, http.StatusBadRequest},
		{"zero id", "0", `{"done":true}`, http.StatusBadRequest},
		{"negative id", "-1", `{"done":true}`, http.StatusBadRequest},
		{"noninteger id", "abc", `{"done":true}`, http.StatusBadRequest},
		{"overflow id", "9223372036854775808", `{"done":true}`, http.StatusBadRequest},
	} {
		t.Run(tt.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPatch, "/"+tt.id, strings.NewReader(tt.body))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != tt.status {
				t.Fatalf("status = %d, want %d; body = %s", response.Code, tt.status, response.Body.String())
			}
		})
	}
}
