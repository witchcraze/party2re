-- Migration: 066_character_gem_boxes.sql
-- Description: Dedicated Gem Box storage table and gem items for Party2 gem store parity (Issue #464)

CREATE TABLE IF NOT EXISTS character_gem_boxes (
    character_id CHAR(32) NOT NULL PRIMARY KEY,
    capacity INT NOT NULL DEFAULT 5,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    CONSTRAINT fk_character_gem_boxes_character
        FOREIGN KEY (character_id) REFERENCES characters (id),
    CONSTRAINT chk_character_gem_boxes_capacity CHECK (capacity > 0)
);

CREATE TABLE IF NOT EXISTS gem_box_items (
    id CHAR(32) NOT NULL PRIMARY KEY,
    character_id CHAR(32) NOT NULL,
    definition_id VARCHAR(64) NOT NULL,
    slot_index INT NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT fk_gem_box_items_character
        FOREIGN KEY (character_id) REFERENCES characters (id)
);

CREATE INDEX idx_gem_box_items_character ON gem_box_items (character_id, slot_index);

INSERT IGNORE INTO schema_migrations (version) VALUES ('066_character_gem_boxes');
