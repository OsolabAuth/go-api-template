package tasks

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("task not found")

type Repository struct{ db *pgxpool.Pool }

func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }
func (r *Repository) List(ctx context.Context) ([]Task, error) {
	rows, err := r.db.Query(ctx, `SELECT id, title, done, created_at FROM tasks ORDER BY id LIMIT 100`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]Task, 0)
	for rows.Next() {
		var task Task
		if err := rows.Scan(&task.ID, &task.Title, &task.Done, &task.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, task)
	}
	return result, rows.Err()
}
func (r *Repository) Create(ctx context.Context, title string) (Task, error) {
	var t Task
	err := r.db.QueryRow(ctx, `INSERT INTO tasks (title) VALUES ($1) RETURNING id, title, done, created_at`, title).Scan(&t.ID, &t.Title, &t.Done, &t.CreatedAt)
	return t, err
}
func (r *Repository) Get(ctx context.Context, id int64) (Task, error) {
	var t Task
	err := r.db.QueryRow(ctx, `SELECT id, title, done, created_at FROM tasks WHERE id=$1`, id).Scan(&t.ID, &t.Title, &t.Done, &t.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Task{}, ErrNotFound
	}
	return t, err
}
func (r *Repository) Delete(ctx context.Context, id int64) error {
	command, err := r.db.Exec(ctx, `DELETE FROM tasks WHERE id=$1`, id)
	if err != nil {
		return err
	}
	if command.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
