CREATE TABLE IF NOT EXISTS guild_decorations (
    id CHAR(32) NOT NULL PRIMARY KEY,
    guild_id CHAR(32) NOT NULL,
    kind VARCHAR(32) NOT NULL,
    icon VARCHAR(255) NOT NULL,
    created_at DATETIME(6) NOT NULL,
    INDEX idx_guild_decorations (guild_id, created_at, id),
    CONSTRAINT fk_guild_decorations_guild FOREIGN KEY (guild_id) REFERENCES guilds(id) ON DELETE CASCADE
);

INSERT IGNORE INTO schema_migrations (version) VALUES ('131_guild_decorations');
