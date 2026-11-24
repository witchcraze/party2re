-- Remove the obsolete history item count after removing the invented dungeon monster drop path (#2741).
ALTER TABLE dungeon_expedition_history
    DROP COLUMN IF EXISTS items_reward_count;

INSERT IGNORE INTO schema_migrations (version) VALUES ('144_drop_dungeon_items_reward_count');
