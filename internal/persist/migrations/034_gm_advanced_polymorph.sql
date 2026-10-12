-- +goose Up
CREATE TABLE character_advanced_polymorphs (
    char_id INT NOT NULL REFERENCES characters(id) ON DELETE CASCADE,
    polyid INT NOT NULL,
    unlock_count INT NOT NULL DEFAULT 1 CHECK (unlock_count > 0),
    is_default BOOLEAN NOT NULL DEFAULT FALSE,
    unlocked_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (char_id, polyid)
);
CREATE UNIQUE INDEX idx_advanced_polymorph_default ON character_advanced_polymorphs(char_id) WHERE is_default;

-- +goose Down
DROP TABLE character_advanced_polymorphs;
