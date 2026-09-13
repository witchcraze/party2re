-- Migration: 079_guild_applications_and_customization.sql
-- Description: Add guild mark, last_active_at, and member application is_pending flag (Issue #591)

ALTER TABLE guilds
    ADD COLUMN IF NOT EXISTS mark VARCHAR(64) NOT NULL DEFAULT '0' AFTER color,
    ADD COLUMN IF NOT EXISTS last_active_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP AFTER updated_at;

ALTER TABLE guilds
    ADD INDEX IF NOT EXISTS idx_guilds_last_active (last_active_at);

ALTER TABLE guild_members
    ADD COLUMN IF NOT EXISTS is_pending BOOLEAN NOT NULL DEFAULT FALSE AFTER title;

ALTER TABLE guild_members
    ADD INDEX IF NOT EXISTS idx_guild_members_pending (guild_id, is_pending);

INSERT IGNORE INTO schema_migrations (version) VALUES ('079_guild_applications_and_customization');
