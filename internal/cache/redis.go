package cache

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
)

func Open(ctx context.Context, redisURL string) (*redis.Client, error) {
	options, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("parse redis config: %w", err)
	}
	// Honor startup and readiness deadlines for socket reads and writes as well.
	options.ContextTimeoutEnabled = true
	client := redis.NewClient(options)
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("ping redis: %w", err)
	}
	return client, nil
}
