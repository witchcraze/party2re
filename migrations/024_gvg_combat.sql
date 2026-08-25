CREATE TABLE IF NOT EXISTS gvg_standings (
    guild_id CHAR(32) NOT NULL PRIMARY KEY,
    rating INT NOT NULL DEFAULT 1000,
    wins INT NOT NULL DEFAULT 0,
    losses INT NOT NULL DEFAULT 0,
    draws INT NOT NULL DEFAULT 0,
    victory_points BIGINT NOT NULL DEFAULT 0,
    bronze_medals INT NOT NULL DEFAULT 0,
    silver_medals INT NOT NULL DEFAULT 0,
    gold_medals INT NOT NULL DEFAULT 0,
    trophies INT NOT NULL DEFAULT 0,
    championship_cups INT NOT NULL DEFAULT 0,
    champion_cups INT NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    CONSTRAINT fk_gvg_standings_guild
        FOREIGN KEY (guild_id) REFERENCES guilds (id) ON DELETE CASCADE,
    CONSTRAINT chk_gvg_standings_rating_positive
        CHECK (rating >= 0)
);

CREATE TABLE IF NOT EXISTS gvg_matches (
    id VARCHAR(64) NOT NULL PRIMARY KEY,
    challenger_guild_id CHAR(32) NOT NULL,
    defender_guild_id CHAR(32) NOT NULL,
    winner_guild_id CHAR(32) NULL,
    challenger_score INT NOT NULL DEFAULT 0,
    defender_score INT NOT NULL DEFAULT 0,
    total_rounds INT NOT NULL DEFAULT 0,
    challenger_rating_before INT NOT NULL,
    challenger_rating_after INT NOT NULL,
    defender_rating_before INT NOT NULL,
    defender_rating_after INT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT fk_gvg_matches_challenger
        FOREIGN KEY (challenger_guild_id) REFERENCES guilds (id) ON DELETE CASCADE,
    CONSTRAINT fk_gvg_matches_defender
        FOREIGN KEY (defender_guild_id) REFERENCES guilds (id) ON DELETE CASCADE,
    CONSTRAINT fk_gvg_matches_winner
        FOREIGN KEY (winner_guild_id) REFERENCES guilds (id) ON DELETE SET NULL
);

CREATE INDEX idx_gvg_matches_challenger ON gvg_matches (challenger_guild_id, created_at DESC);
CREATE INDEX idx_gvg_matches_defender ON gvg_matches (defender_guild_id, created_at DESC);

CREATE TABLE IF NOT EXISTS gvg_match_rounds (
    id VARCHAR(64) NOT NULL PRIMARY KEY,
    match_id VARCHAR(64) NOT NULL,
    round_index INT NOT NULL,
    challenger_character_id CHAR(32) NOT NULL,
    challenger_character_name VARCHAR(64) NOT NULL,
    defender_character_id CHAR(32) NOT NULL,
    defender_character_name VARCHAR(64) NOT NULL,
    winner_character_id CHAR(32) NULL,
    turns INT NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT fk_gvg_rounds_match
        FOREIGN KEY (match_id) REFERENCES gvg_matches (id) ON DELETE CASCADE
);

CREATE INDEX idx_gvg_rounds_match_id ON gvg_match_rounds (match_id, round_index ASC);

INSERT IGNORE INTO schema_migrations (version) VALUES ('024_gvg_combat');
