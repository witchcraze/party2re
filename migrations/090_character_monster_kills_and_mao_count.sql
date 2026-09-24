-- Migration: 090_character_monster_kills_and_mao_count.sql
-- Description: Add monster_kills and mao_count to characters table (Issue #784)

ALTER TABLE characters
    ADD COLUMN IF NOT EXISTS monster_kills INT NOT NULL DEFAULT 0 AFTER casino_wins,
    ADD COLUMN IF NOT EXISTS mao_count INT NOT NULL DEFAULT 0 AFTER monster_kills;

INSERT IGNORE INTO schema_migrations (version) VALUES ('090_character_monster_kills_and_mao_count');
