CREATE TABLE IF NOT EXISTS home_members (
    id VARCHAR(32) NOT NULL PRIMARY KEY,
    character_id VARCHAR(32) NOT NULL,
    is_npc BOOLEAN NOT NULL DEFAULT TRUE,
    name VARCHAR(64) NOT NULL,
    icon VARCHAR(64) NOT NULL,
    color VARCHAR(32) NOT NULL DEFAULT '',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT fk_home_members_character FOREIGN KEY (character_id) REFERENCES characters (id) ON DELETE CASCADE,
    INDEX idx_home_members_character (character_id, created_at ASC)
);

ALTER TABLE character_homes ADD COLUMN IF NOT EXISTS bgimg VARCHAR(64) NOT NULL DEFAULT '';
ALTER TABLE guilds ADD COLUMN IF NOT EXISTS bgimg VARCHAR(64) NOT NULL DEFAULT '';

INSERT IGNORE INTO schema_migrations (version) VALUES ('061_home_members_and_god_parity');
