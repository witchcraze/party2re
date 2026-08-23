CREATE TABLE IF NOT EXISTS character_depots (
    character_id CHAR(32) NOT NULL PRIMARY KEY,
    gold INT NOT NULL DEFAULT 0,
    capacity INT NOT NULL DEFAULT 50,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    CONSTRAINT fk_character_depots_character
        FOREIGN KEY (character_id) REFERENCES characters (id),
    CONSTRAINT chk_character_depots_gold CHECK (gold >= 0),
    CONSTRAINT chk_character_depots_capacity CHECK (capacity > 0)
);

CREATE TABLE IF NOT EXISTS depot_items (
    id CHAR(32) NOT NULL PRIMARY KEY,
    character_id CHAR(32) NOT NULL,
    definition_id VARCHAR(64) NOT NULL,
    quantity INT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT fk_depot_items_character
        FOREIGN KEY (character_id) REFERENCES characters (id),
    CONSTRAINT chk_depot_items_quantity CHECK (quantity > 0)
);

CREATE INDEX idx_depot_items_character ON depot_items (character_id);

INSERT IGNORE INTO schema_migrations (version) VALUES ('011_depot');
