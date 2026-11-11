-- Gold held for a recipient until collected at the Depot (#1109).
CREATE TABLE IF NOT EXISTS character_pending_remittances (
    id CHAR(32) NOT NULL PRIMARY KEY,
    character_id CHAR(32) NOT NULL,
    amount INT NOT NULL,
    source VARCHAR(32) NOT NULL,
    sender_name VARCHAR(32) NOT NULL,
    created_at DATETIME(6) NOT NULL,
    CONSTRAINT fk_pending_remittances_character
        FOREIGN KEY (character_id) REFERENCES characters (id) ON DELETE CASCADE,
    CONSTRAINT chk_pending_remittances_amount CHECK (amount > 0),
    INDEX idx_pending_remittances_character (character_id, created_at, id)
);

INSERT IGNORE INTO schema_migrations (version) VALUES ('125_character_pending_remittances');
