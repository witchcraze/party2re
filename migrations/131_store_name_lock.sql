-- Serializes construction and renaming across towns, including claims of expired names.
CREATE TABLE IF NOT EXISTS store_name_lock (
    id TINYINT NOT NULL PRIMARY KEY
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
INSERT IGNORE INTO store_name_lock (id) VALUES (1);

INSERT IGNORE INTO schema_migrations (version) VALUES ('131_store_name_lock');
