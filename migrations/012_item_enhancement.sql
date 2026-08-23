ALTER TABLE inventory_items ADD COLUMN enhancement_level INT NOT NULL DEFAULT 0;
ALTER TABLE depot_items ADD COLUMN enhancement_level INT NOT NULL DEFAULT 0;

INSERT IGNORE INTO schema_migrations (version) VALUES ('012_item_enhancement');
