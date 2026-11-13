-- Remove retired floor tracking columns from active expedition fallback and settlement history tables.
ALTER TABLE dungeon_active_expeditions
    DROP COLUMN IF EXISTS current_floor;

ALTER TABLE dungeon_expedition_history
    DROP COLUMN IF EXISTS floors_reached;

INSERT IGNORE INTO schema_migrations (version) VALUES ('130_drop_dungeon_floor_columns');
