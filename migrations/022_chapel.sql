CREATE TABLE IF NOT EXISTS character_blessings (
    character_id CHAR(32) NOT NULL PRIMARY KEY,
    active_blessing VARCHAR(32) NOT NULL DEFAULT 'NONE',
    donation_gold_total BIGINT NOT NULL DEFAULT 0,
    prayed_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    CONSTRAINT fk_character_blessings_char FOREIGN KEY (character_id) REFERENCES characters (id) ON DELETE CASCADE,
    CONSTRAINT chk_character_blessings_donation CHECK (donation_gold_total >= 0)
);

INSERT IGNORE INTO schema_migrations (version) VALUES ('022_chapel');
