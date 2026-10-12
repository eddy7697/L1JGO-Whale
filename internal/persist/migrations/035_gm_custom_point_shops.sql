-- +goose Up
-- Management catalogue for Java .cpshopreload. No live shops are enabled by migration.
CREATE TABLE IF NOT EXISTS custom_point_shops (
    shop_id SERIAL PRIMARY KEY,
    npc_id INTEGER NOT NULL UNIQUE CHECK (npc_id > 0),
    name VARCHAR(64) NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT TRUE
);

CREATE TABLE IF NOT EXISTS custom_point_shop_items (
    id SERIAL PRIMARY KEY,
    shop_id INTEGER NOT NULL REFERENCES custom_point_shops(shop_id),
    item_id INTEGER NOT NULL CHECK (item_id > 0),
    price_currency VARCHAR(16) NOT NULL,
    price BIGINT NOT NULL CHECK (price > 0),
    pack_count INTEGER NOT NULL DEFAULT 1 CHECK (pack_count > 0),
    daily_limit INTEGER NOT NULL DEFAULT 0 CHECK (daily_limit >= 0),
    account_limit INTEGER NOT NULL DEFAULT 0 CHECK (account_limit >= 0),
    sort_order INTEGER NOT NULL DEFAULT 0,
    enabled BOOLEAN NOT NULL DEFAULT TRUE
);
CREATE INDEX IF NOT EXISTS custom_point_shop_items_shop_idx ON custom_point_shop_items(shop_id, enabled);

-- +goose Down
DROP TABLE custom_point_shop_items;
DROP TABLE custom_point_shops;
