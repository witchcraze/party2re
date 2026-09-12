-- Migration: 077_challenge_hall_of_fame_and_dungeon_party.sql
-- Description: Support party multiplayer exploration, map scouting, and Hall of Fame records for dungeon and challenge (Issue #483)

ALTER TABLE dungeon_active_expeditions
    ADD COLUMN IF NOT EXISTS party_id VARCHAR(64) NULL DEFAULT NULL AFTER character_id,
    ADD COLUMN IF NOT EXISTS members_json JSON NULL DEFAULT NULL AFTER accumulated_items_json;

ALTER TABLE challenge_sessions
    ADD COLUMN IF NOT EXISTS party_id VARCHAR(64) NULL DEFAULT NULL AFTER character_id,
    ADD COLUMN IF NOT EXISTS party_name VARCHAR(64) NULL DEFAULT NULL AFTER party_id,
    ADD COLUMN IF NOT EXISTS party_color VARCHAR(32) NOT NULL DEFAULT '#FFFFFF' AFTER party_name,
    ADD COLUMN IF NOT EXISTS members_json JSON NULL DEFAULT NULL AFTER accumulated_items_json;

CREATE TABLE IF NOT EXISTS challenge_hall_of_fame (
    tier_id VARCHAR(32) NOT NULL PRIMARY KEY,
    highest_round INT NOT NULL DEFAULT 0,
    party_name VARCHAR(64) NOT NULL,
    party_color VARCHAR(32) NOT NULL DEFAULT '#FFFFFF',
    cleared_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    members_json JSON NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
);

INSERT IGNORE INTO schema_migrations (version) VALUES ('077_challenge_hall_of_fame_and_dungeon_party');
