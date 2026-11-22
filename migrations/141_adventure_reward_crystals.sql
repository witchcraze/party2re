-- 141_adventure_reward_crystals.sql: Persist crystal rewards in adventure history (Issue #2283)
ALTER TABLE adventures
    ADD COLUMN IF NOT EXISTS reward_crystals INT NOT NULL DEFAULT 0 AFTER reward_currency;

INSERT IGNORE INTO schema_migrations (version) VALUES ('141_adventure_reward_crystals');
