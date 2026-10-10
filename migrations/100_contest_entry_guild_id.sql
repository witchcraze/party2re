ALTER TABLE contest_entries
    ADD COLUMN guild_id CHAR(32) NOT NULL DEFAULT '' AFTER character_name;

INSERT IGNORE INTO schema_migrations (version) VALUES ('100_contest_entry_guild_id');
