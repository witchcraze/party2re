-- 056_eliminate_rebirth_add_sp.sql: Eliminate fictional rebirth_count and add sp column
-- 1. Add sp (Skill Points) to characters table
-- 2. Drop index idx_characters_rebirth and drop column rebirth_count from characters table

ALTER TABLE characters ADD COLUMN IF NOT EXISTS sp INT NOT NULL DEFAULT 0;
DROP INDEX IF EXISTS idx_characters_rebirth ON characters;
ALTER TABLE characters DROP COLUMN IF EXISTS rebirth_count;
DELETE FROM ranking_snapshots WHERE ranking_type = 'rebirth';

INSERT IGNORE INTO schema_migrations (version) VALUES ('056_eliminate_rebirth_add_sp');
