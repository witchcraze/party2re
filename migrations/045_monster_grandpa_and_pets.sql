CREATE TABLE IF NOT EXISTS character_monsters (
    id CHAR(32) NOT NULL PRIMARY KEY,
    character_id CHAR(32) NOT NULL,
    monster_id VARCHAR(64) NOT NULL,
    custom_name VARCHAR(64) NOT NULL,
    location VARCHAR(32) NOT NULL DEFAULT 'box',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    INDEX idx_char_monsters_char_loc (character_id, location),
    INDEX idx_char_monsters_created (created_at),
    CONSTRAINT fk_char_monsters_character FOREIGN KEY (character_id) REFERENCES characters (id) ON DELETE CASCADE
);

INSERT IGNORE INTO schema_migrations (version) VALUES ('045_monster_grandpa_and_pets');
