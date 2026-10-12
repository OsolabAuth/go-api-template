package app

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"example.com/go-api-template/internal/cache"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

func TestRedisReadiness(t *testing.T) {
	databaseURL, redisURL := os.Getenv("TEST_DATABASE_URL"), os.Getenv("TEST_REDIS_URL")
	if databaseURL == "" || redisURL == "" {
		t.Skip("set TEST_DATABASE_URL and TEST_REDIS_URL to run the readiness integration test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	db, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	client, err := cache.Open(ctx, redisURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	check := func(redisClient *redis.Client, path string, status int, message string) {
		t.Helper()
		response := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, path, nil).WithContext(ctx)
		Router(db, redisClient, logger).ServeHTTP(response, request)
		if response.Code != status || !strings.Contains(response.Body.String(), message) {
			t.Fatalf("%s = %d %s; want %d containing %q", path, response.Code, response.Body.String(), status, message)
		}
	}

	check(client, "/ready", http.StatusOK, "ready")
	check(nil, "/ready", http.StatusServiceUnavailable, "redis unavailable")
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
	check(client, "/ready", http.StatusServiceUnavailable, "redis unavailable")
	check(client, "/health", http.StatusOK, "ok")
	db.Close()
	check(client, "/ready", http.StatusServiceUnavailable, "database unavailable")
	check(client, "/health", http.StatusOK, "ok")
}
