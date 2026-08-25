CREATE TABLE IF NOT EXISTS character_challenge_records (
    character_id VARCHAR(64) NOT NULL,
    tier_id VARCHAR(32) NOT NULL,
    highest_round INT NOT NULL DEFAULT 0,
    total_attempts INT NOT NULL DEFAULT 0,
    total_victories INT NOT NULL DEFAULT 0,
    best_cleared_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (character_id, tier_id),
    INDEX idx_challenge_leaderboard (tier_id, highest_round DESC, best_cleared_at ASC)
);

CREATE TABLE IF NOT EXISTS challenge_sessions (
    id VARCHAR(64) NOT NULL PRIMARY KEY,
    character_id VARCHAR(64) NOT NULL,
    tier_id VARCHAR(32) NOT NULL,
    current_round INT NOT NULL DEFAULT 1,
    character_current_hp INT NOT NULL DEFAULT 0,
    accumulated_exp INT NOT NULL DEFAULT 0,
    accumulated_gold INT NOT NULL DEFAULT 0,
    accumulated_items_json JSON NOT NULL,
    status VARCHAR(16) NOT NULL DEFAULT 'active',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    INDEX idx_challenge_session_char (character_id, status)
);

INSERT IGNORE INTO schema_migrations (version) VALUES ('028_endurance_challenge');
