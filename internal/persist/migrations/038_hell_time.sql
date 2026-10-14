-- +goose Up
-- PK 地獄剩餘秒數（Java: characters.HellTime）。重登或重生時續算。
ALTER TABLE characters ADD COLUMN hell_time INT NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE characters DROP COLUMN hell_time;
