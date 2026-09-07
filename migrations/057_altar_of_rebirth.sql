-- 057_altar_of_rebirth.sql: Add orb to characters and create altar_ramia_awakenings table
-- 1. Add orb column to characters table
-- 2. Create altar_ramia_awakenings table for managing Ramia presence at Altar of Rebirth

ALTER TABLE characters ADD COLUMN IF NOT EXISTS orb VARCHAR(16) NOT NULL DEFAULT '';

CREATE TABLE IF NOT EXISTS altar_ramia_awakenings (
    id VARCHAR(64) NOT NULL PRIMARY KEY,
    character_id VARCHAR(64) NOT NULL,
    character_name VARCHAR(64) NOT NULL,
    awakened_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at TIMESTAMP NOT NULL,
    INDEX idx_altar_expires (expires_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

INSERT IGNORE INTO schema_migrations (version) VALUES ('057_altar_of_rebirth');
