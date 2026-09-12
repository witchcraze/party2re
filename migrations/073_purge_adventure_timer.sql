-- Migration: 073_purge_adventure_timer.sql
-- Description: Purge fictional 1-hour adventure expedition timer and add 10-floor crawl fields (Issue #478)

DROP INDEX IF EXISTS idx_adventures_character_claimed ON adventures;

ALTER TABLE adventures
    DROP COLUMN IF EXISTS available_at,
    DROP COLUMN IF EXISTS claimed,
    ADD COLUMN floors_cleared INT NOT NULL DEFAULT 0 AFTER battle_turns,
    ADD COLUMN is_cleared BOOLEAN NOT NULL DEFAULT FALSE AFTER floors_cleared,
    ADD COLUMN party_size INT NOT NULL DEFAULT 1 AFTER is_cleared;

ALTER TABLE adventures
    ADD INDEX idx_adventures_character_outcome (character_id, outcome, is_cleared);

INSERT IGNORE INTO schema_migrations (version) VALUES ('073_purge_adventure_timer');
