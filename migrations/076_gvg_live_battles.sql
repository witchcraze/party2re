-- Migration: 076_gvg_live_battles.sql
-- Description: Purge fictional GvG Elo rating and match tables, restore authentic 7 trophy tiers and guild color (Issue #482)

DROP TABLE IF EXISTS gvg_match_rounds;
DROP TABLE IF EXISTS gvg_matches;

ALTER TABLE gvg_standings
    DROP COLUMN IF EXISTS rating,
    ADD COLUMN IF NOT EXISTS orders INT NOT NULL DEFAULT 0 AFTER gold_medals;

ALTER TABLE guilds
    ADD COLUMN IF NOT EXISTS color VARCHAR(32) NOT NULL DEFAULT '#FFFFFF' AFTER notice;

INSERT IGNORE INTO schema_migrations (version) VALUES ('076_gvg_live_battles');
