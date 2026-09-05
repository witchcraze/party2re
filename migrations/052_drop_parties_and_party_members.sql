-- Migration: 052_drop_parties_and_party_members.sql
-- Description: Drop unused parties and party_members tables in favor of Valkey Master ephemeral lobbies (RFC #356, Issue #380)

DROP TABLE IF EXISTS party_members;
DROP TABLE IF EXISTS parties;

INSERT IGNORE INTO schema_migrations (version) VALUES ('052_drop_parties_and_party_members');
