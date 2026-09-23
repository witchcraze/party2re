-- Migration: 087_character_collection_completions.sql
-- Description: Track 100% completion milestones for monster book and item encyclopedia (Issue #797)

CREATE TABLE IF NOT EXISTS character_collection_completions (
    character_id CHAR(32) NOT NULL,
    kind VARCHAR(32) NOT NULL,
    completed_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (character_id, kind),
    CONSTRAINT fk_char_col_completions FOREIGN KEY (character_id) REFERENCES characters (id) ON DELETE CASCADE
);

CREATE INDEX idx_char_col_completions_char ON character_collection_completions (character_id);

INSERT IGNORE INTO schema_migrations (version) VALUES ('087_character_collection_completions');
