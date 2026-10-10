-- Migration: 097_job_popularity_stats.sql
-- Description: Add job_popularity_stats table for cumulative job popularity points (Issue #1241)

CREATE TABLE IF NOT EXISTS job_popularity_stats (
    job_id VARCHAR(64) NOT NULL PRIMARY KEY,
    male_points BIGINT NOT NULL DEFAULT 0,
    female_points BIGINT NOT NULL DEFAULT 0,
    total_points BIGINT NOT NULL DEFAULT 0,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
);

CREATE INDEX idx_job_popularity_stats_total ON job_popularity_stats (total_points DESC, job_id ASC);

INSERT IGNORE INTO schema_migrations (version) VALUES ('097_job_popularity_stats');
