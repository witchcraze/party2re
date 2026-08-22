CREATE TABLE IF NOT EXISTS item_definitions (
    id VARCHAR(64) NOT NULL PRIMARY KEY,
    name VARCHAR(128) NOT NULL
);

CREATE TABLE IF NOT EXISTS inventory_items (
    id CHAR(32) NOT NULL PRIMARY KEY,
    character_id CHAR(32) NOT NULL,
    definition_id VARCHAR(64) NOT NULL,
    quantity INT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT fk_inventory_items_character
        FOREIGN KEY (character_id) REFERENCES characters (id),
    CONSTRAINT chk_inventory_items_quantity CHECK (quantity > 0)
);

CREATE INDEX idx_inventory_items_character ON inventory_items (character_id);

INSERT IGNORE INTO schema_migrations (version) VALUES ('005_items_inventory');
