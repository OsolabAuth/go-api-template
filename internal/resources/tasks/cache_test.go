package tasks

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestDecodeCachedTask(t *testing.T) {
	valid := Task{ID: 42, Title: "sample", Done: false, CreatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	data, err := json.Marshal(valid)
	if err != nil {
		t.Fatal(err)
	}
	if task, ok := decodeCachedTask(data, valid.ID); !ok || task != valid {
		t.Fatalf("valid cached task = %+v, %v; want %+v", task, ok, valid)
	}
	for _, tt := range []struct {
		name string
		data string
	}{
		{"malformed JSON", "{"},
		{"null", "null"},
		{"missing fields", `{}`},
		{"wrong ID", strings.Replace(string(data), `"id":42`, `"id":43`, 1)},
		{"missing done", strings.Replace(string(data), `"done":false,`, "", 1)},
		{"invalid done", strings.Replace(string(data), `"done":false`, `"done":"false"`, 1)},
		{"empty title", strings.Replace(string(data), `"title":"sample"`, `"title":""`, 1)},
		{"blank title", strings.Replace(string(data), `"title":"sample"`, `"title":" "`, 1)},
		{"long title", strings.Replace(string(data), "sample", strings.Repeat("あ", 201), 1)},
		{"invalid time", strings.Replace(string(data), "2026-01-01T00:00:00Z", "yesterday", 1)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if task, ok := decodeCachedTask([]byte(tt.data), valid.ID); ok {
				t.Fatalf("invalid cache accepted: %+v", task)
			}
		})
	}
}
