-- +goose Up

CREATE TABLE banned_ips (
    ip INET PRIMARY KEY,
    banned_by VARCHAR(32) NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE characters
    ADD COLUMN home_town_id SMALLINT NOT NULL DEFAULT 0,
    ADD COLUMN contribution BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN town_pay BIGINT NOT NULL DEFAULT 0;

CREATE TABLE towns (
    town_id SMALLINT PRIMARY KEY,
    name VARCHAR(32) NOT NULL,
    leader_id INT NOT NULL DEFAULT 0,
    leader_name VARCHAR(32) NOT NULL DEFAULT '',
    tax_rate SMALLINT NOT NULL DEFAULT 0,
    tax_rate_reserved SMALLINT NOT NULL DEFAULT 0,
    sales_money BIGINT NOT NULL DEFAULT 0,
    sales_money_yesterday BIGINT NOT NULL DEFAULT 0,
    town_tax BIGINT NOT NULL DEFAULT 0,
    town_fix_tax BIGINT NOT NULL DEFAULT 0
);

INSERT INTO towns(town_id,name) VALUES
(1,'說話之島'),(2,'銀騎士村莊'),(3,'古魯丁村莊'),(4,'燃柳村莊'),(5,'風木村莊'),
(6,'肯特村莊'),(7,'奇岩村莊'),(8,'海音村莊'),(9,'威頓村莊'),(10,'歐瑞村莊');

-- +goose Down
DROP TABLE towns;
ALTER TABLE characters DROP COLUMN town_pay, DROP COLUMN contribution, DROP COLUMN home_town_id;
DROP TABLE banned_ips;
