package app

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestRequestIDHeaderMatchesLogAndError(t *testing.T) {
	generated := make(map[string]bool)
	for _, tt := range []struct {
		name      string
		path      string
		requestID string
		status    int
	}{
		{"generated ID", "/health", "", http.StatusOK},
		{"provided ID", "/health", "client-request-123", http.StatusOK},
		{"not found", "/missing", "", http.StatusNotFound},
		{"invalid input", "/api/v1/tasks/0", "", http.StatusBadRequest},
		{"recovered panic", "/panic", "", http.StatusInternalServerError},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var logs bytes.Buffer
			router := Router(nil, slog.New(slog.NewJSONHandler(&logs, nil))).(*chi.Mux)
			router.Get("/panic", func(http.ResponseWriter, *http.Request) { panic("test panic") })
			request := httptest.NewRequest(http.MethodGet, tt.path, nil)
			if tt.requestID != "" {
				request.Header.Set("X-Request-ID", tt.requestID)
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != tt.status {
				t.Fatalf("status = %d, want %d", response.Code, tt.status)
			}
			requestID := response.Header().Get("X-Request-ID")
			if requestID == "" {
				t.Fatal("missing X-Request-ID response header")
			}
			if tt.requestID != "" && requestID != tt.requestID {
				t.Fatalf("request ID = %q, want %q", requestID, tt.requestID)
			}
			if tt.requestID == "" {
				if generated[requestID] {
					t.Fatalf("generated request ID reused: %q", requestID)
				}
				generated[requestID] = true
			}
			var logged struct {
				RequestID string `json:"request_id"`
			}
			if err := json.Unmarshal(logs.Bytes(), &logged); err != nil {
				t.Fatal(err)
			}
			if logged.RequestID != requestID {
				t.Fatalf("log request ID = %q, header = %q", logged.RequestID, requestID)
			}
			if tt.status == http.StatusBadRequest {
				var failure struct {
					RequestID string `json:"request_id"`
				}
				if err := json.Unmarshal(response.Body.Bytes(), &failure); err != nil {
					t.Fatal(err)
				}
				if failure.RequestID != requestID {
					t.Fatalf("error request ID = %q, header = %q", failure.RequestID, requestID)
				}
			}
		})
	}
}
