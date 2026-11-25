-- Drop non-legacy per-character boss records and challenge history (#2762).
DROP TABLE IF EXISTS boss_challenge_history;
DROP TABLE IF EXISTS character_boss_records;

INSERT IGNORE INTO schema_migrations (version) VALUES ('146_drop_boss_records');
