package orders

import "time"

type Item struct {
	SKU      string `json:"sku"`
	Quantity int    `json:"quantity"`
}

type CreateOrder struct {
	CustomerName string `json:"customer_name"`
	Items        []Item `json:"items"`
}

type Order struct {
	ID           int64     `json:"id"`
	CustomerName string    `json:"customer_name"`
	Items        []Item    `json:"items"`
	CreatedAt    time.Time `json:"created_at"`
}
