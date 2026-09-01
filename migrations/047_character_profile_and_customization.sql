CREATE TABLE IF NOT EXISTS character_profiles (
    character_id CHAR(32) NOT NULL PRIMARY KEY,
    comment VARCHAR(160) NOT NULL DEFAULT '',
    avatar_url VARCHAR(512) NOT NULL DEFAULT '',
    bio_data JSON DEFAULT NULL,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    FOREIGN KEY (character_id) REFERENCES characters(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_characters_name ON characters(name);

INSERT IGNORE INTO schema_migrations (version) VALUES ('047_character_profile_and_customization');
