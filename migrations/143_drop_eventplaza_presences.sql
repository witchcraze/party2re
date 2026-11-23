-- Live Event Plaza presence is owned by the Valkey tracker.
DROP TABLE IF EXISTS eventplaza_presences;

INSERT IGNORE INTO schema_migrations (version) VALUES ('143_drop_eventplaza_presences');
