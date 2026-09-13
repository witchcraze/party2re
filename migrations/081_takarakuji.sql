DROP TABLE IF EXISTS lottery_tickets;
DROP TABLE IF EXISTS lottery_drawings;

CREATE TABLE IF NOT EXISTS takarakuji_rounds (
    round_id INT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    draw_date DATETIME(6) NOT NULL,
    is_drawn BOOLEAN NOT NULL DEFAULT FALSE,
    drawn_at DATETIME(6) NULL,
    prize_1_item_id VARCHAR(64) NOT NULL,
    prize_1_amount INT NOT NULL DEFAULT 1,
    prize_2_item_id VARCHAR(64) NOT NULL,
    prize_2_amount INT NOT NULL DEFAULT 1,
    prize_3_item_id VARCHAR(64) NOT NULL,
    prize_3_amount INT NOT NULL DEFAULT 2,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    INDEX idx_takarakuji_rounds_draw_date (draw_date, is_drawn)
);

CREATE TABLE IF NOT EXISTS takarakuji_tickets (
    id CHAR(32) NOT NULL PRIMARY KEY,
    round_id INT NOT NULL,
    character_id CHAR(32) NOT NULL,
    purchased_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    won_rank INT NOT NULL DEFAULT 0,
    won_item_id VARCHAR(64) NULL,
    CONSTRAINT fk_takarakuji_tickets_round FOREIGN KEY (round_id) REFERENCES takarakuji_rounds (round_id) ON DELETE CASCADE,
    CONSTRAINT fk_takarakuji_tickets_char FOREIGN KEY (character_id) REFERENCES characters (id) ON DELETE CASCADE,
    CONSTRAINT uq_takarakuji_round_char UNIQUE (round_id, character_id)
);

CREATE INDEX idx_takarakuji_tickets_round ON takarakuji_tickets (round_id);
CREATE INDEX idx_takarakuji_tickets_char ON takarakuji_tickets (character_id);

INSERT IGNORE INTO schema_migrations (version) VALUES ('081_takarakuji');
