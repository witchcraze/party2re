-- Serializes house construction per town so the ten-house limit holds under concurrency (#1113).
CREATE TABLE IF NOT EXISTS house_town_capacity_lock (
    town_id VARCHAR(32) NOT NULL PRIMARY KEY
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
INSERT IGNORE INTO house_town_capacity_lock (town_id) VALUES ('town1'), ('town2'), ('town3'), ('town4');

INSERT IGNORE INTO schema_migrations (version) VALUES ('124_house_town_capacity_lock');
