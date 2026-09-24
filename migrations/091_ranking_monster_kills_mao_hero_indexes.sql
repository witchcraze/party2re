-- Migration: 091_ranking_monster_kills_mao_hero_indexes.sql
-- Description: Add indexes for monster_kills, mao_count, and hero_count ranking queries (Issue #825)

CREATE INDEX idx_characters_monster_kills ON characters (monster_kills DESC, level DESC, id ASC);
CREATE INDEX idx_characters_mao_count ON characters (mao_count DESC, level DESC, id ASC);
CREATE INDEX idx_characters_hero_count ON characters (hero_count DESC, level DESC, id ASC);

INSERT IGNORE INTO schema_migrations (version) VALUES ('091_ranking_monster_kills_mao_hero_indexes');
