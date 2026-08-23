CREATE TABLE IF NOT EXISTS bank_accounts (
    player_id VARCHAR(32) NOT NULL PRIMARY KEY,
    balance BIGINT NOT NULL DEFAULT 0,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    CONSTRAINT fk_bank_accounts_player FOREIGN KEY (player_id) REFERENCES players(id)
);

CREATE TABLE IF NOT EXISTS bank_transfers (
    id VARCHAR(32) NOT NULL PRIMARY KEY,
    from_player_id VARCHAR(32) NOT NULL,
    to_player_id VARCHAR(32) NOT NULL,
    amount BIGINT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT fk_bank_transfers_from_player FOREIGN KEY (from_player_id) REFERENCES players(id),
    CONSTRAINT fk_bank_transfers_to_player FOREIGN KEY (to_player_id) REFERENCES players(id)
);

CREATE INDEX idx_bank_transfers_from ON bank_transfers(from_player_id, created_at);
CREATE INDEX idx_bank_transfers_to ON bank_transfers(to_player_id, created_at);

INSERT IGNORE INTO schema_migrations (version) VALUES ('014_bank');
