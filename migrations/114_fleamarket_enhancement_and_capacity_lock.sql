ALTER TABLE fleamarket_listings ADD COLUMN enhancement_level INT NOT NULL DEFAULT 0 AFTER item_category;

CREATE TABLE IF NOT EXISTS fleamarket_capacity_lock (
    id TINYINT NOT NULL PRIMARY KEY
);
INSERT IGNORE INTO fleamarket_capacity_lock (id) VALUES (1);

INSERT IGNORE INTO schema_migrations (version) VALUES ('114_fleamarket_enhancement_and_capacity_lock');
