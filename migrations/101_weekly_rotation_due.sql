-- NULL means weekly scheduling has not yet been initialized; existing counters are preserved.
ALTER TABLE ranking_snapshots
    ADD COLUMN weekly_rotation_due DATETIME NULL DEFAULT NULL;

INSERT IGNORE INTO schema_migrations (version) VALUES ('101_weekly_rotation_due');
