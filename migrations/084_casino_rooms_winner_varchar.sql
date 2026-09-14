-- 084_casino_rooms_winner_varchar.sql: Expand winner_character_id column to prevent truncation crashes on multi-winner High-Low and Doppelganger games

ALTER TABLE casino_rooms MODIFY COLUMN winner_character_id VARCHAR(255) NULL;

INSERT IGNORE INTO schema_migrations (version) VALUES ('084_casino_rooms_winner_varchar');
