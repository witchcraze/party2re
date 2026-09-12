-- Migration: 072_plantation_plots.sql
-- Description: Create plantation_plots table for seed cultivation, overnight maturation, and depot harvest (Issue #489)

CREATE TABLE IF NOT EXISTS plantation_plots (
    character_id CHAR(32) NOT NULL PRIMARY KEY,
    seed_id VARCHAR(32) NOT NULL,
    fertilizer_id VARCHAR(32) DEFAULT NULL,
    sown_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    matures_at TIMESTAMP NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    CONSTRAINT fk_plantation_plots_character FOREIGN KEY (character_id) REFERENCES characters (id) ON DELETE CASCADE
);

INSERT IGNORE INTO schema_migrations (version) VALUES ('072_plantation_plots');
