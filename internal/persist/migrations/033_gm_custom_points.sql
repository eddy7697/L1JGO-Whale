-- +goose Up
CREATE TABLE custom_point_currencies (
    code VARCHAR(16) PRIMARY KEY,
    display_name VARCHAR(32) NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    sort_order INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
INSERT INTO custom_point_currencies (code, display_name) VALUES ('MP', '商城點數');

CREATE TABLE custom_points (
    login VARCHAR(32) NOT NULL REFERENCES accounts(name),
    currency_code VARCHAR(16) NOT NULL REFERENCES custom_point_currencies(code),
    balance BIGINT NOT NULL DEFAULT 0 CHECK (balance >= 0),
    total_credited BIGINT NOT NULL DEFAULT 0 CHECK (total_credited >= 0),
    total_debited BIGINT NOT NULL DEFAULT 0 CHECK (total_debited >= 0),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (login, currency_code)
);

CREATE TABLE custom_point_logs (
    id BIGSERIAL PRIMARY KEY,
    login VARCHAR(32) NOT NULL REFERENCES accounts(name),
    currency_code VARCHAR(16) NOT NULL REFERENCES custom_point_currencies(code),
    char_id INT,
    delta BIGINT NOT NULL CHECK (delta <> 0),
    balance_after BIGINT NOT NULL CHECK (balance_after >= 0),
    type VARCHAR(32) NOT NULL,
    source VARCHAR(64) NOT NULL,
    external_order_id VARCHAR(64) UNIQUE,
    refund_of_order_id VARCHAR(64) UNIQUE,
    memo VARCHAR(255) NOT NULL,
    gm_name VARCHAR(32) NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX custom_point_logs_account_recent ON custom_point_logs (login, id DESC);
CREATE INDEX custom_point_logs_currency_recent ON custom_point_logs (login, currency_code, id DESC);

-- +goose Down
DROP TABLE custom_point_logs;
DROP TABLE custom_points;
DROP TABLE custom_point_currencies;
