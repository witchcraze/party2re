-- Migration: 050_achievements_and_medals.sql
-- Description: Character lifetime milestone achievements and commemorative medals

CREATE TABLE IF NOT EXISTS character_achievements (
    character_id CHAR(32) NOT NULL,
    achievement_id VARCHAR(64) NOT NULL,
    current_progress INT NOT NULL DEFAULT 0,
    is_completed BOOLEAN NOT NULL DEFAULT FALSE,
    completed_at TIMESTAMP NULL,
    is_claimed BOOLEAN NOT NULL DEFAULT FALSE,
    claimed_at TIMESTAMP NULL,
    PRIMARY KEY (character_id, achievement_id),
    CONSTRAINT fk_char_achievements FOREIGN KEY (character_id) REFERENCES characters (id) ON DELETE CASCADE
);

CREATE INDEX idx_char_achievements_char ON character_achievements (character_id);

CREATE TABLE IF NOT EXISTS character_medals (
    character_id CHAR(32) NOT NULL,
    medal_id VARCHAR(64) NOT NULL,
    medal_name VARCHAR(128) NOT NULL,
    category VARCHAR(64) NOT NULL,
    description VARCHAR(255) NOT NULL DEFAULT '',
    awarded_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (character_id, medal_id),
    CONSTRAINT fk_char_medals FOREIGN KEY (character_id) REFERENCES characters (id) ON DELETE CASCADE
);

CREATE INDEX idx_char_medals_char ON character_medals (character_id);

INSERT IGNORE INTO schema_migrations (version) VALUES ('050_achievements_and_medals');
