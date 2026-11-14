-- Milestone log of a character (legacy memory.cgi). Each row keeps who the character was when it was written.
CREATE TABLE IF NOT EXISTS character_memories (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
    character_id CHAR(32) NOT NULL,
    kind VARCHAR(32) NOT NULL,
    content VARCHAR(255) NOT NULL,
    name VARCHAR(64) NOT NULL,
    guild_name VARCHAR(64) NOT NULL DEFAULT '',
    color VARCHAR(16) NOT NULL DEFAULT '',
    level INT NOT NULL,
    job_id VARCHAR(32) NOT NULL,
    gender VARCHAR(8) NOT NULL,
    sp INT NOT NULL,
    created_at DATETIME(6) NOT NULL,
    CONSTRAINT fk_character_memories_character
        FOREIGN KEY (character_id) REFERENCES characters (id) ON DELETE CASCADE,
    INDEX idx_character_memories_character (character_id, id)
);

INSERT IGNORE INTO schema_migrations (version) VALUES ('132_character_memories');
