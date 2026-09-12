-- Migration: 075_purge_fictional_pvp_arena.sql
-- Description: Purge fictional Elo arena tables (arena_matches, arena_ratings) and add pvp_wins to characters (Issue #481)

DROP TABLE IF EXISTS arena_matches;
DROP TABLE IF EXISTS arena_ratings;

ALTER TABLE characters
    ADD COLUMN IF NOT EXISTS pvp_wins INT NOT NULL DEFAULT 0 AFTER hero_count;

INSERT IGNORE INTO schema_migrations (version) VALUES ('075_purge_fictional_pvp_arena');
