-- Migration: 078_guild_points_and_custom_titles.sql
-- Description: Purge fictional donation leveling (level, exp, gold) and capacity scaling; add dynamic guild points (points) and custom member titles (title) (Issue #490)

ALTER TABLE guilds
    DROP COLUMN IF EXISTS level,
    DROP COLUMN IF EXISTS exp,
    DROP COLUMN IF EXISTS gold,
    ADD COLUMN IF NOT EXISTS points BIGINT NOT NULL DEFAULT 0 AFTER leader_character_id;

ALTER TABLE guilds
    ADD INDEX IF NOT EXISTS idx_guilds_points (points DESC);

ALTER TABLE guild_members
    DROP COLUMN IF EXISTS total_donated_gold,
    ADD COLUMN IF NOT EXISTS title VARCHAR(36) NOT NULL DEFAULT '' AFTER role;

INSERT IGNORE INTO schema_migrations (version) VALUES ('078_guild_points_and_custom_titles');
