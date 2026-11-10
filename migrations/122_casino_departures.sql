-- Only pending departure publication reserves roster members; latest receipts survive room loss.
CREATE TABLE IF NOT EXISTS casino_departures (
    room_id VARCHAR(36) NOT NULL PRIMARY KEY,
    version INT NOT NULL DEFAULT 0,
    pending BOOLEAN NOT NULL DEFAULT FALSE,
    departure_json JSON NULL
) ENGINE=InnoDB;

CREATE TABLE IF NOT EXISTS casino_departure_claims (
    character_id VARCHAR(36) NOT NULL PRIMARY KEY,
    room_id VARCHAR(36) NOT NULL,
    INDEX idx_casino_departure_room (room_id)
) ENGINE=InnoDB;

INSERT IGNORE INTO schema_migrations (version) VALUES ('122_casino_departures');
