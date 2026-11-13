ALTER TABLE characters ADD COLUMN IF NOT EXISTS god_visit VARCHAR(16) NOT NULL DEFAULT '';

INSERT IGNORE INTO schema_migrations (version) VALUES ('128_character_god_visit');
