CREATE TABLE IF NOT EXISTS battle_replays (
    id VARCHAR(64) NOT NULL PRIMARY KEY,
    combat_type VARCHAR(32) NOT NULL,
    initiator_id VARCHAR(64) NOT NULL,
    initiator_name VARCHAR(64) NOT NULL,
    opponent_id VARCHAR(64) NOT NULL,
    opponent_name VARCHAR(64) NOT NULL,
    outcome VARCHAR(16) NOT NULL,
    winner_id VARCHAR(64) NULL,
    loser_id VARCHAR(64) NULL,
    total_turns INT NOT NULL DEFAULT 0,
    initial_participants_json JSON NOT NULL,
    turn_logs_json JSON NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_replays_initiator ON battle_replays (initiator_id, created_at DESC);
CREATE INDEX idx_replays_opponent ON battle_replays (opponent_id, created_at DESC);
CREATE INDEX idx_replays_combat_type ON battle_replays (combat_type, created_at DESC);

INSERT IGNORE INTO schema_migrations (version) VALUES ('027_battle_replays');
