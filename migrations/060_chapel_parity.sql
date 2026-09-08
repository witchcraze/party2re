-- 060_chapel_parity.sql: Eliminate fictional donation_gold_total from character_blessings table
-- 1. Eliminate fictional donations column and non-negative check constraint
-- 2. Preserve active_blessing, prayed_at, updated_at for single-active-wish parity

ALTER TABLE character_blessings DROP CONSTRAINT IF EXISTS chk_character_blessings_donation;
ALTER TABLE character_blessings DROP COLUMN IF EXISTS donation_gold_total;

INSERT IGNORE INTO schema_migrations (version) VALUES ('060_chapel_parity');
