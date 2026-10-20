-- Durable reward identity and last committed round; independent of ephemeral room cleanup.
CREATE TABLE IF NOT EXISTS gvg_match_transitions (
    room_id VARCHAR(36) NOT NULL PRIMARY KEY,
    round_number INT NOT NULL DEFAULT 0,
    transition_json JSON NULL
) ENGINE=InnoDB;

INSERT IGNORE INTO schema_migrations (version) VALUES ('105_gvg_match_transitions');
