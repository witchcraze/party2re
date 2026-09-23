-- Migration: 088_character_casino_wins.sql
-- Description: Add casino_wins to characters table (Issue #799)

ALTER TABLE characters
    ADD COLUMN IF NOT EXISTS casino_wins INT NOT NULL DEFAULT 0 AFTER pvp_wins;

INSERT IGNORE INTO schema_migrations (version) VALUES ('088_character_casino_wins');
