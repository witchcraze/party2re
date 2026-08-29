-- Migration 037: Adventure history, chronicle and performance indices
ALTER TABLE adventures
    ADD INDEX idx_adventures_character_started (character_id, started_at DESC),
    ADD INDEX idx_adventures_character_claimed (character_id, claimed, outcome);

INSERT IGNORE INTO schema_migrations (version) VALUES ('037_adventure_chronicle');
