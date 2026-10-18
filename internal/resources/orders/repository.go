package orders

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNotFound     = errors.New("order not found")
	ErrDuplicateSKU = errors.New("an order cannot contain duplicate SKUs")
)

type Repository struct{ db *pgxpool.Pool }

func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

func (r *Repository) Create(ctx context.Context, input CreateOrder) (Order, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return Order{}, fmt.Errorf("begin order: %w", err)
	}
	defer func() {
		// Roll back even when the request was canceled; after Commit this is a no-op.
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = tx.Rollback(cleanupCtx)
	}()

	var order Order
	err = tx.QueryRow(ctx, `INSERT INTO orders (customer_name) VALUES ($1)
		RETURNING id, customer_name, created_at`, input.CustomerName).
		Scan(&order.ID, &order.CustomerName, &order.CreatedAt)
	if err != nil {
		return Order{}, fmt.Errorf("insert order: %w", err)
	}
	for i, item := range input.Items {
		_, err = tx.Exec(ctx, `INSERT INTO order_items (order_id, position, sku, quantity)
			VALUES ($1, $2, $3, $4)`, order.ID, i+1, item.SKU, item.Quantity)
		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "order_items_order_sku_key" {
				return Order{}, ErrDuplicateSKU
			}
			return Order{}, fmt.Errorf("insert order item: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Order{}, fmt.Errorf("commit order: %w", err)
	}
	order.Items = input.Items
	return order, nil
}

func (r *Repository) Get(ctx context.Context, id int64) (Order, error) {
	var order Order
	err := r.db.QueryRow(ctx, `SELECT id, customer_name, created_at FROM orders WHERE id=$1`, id).
		Scan(&order.ID, &order.CustomerName, &order.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Order{}, ErrNotFound
	}
	if err != nil {
		return Order{}, err
	}
	rows, err := r.db.Query(ctx, `SELECT sku, quantity FROM order_items WHERE order_id=$1 ORDER BY position`, id)
	if err != nil {
		return Order{}, err
	}
	defer rows.Close()
	order.Items = make([]Item, 0)
	for rows.Next() {
		var item Item
		if err := rows.Scan(&item.SKU, &item.Quantity); err != nil {
			return Order{}, err
		}
		order.Items = append(order.Items, item)
	}
	return order, rows.Err()
}
