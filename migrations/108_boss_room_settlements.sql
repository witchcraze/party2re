CREATE TABLE IF NOT EXISTS boss_room_settlements (
    room_id VARCHAR(64) NOT NULL PRIMARY KEY,
    settlement_json JSON NULL
) ENGINE=InnoDB;

INSERT IGNORE INTO schema_migrations (version) VALUES ('108_boss_room_settlements');
