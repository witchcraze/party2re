-- Receipts for Dungeon defeat credits committed during a run; FinalizeExpedition removes a run's rows.
CREATE TABLE IF NOT EXISTS dungeon_defeat_credits (
    run_id VARCHAR(64) NOT NULL,
    credit_id VARCHAR(128) NOT NULL,
    character_id CHAR(32) NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (run_id, credit_id),
    CONSTRAINT fk_dungeon_defeat_credits_character
        FOREIGN KEY (character_id) REFERENCES characters (id) ON DELETE CASCADE
);

INSERT IGNORE INTO schema_migrations (version) VALUES ('145_dungeon_defeat_credits');
