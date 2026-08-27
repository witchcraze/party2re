ALTER TABLE characters ADD COLUMN help_count INT NOT NULL DEFAULT 0;

CREATE TABLE IF NOT EXISTS helper_quests (
    id VARCHAR(36) NOT NULL PRIMARY KEY,
    title VARCHAR(64) NOT NULL,
    kind TINYINT NOT NULL,
    target_id VARCHAR(64) NOT NULL,
    target_name VARCHAR(64) NOT NULL,
    required_count INT NOT NULL,
    reward_item_id VARCHAR(64) NOT NULL,
    is_rare BOOLEAN NOT NULL DEFAULT FALSE,
    is_guild BOOLEAN NOT NULL DEFAULT FALSE,
    expires_at TIMESTAMP NOT NULL,
    completed_at TIMESTAMP NULL,
    completed_by VARCHAR(32) NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_helper_quests_expires (expires_at),
    INDEX idx_helper_quests_completed (completed_at)
);

CREATE TABLE IF NOT EXISTS rescue_records (
    id VARCHAR(36) NOT NULL PRIMARY KEY,
    character_id VARCHAR(32) NOT NULL,
    reason VARCHAR(128) NOT NULL,
    penalty_seconds INT NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_rescue_records_char (character_id)
);

INSERT IGNORE INTO schema_migrations (version) VALUES ('031_rescue_and_helper');
