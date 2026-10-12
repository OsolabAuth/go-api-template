package notes

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("note not found")

type Repository struct{ db *pgxpool.Pool }

func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }
func (r *Repository) List(ctx context.Context) ([]Note, error) {
	rows, err := r.db.Query(ctx, `SELECT id, content, created_at FROM notes ORDER BY id LIMIT 100`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]Note, 0)
	for rows.Next() {
		var note Note
		if err := rows.Scan(&note.ID, &note.Content, &note.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, note)
	}
	return result, rows.Err()
}
func (r *Repository) Create(ctx context.Context, content string) (Note, error) {
	var note Note
	err := r.db.QueryRow(ctx, `INSERT INTO notes (content) VALUES ($1) RETURNING id, content, created_at`, content).Scan(&note.ID, &note.Content, &note.CreatedAt)
	return note, err
}
func (r *Repository) Get(ctx context.Context, id int64) (Note, error) {
	var note Note
	err := r.db.QueryRow(ctx, `SELECT id, content, created_at FROM notes WHERE id=$1`, id).Scan(&note.ID, &note.Content, &note.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Note{}, ErrNotFound
	}
	return note, err
}
func (r *Repository) Delete(ctx context.Context, id int64) error {
	command, err := r.db.Exec(ctx, `DELETE FROM notes WHERE id=$1`, id)
	if err != nil {
		return err
	}
	if command.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
