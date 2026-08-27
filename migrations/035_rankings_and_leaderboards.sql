CREATE TABLE IF NOT EXISTS ranking_snapshots (
    ranking_type VARCHAR(64) NOT NULL PRIMARY KEY,
    snapshot_data MEDIUMTEXT NOT NULL,
    total_count INT NOT NULL DEFAULT 0,
    calculated_at DATETIME(6) NOT NULL,
    updated_at DATETIME(6) NOT NULL
);

CREATE INDEX idx_characters_level_exp ON characters (level DESC, experience DESC, id ASC);
CREATE INDEX idx_characters_money ON characters (money DESC, id ASC);
CREATE INDEX idx_characters_rebirth ON characters (rebirth_count DESC, level DESC, id ASC);
CREATE INDEX idx_characters_help ON characters (help_count DESC, level DESC, id ASC);
CREATE INDEX idx_adventures_char_outcome ON adventures (character_id, outcome);

INSERT IGNORE INTO schema_migrations (version) VALUES ('035_rankings_and_leaderboards');
