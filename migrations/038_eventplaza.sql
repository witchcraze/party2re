-- Migration 038: Event Plaza celebration banquets and toasts
CREATE TABLE IF NOT EXISTS celebration_banquets (
    id CHAR(32) NOT NULL PRIMARY KEY,
    boss_id VARCHAR(64) NOT NULL,
    boss_name VARCHAR(128) NOT NULL,
    slayer_character_id CHAR(32) NOT NULL,
    slayer_character_name VARCHAR(128) NOT NULL,
    tier INT NOT NULL DEFAULT 1,
    toast_count INT NOT NULL DEFAULT 0,
    celebrated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at TIMESTAMP NOT NULL,
    INDEX idx_banquets_expires (expires_at),
    INDEX idx_banquets_celebrated (celebrated_at DESC)
);

CREATE TABLE IF NOT EXISTS banquet_toasts (
    banquet_id CHAR(32) NOT NULL,
    character_id CHAR(32) NOT NULL,
    toasted_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (banquet_id, character_id),
    CONSTRAINT fk_banquet_toasts_banquet FOREIGN KEY (banquet_id) REFERENCES celebration_banquets (id) ON DELETE CASCADE,
    CONSTRAINT fk_banquet_toasts_character FOREIGN KEY (character_id) REFERENCES characters (id) ON DELETE CASCADE
);

INSERT IGNORE INTO schema_migrations (version) VALUES ('038_eventplaza');
