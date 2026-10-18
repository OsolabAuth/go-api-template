package orders

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"example.com/go-api-template/internal/common"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestCreateValidation(t *testing.T) {
	body := func(name string, items []Item) string {
		t.Helper()
		data, err := json.Marshal(CreateOrder{CustomerName: name, Items: items})
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	validItems := []Item{{SKU: "book", Quantity: 1}}
	for _, tt := range []struct {
		name string
		body string
	}{
		{"empty body", ""},
		{"null body", "null"},
		{"unknown field", `{"customer_name":"A","items":[{"sku":"book","quantity":1,"price":10}]}`},
		{"extra JSON", `{"customer_name":"A","items":[{"sku":"book","quantity":1}]} {}`},
		{"blank customer", body(" \t ", validItems)},
		{"long customer", body(strings.Repeat("あ", 201), validItems)},
		{"NUL customer", `{"customer_name":"A\u0000B","items":[{"sku":"book","quantity":1}]}`},
		{"null items", body("A", nil)},
		{"empty items", body("A", []Item{})},
		{"too many items", body("A", make([]Item, 101))},
		{"blank SKU", body("A", []Item{{SKU: " \t ", Quantity: 1}})},
		{"long SKU", body("A", []Item{{SKU: strings.Repeat("あ", 65), Quantity: 1}})},
		{"NUL SKU", `{"customer_name":"A","items":[{"sku":"bo\u0000ok","quantity":1}]}`},
		{"zero quantity", body("A", []Item{{SKU: "book", Quantity: 0}})},
		{"negative quantity", body("A", []Item{{SKU: "book", Quantity: -1}})},
		{"large quantity", body("A", []Item{{SKU: "book", Quantity: 1001}})},
		{"fractional quantity", `{"customer_name":"A","items":[{"sku":"book","quantity":1.5}]}`},
		{"string quantity", `{"customer_name":"A","items":[{"sku":"book","quantity":"1"}]}`},
		{"missing quantity", `{"customer_name":"A","items":[{"sku":"book"}]}`},
		{"null quantity", `{"customer_name":"A","items":[{"sku":"book","quantity":null}]}`},
		{"wrong items type", `{"customer_name":"A","items":{}}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			// Invalid input must be rejected before a repository is needed.
			router := NewHandler(nil, slog.New(slog.NewTextHandler(io.Discard, nil))).Routes()
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tt.body)))
			if response.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400; body = %s", response.Code, response.Body.String())
			}
		})
	}
}

func TestCreateBoundaryAndDatabaseFailure(t *testing.T) {
	pool, err := pgxpool.New(context.Background(), "postgres://postgres:postgres@127.0.0.1:1/postgres")
	if err != nil {
		t.Fatal(err)
	}
	pool.Close()
	items := make([]Item, 100)
	for i := range items {
		items[i] = Item{SKU: fmt.Sprintf("sku-%d", i), Quantity: 1000}
	}
	items[0].SKU = " " + strings.Repeat("あ", 64) + " "
	data, err := json.Marshal(CreateOrder{CustomerName: " " + strings.Repeat("あ", 200) + " ", Items: items})
	if err != nil {
		t.Fatal(err)
	}
	var logs bytes.Buffer
	router := middleware.RequestID(NewHandler(NewRepository(pool), slog.New(slog.NewJSONHandler(&logs, nil))).Routes())
	request := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(data))
	request.Header.Set("X-Request-Id", "order-test")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want valid input to reach the closed DB; body = %s", response.Code, response.Body.String())
	}
	var apiError common.APIError
	if err := json.Unmarshal(response.Body.Bytes(), &apiError); err != nil {
		t.Fatal(err)
	}
	if apiError.Error != "internal server error" || apiError.RequestID != "order-test" {
		t.Fatalf("unexpected public error: %+v", apiError)
	}
	var entry struct {
		Error     string `json:"error"`
		RequestID string `json:"request_id"`
	}
	if err := json.Unmarshal(logs.Bytes(), &entry); err != nil {
		t.Fatal(err)
	}
	if entry.Error == "" || entry.RequestID != "order-test" {
		t.Fatalf("missing internal error details or request ID: %+v", entry)
	}
}

func TestGetInvalidID(t *testing.T) {
	router := NewHandler(nil, slog.New(slog.NewTextHandler(io.Discard, nil))).Routes()
	for _, id := range []string{"0", "-1", "abc", "9223372036854775808"} {
		t.Run(id, func(t *testing.T) {
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/"+id, nil))
			if response.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400", response.Code)
			}
		})
	}
}
