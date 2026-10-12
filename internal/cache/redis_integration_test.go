package cache

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestOpenRedis(t *testing.T) {
	redisURL := os.Getenv("TEST_REDIS_URL")
	if redisURL == "" {
		t.Skip("set TEST_REDIS_URL to run the Redis integration test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client, err := Open(ctx, redisURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if pong, err := client.Ping(ctx).Result(); err != nil || pong != "PONG" {
		t.Fatalf("Ping() = %q, %v; want PONG", pong, err)
	}
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
	if err := client.Ping(ctx).Err(); err == nil {
		t.Fatal("closed Redis client should reject commands")
	}
}
