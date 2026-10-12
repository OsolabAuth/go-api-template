package common

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5/middleware"
)

func TestJSON(t *testing.T) {
	w := httptest.NewRecorder()
	JSON(w, http.StatusCreated, map[string]string{"title": "日本語のタスク"})

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusCreated)
	}
	if got := w.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Fatalf("Content-Type = %q", got)
	}
	var body map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["title"] != "日本語のタスク" {
		t.Fatalf("response = %v", body)
	}
}

func TestError(t *testing.T) {
	for _, requestID := range []string{"", "test-request-id"} {
		name := "without request ID"
		if requestID != "" {
			name = "with request ID"
		}
		t.Run(name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			var handler http.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				Error(w, r, http.StatusBadRequest, "invalid title")
			})
			if requestID != "" {
				r.Header.Set(middleware.RequestIDHeader, requestID)
				handler = middleware.RequestID(handler)
			}
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)

			if w.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
			}
			var body map[string]string
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if body["error"] != "invalid title" {
				t.Fatalf("error = %q", body["error"])
			}
			got, present := body["request_id"]
			if got != requestID || present != (requestID != "") {
				t.Fatalf("request_id = %q, present = %t; want %q", got, present, requestID)
			}
		})
	}
}

func TestDecodeJSON(t *testing.T) {
	const maxBodyBytes = 1 << 20
	maxTitle := strings.Repeat("a", maxBodyBytes-len(`{"title":""}`))
	for _, tc := range []struct {
		name      string
		body      string
		wantTitle string
		wantError bool
	}{
		{name: "valid", body: `{"title":"日本語"}`, wantTitle: "日本語"},
		{name: "trailing whitespace", body: "{\"title\":\"task\"}\n \t", wantTitle: "task"},
		{name: "empty", body: "", wantError: true},
		{name: "malformed", body: `{"title":`, wantError: true},
		{name: "wrong field type", body: `{"title":1}`, wantError: true},
		{name: "unknown field", body: `{"title":"task","extra":true}`, wantError: true},
		{name: "multiple values", body: `{"title":"first"} {"title":"second"}`, wantError: true},
		{name: "trailing garbage", body: `{"title":"task"} invalid`, wantError: true},
		{name: "size limit", body: `{"title":"` + maxTitle + `"}`, wantTitle: maxTitle},
		{name: "over size limit", body: `{"title":"` + maxTitle + `a"}`, wantError: true},
		{name: "oversized trailing whitespace", body: `{"title":"task"}` + strings.Repeat(" ", maxBodyBytes), wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := &trackedBody{Reader: strings.NewReader(tc.body)}
			r := httptest.NewRequest(http.MethodPost, "/", body)
			var value struct {
				Title string `json:"title"`
			}
			err := DecodeJSON(r, &value)
			if (err != nil) != tc.wantError {
				t.Fatalf("DecodeJSON error = %v, want error = %t", err, tc.wantError)
			}
			if !tc.wantError && value.Title != tc.wantTitle {
				t.Fatalf("decoded title differs: got %d bytes, want %d bytes", len(value.Title), len(tc.wantTitle))
			}
			if !body.closed {
				t.Fatal("request body was not closed")
			}
		})
	}
}

type trackedBody struct {
	io.Reader
	closed bool
}

func (b *trackedBody) Close() error {
	b.closed = true
	return nil
}
