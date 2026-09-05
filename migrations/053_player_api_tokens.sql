-- Migration: 053_player_api_tokens.sql
-- Description: Create player_api_tokens table for Personal Access Tokens (API Keys) (Issue #163)

CREATE TABLE IF NOT EXISTS player_api_tokens (
    id VARCHAR(64) PRIMARY KEY,
    player_id VARCHAR(64) NOT NULL,
    token_hash VARCHAR(64) NOT NULL UNIQUE,
    name VARCHAR(64) NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_used_at TIMESTAMP NULL,
    expires_at TIMESTAMP NULL,
    CONSTRAINT fk_player_api_tokens_player
        FOREIGN KEY (player_id) REFERENCES players (id) ON DELETE CASCADE,
    INDEX idx_player_api_tokens_player (player_id, created_at DESC)
);

INSERT IGNORE INTO schema_migrations (version) VALUES ('053_player_api_tokens');
