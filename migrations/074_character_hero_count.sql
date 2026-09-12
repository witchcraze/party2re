-- Migration: 074_character_hero_count.sql
-- Description: Add hero_count to characters and purge fictional daily attempts from character_boss_records (Issue #479)

ALTER TABLE characters
    ADD COLUMN IF NOT EXISTS hero_count INT NOT NULL DEFAULT 0 AFTER help_count;

ALTER TABLE character_boss_records
    DROP COLUMN IF EXISTS daily_attempts_used,
    DROP COLUMN IF EXISTS daily_attempts_reset_at;

INSERT IGNORE INTO schema_migrations (version) VALUES ('074_character_hero_count');
