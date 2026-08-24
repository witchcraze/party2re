CREATE TABLE IF NOT EXISTS casino_accounts (
    character_id CHAR(32) NOT NULL PRIMARY KEY,
    coins BIGINT NOT NULL DEFAULT 0,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    CONSTRAINT fk_casino_accounts_character
        FOREIGN KEY (character_id) REFERENCES characters (id) ON DELETE CASCADE,
    CONSTRAINT chk_casino_accounts_coins CHECK (coins >= 0)
);

INSERT IGNORE INTO schema_migrations (version) VALUES ('017_casino');
