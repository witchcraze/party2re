CREATE TABLE IF NOT EXISTS guilds (
    id CHAR(32) NOT NULL PRIMARY KEY,
    name VARCHAR(64) NOT NULL UNIQUE,
    leader_character_id CHAR(32) NOT NULL,
    level INT NOT NULL DEFAULT 1,
    exp BIGINT NOT NULL DEFAULT 0,
    gold BIGINT NOT NULL DEFAULT 0,
    notice TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    CONSTRAINT fk_guilds_leader
        FOREIGN KEY (leader_character_id) REFERENCES characters (id)
);

CREATE TABLE IF NOT EXISTS guild_members (
    guild_id CHAR(32) NOT NULL,
    character_id CHAR(32) NOT NULL PRIMARY KEY,
    role VARCHAR(16) NOT NULL,
    joined_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    total_donated_gold BIGINT NOT NULL DEFAULT 0,
    CONSTRAINT fk_guild_members_guild
        FOREIGN KEY (guild_id) REFERENCES guilds (id) ON DELETE CASCADE,
    CONSTRAINT fk_guild_members_character
        FOREIGN KEY (character_id) REFERENCES characters (id) ON DELETE CASCADE
);

CREATE INDEX idx_guild_members_guild_id ON guild_members (guild_id);

INSERT IGNORE INTO schema_migrations (version) VALUES ('016_guilds');
