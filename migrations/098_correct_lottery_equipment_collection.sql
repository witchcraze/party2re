-- Migration: 098_correct_lottery_equipment_collection.sql
-- Description: Correct equipment items mistakenly saved with category 'item' in character_item_collection (Issue #1239)

UPDATE character_item_collection
SET category = 'weapon'
WHERE item_id LIKE 'weapon-%' AND category != 'weapon';

UPDATE character_item_collection
SET category = 'armor'
WHERE item_id LIKE 'armor-%' AND category != 'armor';

INSERT IGNORE INTO schema_migrations (version) VALUES ('098_correct_lottery_equipment_collection');
