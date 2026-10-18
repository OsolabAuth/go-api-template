package tasks

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/redis/go-redis/v9"
)

const (
	// Use a distinct prefix for each service/environment sharing a Redis DB.
	taskCachePrefix  = "go-api-template:tasks:v1:"
	taskCacheTTL     = 10 * time.Second
	taskCacheTimeout = 100 * time.Millisecond
)

func taskCacheKey(id int64) string { return taskCachePrefix + strconv.FormatInt(id, 10) }

func (h *Handler) cachedTask(ctx context.Context, id int64) (Task, bool) {
	if h.redis == nil {
		return Task{}, false
	}
	cacheCtx, cancel := context.WithTimeout(ctx, taskCacheTimeout)
	defer cancel()
	data, err := h.redis.Get(cacheCtx, taskCacheKey(id)).Bytes()
	if err != nil {
		if !errors.Is(err, redis.Nil) {
			h.logger.WarnContext(ctx, "task cache read failed", "task_id", id, "error", err)
		}
		return Task{}, false
	}
	task, ok := decodeCachedTask(data, id)
	if !ok {
		h.logger.WarnContext(ctx, "invalid cached task", "task_id", id)
	}
	return task, ok
}

func decodeCachedTask(data []byte, id int64) (Task, bool) {
	// A missing done field must not silently become false.
	var value struct {
		ID        int64     `json:"id"`
		Title     string    `json:"title"`
		Done      *bool     `json:"done"`
		CreatedAt time.Time `json:"created_at"`
	}
	if err := json.Unmarshal(data, &value); err != nil || value.ID != id || value.Done == nil || value.CreatedAt.IsZero() {
		return Task{}, false
	}
	if length := utf8.RuneCountInString(value.Title); length == 0 || length > 200 || strings.TrimSpace(value.Title) != value.Title {
		return Task{}, false
	}
	return Task{ID: value.ID, Title: value.Title, Done: *value.Done, CreatedAt: value.CreatedAt}, true
}

func (h *Handler) cacheTask(ctx context.Context, task Task) {
	if h.redis == nil {
		return
	}
	data, err := json.Marshal(task)
	if err != nil {
		h.logger.WarnContext(ctx, "task cache encode failed", "task_id", task.ID, "error", err)
		return
	}
	cacheCtx, cancel := context.WithTimeout(ctx, taskCacheTimeout)
	defer cancel()
	if err := h.redis.Set(cacheCtx, taskCacheKey(task.ID), data, taskCacheTTL).Err(); err != nil {
		h.logger.WarnContext(ctx, "task cache write failed", "task_id", task.ID, "error", err)
	}
}

func (h *Handler) invalidateTask(ctx context.Context, id int64) {
	if h.redis == nil {
		return
	}
	cacheCtx, cancel := context.WithTimeout(ctx, taskCacheTimeout)
	defer cancel()
	if err := h.redis.Del(cacheCtx, taskCacheKey(id)).Err(); err != nil {
		h.logger.WarnContext(ctx, "task cache delete failed", "task_id", id, "error", err)
	}
}
