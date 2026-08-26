ALTER TABLE characters ADD COLUMN small_medals INT NOT NULL DEFAULT 0;

INSERT IGNORE INTO schema_migrations (version) VALUES ('030_small_medals');
