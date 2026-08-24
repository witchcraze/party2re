CREATE TABLE IF NOT EXISTS character_lottery (
    character_id CHAR(32) NOT NULL PRIMARY KEY,
    raffle_tickets INT NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    CONSTRAINT fk_character_lottery FOREIGN KEY (character_id) REFERENCES characters (id) ON DELETE CASCADE,
    CONSTRAINT chk_character_lottery_tickets CHECK (raffle_tickets >= 0)
);

CREATE TABLE IF NOT EXISTS lottery_drawings (
    round_id INT NOT NULL PRIMARY KEY,
    winning_number VARCHAR(6) NOT NULL,
    drawn_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    is_settled BOOLEAN NOT NULL DEFAULT FALSE
);

CREATE TABLE IF NOT EXISTS lottery_tickets (
    id CHAR(32) NOT NULL PRIMARY KEY,
    character_id CHAR(32) NOT NULL,
    round_id INT NOT NULL,
    ticket_number VARCHAR(6) NOT NULL,
    purchased_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    claimed BOOLEAN NOT NULL DEFAULT FALSE,
    prize_tier VARCHAR(32) NOT NULL DEFAULT 'NONE',
    prize_gold INT NOT NULL DEFAULT 0,
    claimed_at TIMESTAMP NULL,
    CONSTRAINT fk_lottery_tickets_char FOREIGN KEY (character_id) REFERENCES characters (id) ON DELETE CASCADE
);

CREATE INDEX idx_lottery_tickets_char ON lottery_tickets (character_id);
CREATE INDEX idx_lottery_tickets_round ON lottery_tickets (round_id);

INSERT IGNORE INTO schema_migrations (version) VALUES ('018_lottery');
