package tasks

import "time"

type Task struct {
	ID        int64     `json:"id"`
	Title     string    `json:"title"`
	Done      bool      `json:"done"`
	CreatedAt time.Time `json:"created_at"`
}
type CreateTask struct {
	Title string `json:"title"`
}

type UpdateTask struct {
	Done *bool `json:"done"`
}
