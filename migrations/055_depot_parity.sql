-- 055_depot_parity.sql: Align character_depots with original Party2 item storage specification
-- 1. Eliminate fictional gold deposit (gold is exclusively managed by bank)
-- 2. Add ex_depot (expansion count, 0-20)
-- 3. Adjust default capacity to 5 (dynamic capacity based on job_lv, ex_depot, over_depot)

ALTER TABLE character_depots DROP CONSTRAINT IF EXISTS chk_character_depots_gold;
ALTER TABLE character_depots DROP COLUMN IF EXISTS gold;
ALTER TABLE character_depots ADD COLUMN IF NOT EXISTS ex_depot INT NOT NULL DEFAULT 0;
ALTER TABLE character_depots MODIFY COLUMN capacity INT NOT NULL DEFAULT 5;
ALTER TABLE character_depots ADD CONSTRAINT chk_character_depots_ex_depot CHECK (ex_depot >= 0 AND ex_depot <= 20);

INSERT IGNORE INTO schema_migrations (version) VALUES ('055_depot_parity');
