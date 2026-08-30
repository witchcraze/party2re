CREATE TABLE IF NOT EXISTS tavern_deliveries (
    character_id CHAR(32) NOT NULL PRIMARY KEY,
    item_id VARCHAR(64) NOT NULL,
    item_name VARCHAR(128) NOT NULL,
    price INT NOT NULL,
    hp_heal INT NOT NULL,
    mp_heal INT NOT NULL,
    tickets INT NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT fk_tavern_deliveries_char FOREIGN KEY (character_id) REFERENCES characters (id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS tavern_character_status (
    character_id CHAR(32) NOT NULL PRIMARY KEY,
    is_full BOOLEAN NOT NULL DEFAULT FALSE,
    last_eaten_at TIMESTAMP NULL,
    total_meals_eaten INT NOT NULL DEFAULT 0,
    total_gold_spent BIGINT NOT NULL DEFAULT 0,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    CONSTRAINT fk_tavern_char_status FOREIGN KEY (character_id) REFERENCES characters (id) ON DELETE CASCADE
);

INSERT IGNORE INTO schema_migrations (version) VALUES ('039_tavern');
