-- The park board duplicated the shared place log: legacy park speech lives in the town hub's log.
DROP TABLE IF EXISTS park_posts;
INSERT IGNORE INTO schema_migrations (version) VALUES ('129_drop_park_posts');
