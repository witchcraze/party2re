-- Drop the non-legacy achievement and commemorative medal records (#2302).
DROP TABLE IF EXISTS character_achievements;
DROP TABLE IF EXISTS character_medals;

INSERT IGNORE INTO schema_migrations (version) VALUES ('135_drop_achievements_and_medals');
