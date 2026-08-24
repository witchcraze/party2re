CREATE TABLE IF NOT EXISTS character_monster_book (
    character_id CHAR(32) NOT NULL,
    monster_id VARCHAR(64) NOT NULL,
    monster_name VARCHAR(64) NOT NULL,
    habitat VARCHAR(64) NOT NULL DEFAULT '',
    defeated_count INT NOT NULL DEFAULT 1,
    first_defeated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_defeated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (character_id, monster_id),
    CONSTRAINT fk_monster_book_char FOREIGN KEY (character_id) REFERENCES characters (id) ON DELETE CASCADE
);

CREATE INDEX idx_monster_book_char ON character_monster_book (character_id);

CREATE TABLE IF NOT EXISTS character_item_collection (
    character_id CHAR(32) NOT NULL,
    item_id VARCHAR(64) NOT NULL,
    item_name VARCHAR(64) NOT NULL,
    category VARCHAR(32) NOT NULL,
    discovered_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (character_id, item_id),
    CONSTRAINT fk_item_collection_char FOREIGN KEY (character_id) REFERENCES characters (id) ON DELETE CASCADE
);

CREATE INDEX idx_item_collection_char ON character_item_collection (character_id);

INSERT IGNORE INTO schema_migrations (version) VALUES ('021_collection');
