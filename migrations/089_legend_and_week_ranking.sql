-- Migration: 089_legend_and_week_ranking.sql
-- Description: Add legend_records, weekly_job_changes tables and ranking performance indexes (Issue #798)

CREATE TABLE IF NOT EXISTS legend_records (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    category VARCHAR(32) NOT NULL,
    character_id CHAR(32) NOT NULL,
    character_name VARCHAR(64) NOT NULL,
    guild_name VARCHAR(64) NOT NULL DEFAULT '',
    color VARCHAR(16) NOT NULL DEFAULT '',
    icon VARCHAR(64) NOT NULL DEFAULT '',
    message VARCHAR(255) NOT NULL DEFAULT '',
    inducted_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT uk_legend_category_character UNIQUE (category, character_id),
    CONSTRAINT fk_legend_records_character FOREIGN KEY (character_id) REFERENCES characters (id) ON DELETE CASCADE
);

CREATE INDEX idx_legend_records_category ON legend_records (category, inducted_at ASC);

CREATE TABLE IF NOT EXISTS weekly_job_changes (
    character_id CHAR(32) NOT NULL PRIMARY KEY,
    change_count INT NOT NULL DEFAULT 0,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    CONSTRAINT fk_weekly_job_changes_character FOREIGN KEY (character_id) REFERENCES characters (id) ON DELETE CASCADE
);

CREATE INDEX idx_weekly_job_changes_count ON weekly_job_changes (change_count DESC);

CREATE INDEX idx_characters_casino_wins ON characters (casino_wins DESC, level DESC, id ASC);
CREATE INDEX idx_character_alchemy_crafts ON character_alchemy (total_crafts DESC);

INSERT IGNORE INTO schema_migrations (version) VALUES ('089_legend_and_week_ranking');
