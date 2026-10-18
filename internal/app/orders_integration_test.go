package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"example.com/go-api-template/internal/common"
	"example.com/go-api-template/internal/resources/orders"
	"example.com/go-api-template/internal/resources/tasks"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestOrderAPI(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set TEST_DATABASE_URL to run the PostgreSQL integration test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Close(context.Background()) })

	// Keep the sample self-contained and leave every other schema untouched.
	schema := pgx.Identifier{fmt.Sprintf("test_orders_%d_%d", os.Getpid(), time.Now().UnixNano())}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		if _, err := admin.Exec(cleanupCtx, "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Errorf("remove test schema: %v", err)
		}
	})
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	migrate := func(path string) {
		t.Helper()
		migration, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, string(migration)); err != nil {
			t.Fatal(err)
		}
	}
	migrate("../../migrations/000001_create_tasks.up.sql")
	migrate("../../migrations/000002_create_orders.up.sql")

	router := Router(pool, slog.New(slog.NewTextHandler(io.Discard, nil)))
	request := func(method, path, body string, wantStatus int, result any) {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body)).WithContext(ctx)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Request-Id", "order-integration")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		if response.Code != wantStatus {
			t.Fatalf("%s %s: status = %d, want %d; body = %s", method, path, response.Code, wantStatus, response.Body.String())
		}
		if result != nil {
			if err := json.Unmarshal(response.Body.Bytes(), result); err != nil {
				t.Fatal(err)
			}
		}
	}
	assertCounts := func(wantOrders, wantItems int) {
		t.Helper()
		var orderCount, itemCount int
		if err := pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM orders), (SELECT count(*) FROM order_items)`).Scan(&orderCount, &itemCount); err != nil {
			t.Fatal(err)
		}
		if orderCount != wantOrders || itemCount != wantItems {
			t.Fatalf("row counts = orders:%d items:%d, want orders:%d items:%d", orderCount, itemCount, wantOrders, wantItems)
		}
	}

	var created orders.Order
	request(http.MethodPost, "/api/v1/orders", `{"customer_name":" 山田 ","items":[{"sku":" book ","quantity":2},{"sku":"pen","quantity":1}]}`, http.StatusCreated, &created)
	wantItems := []orders.Item{{SKU: "book", Quantity: 2}, {SKU: "pen", Quantity: 1}}
	if created.ID <= 0 || created.CustomerName != "山田" || created.CreatedAt.IsZero() || !reflect.DeepEqual(created.Items, wantItems) {
		t.Fatalf("unexpected created order: %+v", created)
	}
	assertCounts(1, 2)
	path := "/api/v1/orders/" + strconv.FormatInt(created.ID, 10)
	var fetched orders.Order
	request(http.MethodGet, path, "", http.StatusOK, &fetched)
	if !reflect.DeepEqual(fetched, created) {
		t.Fatalf("fetched order = %+v, want %+v", fetched, created)
	}
	request(http.MethodGet, "/api/v1/orders/9223372036854775807", "", http.StatusNotFound, nil)

	// The order and first item are inserted before the second item violates UNIQUE.
	// The 409 alone is insufficient: neither the order nor its first item may remain.
	var conflict common.APIError
	request(http.MethodPost, "/api/v1/orders", `{"customer_name":"rollback","items":[{"sku":"book","quantity":1},{"sku":" book ","quantity":2}]}`, http.StatusConflict, &conflict)
	if conflict.Error != orders.ErrDuplicateSKU.Error() || conflict.RequestID != "order-integration" {
		t.Fatalf("unexpected conflict response: %+v", conflict)
	}
	assertCounts(1, 2)
	request(http.MethodGet, path, "", http.StatusOK, &fetched)
	if !reflect.DeepEqual(fetched, created) {
		t.Fatalf("rollback changed an existing order: %+v", fetched)
	}

	// A SKU can appear in a different order; the connection is usable after rollback.
	request(http.MethodPost, "/api/v1/orders/", `{"customer_name":"next","items":[{"sku":"book","quantity":1000}]}`, http.StatusCreated, nil)
	assertCounts(2, 3)

	// Removing this example's schema change must preserve the original task resource.
	var task tasks.Task
	request(http.MethodPost, "/api/v1/tasks/", `{"title":"keep this task"}`, http.StatusCreated, &task)
	migrate("../../migrations/000002_create_orders.down.sql")
	var removed bool
	if err := pool.QueryRow(ctx, `SELECT to_regclass('orders') IS NULL AND to_regclass('order_items') IS NULL`).Scan(&removed); err != nil {
		t.Fatal(err)
	}
	if !removed {
		t.Fatal("order tables still exist after down migration")
	}
	var fetchedTask tasks.Task
	request(http.MethodGet, "/api/v1/tasks/"+strconv.FormatInt(task.ID, 10), "", http.StatusOK, &fetchedTask)
	if !reflect.DeepEqual(fetchedTask, task) {
		t.Fatalf("down migration changed the task: %+v", fetchedTask)
	}
}
