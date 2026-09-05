CREATE TABLE IF NOT EXISTS casino_poker_sessions (
    id VARCHAR(64) NOT NULL PRIMARY KEY,
    character_id CHAR(32) NOT NULL,
    base_rate BIGINT NOT NULL,
    max_rounds INT NOT NULL DEFAULT 5,
    current_round INT NOT NULL DEFAULT 1,
    current_bet BIGINT NOT NULL,
    player_card_suit VARCHAR(16) NOT NULL,
    player_card_rank INT NOT NULL,
    dealer_card_suit VARCHAR(16) NOT NULL,
    dealer_card_rank INT NOT NULL,
    player_committed_coins BIGINT NOT NULL,
    dealer_committed_coins BIGINT NOT NULL,
    pot BIGINT NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'in_progress',
    winner VARCHAR(32) NOT NULL DEFAULT '',
    payout_coins BIGINT NOT NULL DEFAULT 0,
    logs_json JSON NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    CONSTRAINT fk_casino_poker_sessions_character
        FOREIGN KEY (character_id) REFERENCES characters (id) ON DELETE CASCADE,
    INDEX idx_casino_poker_char_status (character_id, status)
);

INSERT IGNORE INTO schema_migrations (version) VALUES ('054_casino_poker_sessions');
