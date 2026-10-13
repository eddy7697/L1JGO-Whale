-- +goose Up
-- 舊版未保存創角 STR/CON；以目前永久屬性做一次性基準。
-- 歷史升級配點無法逆推，後續創角與洗點會保存正確的初始屬性。
ALTER TABLE characters ADD COLUMN original_str SMALLINT NOT NULL DEFAULT 0;
ALTER TABLE characters ADD COLUMN original_con SMALLINT NOT NULL DEFAULT 0;
UPDATE characters SET original_str = str, original_con = con;

-- +goose Down
ALTER TABLE characters DROP COLUMN original_str;
ALTER TABLE characters DROP COLUMN original_con;
