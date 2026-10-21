-- Financial recovery survives lobby expiration. Claims reserve active or unpublished membership.
CREATE TABLE IF NOT EXISTS pvp_room_transitions (
    room_id VARCHAR(36) NOT NULL PRIMARY KEY,
    version INT NOT NULL DEFAULT 0,
    pending BOOLEAN NOT NULL DEFAULT FALSE,
    transition_json JSON NULL
) ENGINE=InnoDB;

CREATE TABLE IF NOT EXISTS pvp_character_claims (
    character_id VARCHAR(36) NOT NULL PRIMARY KEY,
    room_id VARCHAR(36) NOT NULL,
    INDEX idx_pvp_claim_room (room_id),
    FOREIGN KEY (character_id) REFERENCES characters(id) ON DELETE CASCADE
) ENGINE=InnoDB;

INSERT IGNORE INTO schema_migrations (version) VALUES ('106_pvp_room_transitions');
