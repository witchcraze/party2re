ALTER TABLE characters ADD COLUMN IF NOT EXISTS pending_wake TINYINT(1) NOT NULL DEFAULT 0;

INSERT IGNORE INTO schema_migrations (version) VALUES ('093_character_pending_wake');
