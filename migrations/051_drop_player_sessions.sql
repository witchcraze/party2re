-- Migration: 051_drop_player_sessions.sql
-- Description: Drop player_sessions table in favor of Valkey Master ephemeral sessions with native TTL (RFC #356, Issue #366)

DROP TABLE IF EXISTS player_sessions;

INSERT IGNORE INTO schema_migrations (version) VALUES ('051_drop_player_sessions');
