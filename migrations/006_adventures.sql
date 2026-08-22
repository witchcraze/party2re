CREATE TABLE IF NOT EXISTS adventures (
    id CHAR(32) NOT NULL PRIMARY KEY,
    character_id CHAR(32) NOT NULL,
    adventure_type VARCHAR(64) NOT NULL,
    started_at DATETIME(6) NOT NULL,
    available_at DATETIME(6) NOT NULL,
    experience_reward INT NOT NULL,
    outcome VARCHAR(16) NULL,
    winner_id CHAR(32) NULL,
    loser_id CHAR(32) NULL,
    battle_turns INT NULL,
    resolved BOOLEAN NOT NULL DEFAULT FALSE,
    claimed BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT fk_adventures_character
        FOREIGN KEY (character_id) REFERENCES characters (id)
);

INSERT IGNORE INTO schema_migrations (version) VALUES ('006_adventures');
