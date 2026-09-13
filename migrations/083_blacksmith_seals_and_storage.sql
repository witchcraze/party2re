-- Migration: 083_blacksmith_seals_and_storage.sql
-- Description: Add authentic 12 weapon seals (wea_seal), custom names (wea_name, arm_name), and crystal currency (crystal) to characters; create dedicated 3-slot blacksmith storage (blacksmith_deposits) (Issue #458)

ALTER TABLE characters
    ADD COLUMN IF NOT EXISTS crystal INT NOT NULL DEFAULT 0 AFTER deposit,
    ADD COLUMN IF NOT EXISTS wea_seal INT NOT NULL DEFAULT 0 AFTER crystal,
    ADD COLUMN IF NOT EXISTS wea_name VARCHAR(60) NOT NULL DEFAULT '' AFTER wea_seal,
    ADD COLUMN IF NOT EXISTS arm_name VARCHAR(60) NOT NULL DEFAULT '' AFTER wea_name;

CREATE TABLE IF NOT EXISTS blacksmith_deposits (
    id CHAR(32) NOT NULL PRIMARY KEY,
    character_id CHAR(32) NOT NULL,
    slot INT NOT NULL,
    item_definition_id VARCHAR(64) NOT NULL,
    seal_id INT NOT NULL DEFAULT 0,
    custom_name VARCHAR(60) NOT NULL DEFAULT '',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT fk_blacksmith_deposits_character
        FOREIGN KEY (character_id) REFERENCES characters (id) ON DELETE CASCADE,
    CONSTRAINT uq_blacksmith_deposits_char_slot
        UNIQUE (character_id, slot)
);

INSERT IGNORE INTO schema_migrations (version) VALUES ('083_blacksmith_seals_and_storage');
