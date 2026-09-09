-- Migration 063: Home Estate Parity & Fictional Column Purge
-- Parity with legacy Party2 (home.cgi, _town.cgi):
-- 1. Adds characters.color for player chat/display font color (#RRGGBB).
-- 2. Adds town_id, house_style, and expires_at to character_homes for estate cycle management.
-- 3. Purges fictional columns: theme, motto, visitor_count, last_visited_at.

ALTER TABLE characters
    ADD COLUMN IF NOT EXISTS color VARCHAR(7) NOT NULL DEFAULT '#ffffff';

ALTER TABLE character_homes
    ADD COLUMN IF NOT EXISTS town_id VARCHAR(32) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS house_style VARCHAR(32) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS expires_at DATETIME(6) NULL;

ALTER TABLE character_homes
    ADD INDEX IF NOT EXISTS idx_character_homes_town_expires (town_id, expires_at);

ALTER TABLE character_homes
    DROP COLUMN IF EXISTS theme,
    DROP COLUMN IF EXISTS motto,
    DROP COLUMN IF EXISTS visitor_count,
    DROP COLUMN IF EXISTS last_visited_at;

INSERT IGNORE INTO schema_migrations (version) VALUES ('063_home_estate_parity');
