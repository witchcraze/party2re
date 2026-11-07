-- 112_alchemy_sleep_completion.sql: Legacy completes a synthesis only through home sleep (lib/home.cgi:191), so the midnight deadline is removed.
ALTER TABLE character_alchemy DROP COLUMN IF EXISTS matures_at;

INSERT IGNORE INTO schema_migrations (version) VALUES ('112_alchemy_sleep_completion');
