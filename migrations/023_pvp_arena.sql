CREATE TABLE IF NOT EXISTS arena_ratings (
    character_id CHAR(32) NOT NULL PRIMARY KEY,
    rating INT NOT NULL DEFAULT 1000,
    wins INT NOT NULL DEFAULT 0,
    losses INT NOT NULL DEFAULT 0,
    draws INT NOT NULL DEFAULT 0,
    last_matched_at TIMESTAMP NULL DEFAULT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    CONSTRAINT fk_arena_ratings_char FOREIGN KEY (character_id) REFERENCES characters (id) ON DELETE CASCADE,
    CONSTRAINT chk_arena_ratings_rating CHECK (rating >= 0),
    CONSTRAINT chk_arena_ratings_wins CHECK (wins >= 0),
    CONSTRAINT chk_arena_ratings_losses CHECK (losses >= 0),
    CONSTRAINT chk_arena_ratings_draws CHECK (draws >= 0)
);

CREATE TABLE IF NOT EXISTS arena_matches (
    id CHAR(32) NOT NULL PRIMARY KEY,
    attacker_id CHAR(32) NOT NULL,
    defender_id CHAR(32) NOT NULL,
    winner_id CHAR(32) NULL,
    loser_id CHAR(32) NULL,
    outcome VARCHAR(16) NOT NULL,
    turns INT NOT NULL DEFAULT 1,
    attacker_rating_before INT NOT NULL,
    attacker_rating_after INT NOT NULL,
    defender_rating_before INT NOT NULL,
    defender_rating_after INT NOT NULL,
    reward_gold INT NOT NULL DEFAULT 0,
    reward_exp INT NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT fk_arena_matches_attacker FOREIGN KEY (attacker_id) REFERENCES characters (id) ON DELETE CASCADE,
    CONSTRAINT fk_arena_matches_defender FOREIGN KEY (defender_id) REFERENCES characters (id) ON DELETE CASCADE,
    INDEX idx_arena_matches_attacker (attacker_id, created_at DESC),
    INDEX idx_arena_matches_defender (defender_id, created_at DESC)
);

INSERT IGNORE INTO schema_migrations (version) VALUES ('023_pvp_arena');
