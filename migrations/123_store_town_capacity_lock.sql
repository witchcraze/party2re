-- Serializes store construction per town so the ten-store limit holds under concurrency (#1114).
CREATE TABLE IF NOT EXISTS store_town_capacity_lock (
    town_id VARCHAR(32) NOT NULL PRIMARY KEY
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
INSERT IGNORE INTO store_town_capacity_lock (town_id) VALUES ('town1'), ('town2'), ('town3'), ('town4');

INSERT IGNORE INTO schema_migrations (version) VALUES ('123_store_town_capacity_lock');
