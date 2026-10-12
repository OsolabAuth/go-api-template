package notes

import "time"

type Note struct {
	ID        int64     `json:"id"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}
type CreateNote struct {
	Content string `json:"content"`
}
