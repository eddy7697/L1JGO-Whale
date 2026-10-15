-- +goose Up
ALTER TABLE characters ADD COLUMN elf_attr SMALLINT NOT NULL DEFAULT 0
    CHECK (elf_attr IN (0, 1, 2, 4, 8));

-- +goose Down
ALTER TABLE characters DROP COLUMN elf_attr;
