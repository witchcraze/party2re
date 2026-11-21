-- 139_remove_non_legacy_rankings.sql: Remove stale non-legacy ranking snapshots and unused ranking indexes (Issue #2360)

DELETE FROM ranking_snapshots
WHERE ranking_type IN (
    'level',
    'player_wealth',
    'character_wealth',
    'boss_defeat',
    'adventure_victory',
    'job_mastery',
    'small_medals'
);

DROP INDEX IF EXISTS idx_characters_level_exp ON characters;
DROP INDEX IF EXISTS idx_characters_money ON characters;
DROP INDEX IF EXISTS idx_adventures_char_outcome ON adventures;

INSERT IGNORE INTO schema_migrations (version) VALUES ('139_remove_non_legacy_rankings');
