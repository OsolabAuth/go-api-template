CREATE TABLE orders (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  customer_name VARCHAR(200) NOT NULL CHECK (char_length(customer_name) BETWEEN 1 AND 200),
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE order_items (
  order_id BIGINT NOT NULL REFERENCES orders (id) ON DELETE CASCADE,
  position INTEGER NOT NULL CHECK (position BETWEEN 1 AND 100),
  sku VARCHAR(64) NOT NULL CHECK (char_length(sku) BETWEEN 1 AND 64),
  quantity INTEGER NOT NULL CHECK (quantity BETWEEN 1 AND 1000),
  PRIMARY KEY (order_id, position),
  CONSTRAINT order_items_order_sku_key UNIQUE (order_id, sku)
);
