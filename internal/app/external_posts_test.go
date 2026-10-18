package app

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"example.com/go-api-template/internal/common"
	"example.com/go-api-template/internal/integrations/posts"
)

func TestExternalPostAPI(t *testing.T) {
	const validPost = `{"id":1,"userId":2,"title":"Example","body":"Post body","newUpstreamField":true}`
	for _, tt := range []struct {
		name       string
		id         string
		upstream   int
		body       string
		mode       string
		wantStatus int
		wantError  string
	}{
		{"success", "1", 200, validPost, "", 200, ""},
		{"not found", "1", 404, "private upstream body", "", 404, "external post not found"},
		{"upstream error", "1", 503, "private upstream body", "", 502, "external API unavailable"},
		{"redirect", "1", 302, "", "redirect", 502, "external API unavailable"},
		{"invalid JSON", "1", 200, "not JSON", "", 502, "external API unavailable"},
		{"trailing JSON", "1", 200, validPost + `{}`, "", 502, "external API unavailable"},
		{"null", "1", 200, "null", "", 502, "external API unavailable"},
		{"wrong id", "2", 200, validPost, "", 502, "external API unavailable"},
		{"missing fields", "1", 200, `{"id":1}`, "", 502, "external API unavailable"},
		{"empty response", "1", 204, "", "", 502, "external API unavailable"},
		{"header timeout", "1", 200, "", "header-timeout", 504, "external API timed out"},
		{"body timeout", "1", 200, "", "body-timeout", 504, "external API timed out"},
		{"connection failure", "1", 200, "", "closed", 502, "external API unavailable"},
		{"zero id", "0", 200, "", "", 400, "invalid external post id"},
		{"negative id", "-1", 200, "", "", 400, "invalid external post id"},
		{"non-number id", "hello", 200, "", "", 400, "invalid external post id"},
		{"overflow id", "9223372036854775808", 200, "", "", 400, "invalid external post id"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.Method != http.MethodGet || r.URL.Path != "/posts/"+tt.id || r.URL.RawQuery != "" || r.Header.Get("Accept") != "application/json" {
					t.Errorf("unexpected upstream request: %s %s; headers=%v", r.Method, r.URL, r.Header)
				}
				if tt.mode == "header-timeout" {
					<-r.Context().Done()
					return
				}
				if tt.mode == "redirect" {
					w.Header().Set("Location", "/redirected")
				}
				w.WriteHeader(tt.upstream)
				if tt.mode == "body-timeout" {
					w.(http.Flusher).Flush()
					<-r.Context().Done()
					return
				}
				_, _ = io.WriteString(w, tt.body)
			}))
			defer upstream.Close()
			if tt.mode == "closed" {
				upstream.Close()
			}
			timeout := time.Second
			if tt.mode == "header-timeout" || tt.mode == "body-timeout" {
				timeout = 50 * time.Millisecond
			}
			client, err := posts.NewClient(upstream.URL, timeout)
			if err != nil {
				t.Fatal(err)
			}
			defer client.CloseIdleConnections()
			router := Router(nil, client, slog.New(slog.NewTextHandler(io.Discard, nil)))
			req := httptest.NewRequest(http.MethodGet, "/api/v1/external/posts/"+tt.id+"?url=https://ignored.example", nil)
			req.Header.Set("X-Request-Id", "external-post-test")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, req)
			if response.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d: %s", response.Code, tt.wantStatus, response.Body.String())
			}
			if tt.wantStatus == http.StatusOK {
				var got posts.Post
				if err := json.Unmarshal(response.Body.Bytes(), &got); err != nil {
					t.Fatal(err)
				}
				if got != (posts.Post{ID: 1, UserID: 2, Title: "Example", Body: "Post body"}) {
					t.Fatalf("response = %+v", got)
				}
			} else {
				var got common.APIError
				if err := json.Unmarshal(response.Body.Bytes(), &got); err != nil {
					t.Fatal(err)
				}
				if got.Error != tt.wantError || got.RequestID != "external-post-test" {
					t.Fatalf("error response = %+v", got)
				}
			}
			wantCalls := int32(1)
			if tt.wantStatus == http.StatusBadRequest || tt.mode == "closed" {
				wantCalls = 0
			}
			if calls.Load() != wantCalls {
				t.Fatalf("upstream calls = %d, want %d", calls.Load(), wantCalls)
			}
		})
	}
}
