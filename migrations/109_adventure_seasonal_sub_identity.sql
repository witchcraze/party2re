-- Migration 109: Adventure seasonal sub identity (Issue #1222)
ALTER TABLE adventures
    ADD COLUMN seasonal_sub_id INT NOT NULL DEFAULT 0 AFTER is_cleared,
    ADD COLUMN season VARCHAR(32) NOT NULL DEFAULT '' AFTER seasonal_sub_id,
    ADD INDEX idx_adventures_character_stage_season (character_id, resolved, adventure_type, seasonal_sub_id);

INSERT IGNORE INTO schema_migrations (version) VALUES ('109_adventure_seasonal_sub_identity');
