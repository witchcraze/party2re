-- 062_character_tired.sql: Add tired column to characters table
-- Parity with original Party2 ($m{tired}) for fatigue tracking and recovery.

ALTER TABLE characters ADD COLUMN IF NOT EXISTS tired INT NOT NULL DEFAULT 0;

INSERT IGNORE INTO schema_migrations (version) VALUES ('062_character_tired');
