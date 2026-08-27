CREATE TABLE IF NOT EXISTS park_posts (
    id CHAR(32) NOT NULL PRIMARY KEY,
    character_id CHAR(32) NOT NULL,
    character_name VARCHAR(64) NOT NULL,
    content VARCHAR(255) NOT NULL,
    color VARCHAR(32) NOT NULL DEFAULT '#000000',
    recipient_name VARCHAR(64) NOT NULL DEFAULT '',
    created_at TIMESTAMP(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    CONSTRAINT fk_park_posts_character FOREIGN KEY (character_id) REFERENCES characters (id) ON DELETE CASCADE,
    INDEX idx_park_posts_created_at (created_at DESC),
    INDEX idx_park_posts_char_created (character_id, created_at DESC)
);

INSERT IGNORE INTO schema_migrations (version) VALUES ('032_town_park');
