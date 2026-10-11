-- Preserve historical payloads without guessing archive recipients or publication.
ALTER TABLE battle_replays
    ADD COLUMN provenance VARCHAR(32) NOT NULL DEFAULT '',
    ADD COLUMN archive_recipient_id VARCHAR(64) NOT NULL DEFAULT '',
    ADD INDEX idx_replays_recipient (archive_recipient_id, provenance, created_at, id);

INSERT IGNORE INTO schema_migrations (version) VALUES ('102_replay_archive_provenance');
