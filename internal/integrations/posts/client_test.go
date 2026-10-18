package posts

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestNewClientValidatesOrigin(t *testing.T) {
	for _, origin := range []string{"", "file:///tmp/posts", "https:///", "https://example.com/api", "https://user:secret@example.com", "https://example.com?token=secret", "https://example.com?", "https://example.com#fragment", "%"} {
		t.Run(origin, func(t *testing.T) {
			if _, err := NewClient(origin, time.Second); err == nil {
				t.Fatalf("accepted invalid origin %q", origin)
			}
		})
	}
	for _, origin := range []string{"https://example.com", "http://localhost:9000/"} {
		client, err := NewClient(origin, time.Second)
		if err != nil {
			t.Fatal(err)
		}
		client.CloseIdleConnections()
	}
	if _, err := NewClient("https://example.com", 0); err == nil {
		t.Fatal("accepted a timeout that disables the deadline")
	}
}

func TestGetBoundsResponseAndHonorsContext(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_, _ = io.WriteString(w, strings.Repeat(" ", maxResponseBytes+1))
	}))
	defer server.Close()
	client, err := NewClient(server.URL+"/", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer client.CloseIdleConnections()
	if _, err := client.Get(context.Background(), 1); err == nil || err.Error() != "external response exceeds 1 MiB" {
		t.Fatalf("oversized response error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := client.Get(ctx, 1); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled request error = %v", err)
	}
	if _, err := client.Get(context.Background(), 0); err == nil {
		t.Fatal("accepted invalid post id")
	}
	if calls.Load() != 1 {
		t.Fatalf("canceled or invalid request reached upstream; calls = %d", calls.Load())
	}
}
