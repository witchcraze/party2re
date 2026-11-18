-- Public guild list (guild_list.cgi): one snapshot taken with the daily point decay.
CREATE TABLE IF NOT EXISTS guild_list_snapshot (
    id TINYINT NOT NULL PRIMARY KEY,
    guilds MEDIUMTEXT NOT NULL,
    generated_at DATETIME(6) NOT NULL
);

INSERT IGNORE INTO schema_migrations (version) VALUES ('137_guild_list_snapshot');
