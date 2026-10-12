package cache

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestOpenRejectsInvalidURL(t *testing.T) {
	client, err := Open(context.Background(), "https://localhost:6379")
	if err == nil || !strings.Contains(err.Error(), "parse redis config") || client != nil {
		t.Fatalf("Open() = %v, %v; want nil client and configuration error", client, err)
	}
}

func TestOpenRespectsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	client, err := Open(ctx, "redis://localhost:6379/0")
	if !errors.Is(err, context.Canceled) || client != nil {
		t.Fatalf("Open() = %v, %v; want nil client and context.Canceled", client, err)
	}
}
