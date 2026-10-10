-- Migration: 096_guild_pending_applications.sql
-- Description: Allow multiple pending guild applications per character with composite primary key (Issue #1244)

ALTER TABLE guild_members
    ADD INDEX IF NOT EXISTS idx_guild_members_character_id (character_id);

ALTER TABLE guild_members
    DROP PRIMARY KEY,
    ADD PRIMARY KEY (guild_id, character_id);

INSERT IGNORE INTO schema_migrations (version) VALUES ('096_guild_pending_applications');
