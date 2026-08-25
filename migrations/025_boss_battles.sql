CREATE TABLE IF NOT EXISTS character_boss_records (
    character_id CHAR(32) NOT NULL PRIMARY KEY,
    highest_tier_cleared INT NOT NULL DEFAULT 0,
    total_boss_defeats INT NOT NULL DEFAULT 0,
    first_cleared_at TIMESTAMP NULL,
    last_challenged_at TIMESTAMP NULL,
    daily_attempts_used INT NOT NULL DEFAULT 0,
    daily_attempts_reset_at DATE NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    CONSTRAINT fk_boss_records_character
        FOREIGN KEY (character_id) REFERENCES characters (id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS boss_challenge_history (
    id VARCHAR(64) NOT NULL PRIMARY KEY,
    character_id CHAR(32) NOT NULL,
    boss_id VARCHAR(64) NOT NULL,
    tier INT NOT NULL,
    outcome VARCHAR(16) NOT NULL,
    turns INT NOT NULL DEFAULT 0,
    reward_exp INT NOT NULL DEFAULT 0,
    reward_gold INT NOT NULL DEFAULT 0,
    reward_item_id VARCHAR(64) NULL,
    is_first_clear BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT fk_boss_history_character
        FOREIGN KEY (character_id) REFERENCES characters (id) ON DELETE CASCADE
);

CREATE INDEX idx_boss_history_character ON boss_challenge_history (character_id, created_at DESC);
CREATE INDEX idx_boss_history_boss_id ON boss_challenge_history (boss_id, created_at DESC);

INSERT IGNORE INTO schema_migrations (version) VALUES ('025_boss_battles');
