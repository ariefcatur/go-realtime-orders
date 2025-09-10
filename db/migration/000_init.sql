CREATE EXTENSION IF NOT EXISTS pgcrypto; -- untuk gen_random_uuid()

-- Products
CREATE TABLE products (
                          id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
                          sku           TEXT NOT NULL UNIQUE,
                          name          TEXT NOT NULL,
                          stock         INTEGER NOT NULL CHECK (stock >= 0),
                          price_cents   INTEGER NOT NULL CHECK (price_cents >= 0),
                          created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
                          updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Orders
CREATE TABLE orders (
                        id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
                        external_id   TEXT NOT NULL UNIQUE,        -- buat idempotency saat POST /orders
                        user_id       UUID NOT NULL,
                        status        TEXT NOT NULL,               -- CREATED | STOCK_RESERVED | PAID | COMPLETED | FAILED
                        total_cents   INTEGER NOT NULL CHECK (total_cents >= 0),
                        created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
                        updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_orders_status_created_at ON orders(status, created_at DESC);

-- Order Items
CREATE TABLE order_items (
                             id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
                             order_id      UUID NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
                             product_id    UUID NOT NULL REFERENCES products(id),
                             qty           INTEGER NOT NULL CHECK (qty > 0),
                             price_cents   INTEGER NOT NULL CHECK (price_cents >= 0)
);

CREATE INDEX idx_order_items_order ON order_items(order_id);

-- Reservations (untuk reserve/release stock per item)
CREATE TABLE reservations (
                              id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
                              order_id      UUID NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
                              product_id    UUID NOT NULL REFERENCES products(id),
                              qty           INTEGER NOT NULL CHECK (qty > 0),
                              status        TEXT NOT NULL,              -- RESERVED | RELEASED
                              created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Satu order satu baris reservation per product
ALTER TABLE reservations
    ADD CONSTRAINT uq_reservation UNIQUE(order_id, product_id);

-- (Opsional, tapi sangat direkomendasikan ke depan)
-- Outbox Pattern untuk atomic write + publish
CREATE TABLE outbox_messages (
                                 id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
                                 aggregate_type TEXT NOT NULL,             -- 'order'
                                 aggregate_id   UUID NOT NULL,             -- order_id
                                 event_type     TEXT NOT NULL,             -- OrderCreated, dsb
                                 payload        JSONB NOT NULL,
                                 headers        JSONB NOT NULL DEFAULT '{}'::jsonb,
                                 published      BOOLEAN NOT NULL DEFAULT false,
                                 created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_outbox_unpublished ON outbox_messages(published, created_at);
