-- Character-owned login message, independent sampled-save clock (UTC epoch seconds)
-- and at most one entry and one sample public presence snapshot per character.
ALTER TABLE characters
    ADD COLUMN IF NOT EXISTS login_message VARCHAR(60) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS presence_sampled_at BIGINT NULL DEFAULT NULL;

-- seq is reassigned by every REPLACE, so it records publication order for equal timestamps.
-- Display fields are an immutable copy taken at publication.
CREATE TABLE IF NOT EXISTS character_presence_snapshots (
    seq BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    character_id CHAR(32) NOT NULL,
    kind VARCHAR(8) NOT NULL,
    recorded_at BIGINT NOT NULL,
    name VARCHAR(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
    color VARCHAR(7) NOT NULL,
    guild_id CHAR(32) NOT NULL DEFAULT '',
    guild_name VARCHAR(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL DEFAULT '',
    icon_url VARCHAR(512) NOT NULL DEFAULT '',
    aura_effect INT NOT NULL DEFAULT 0,
    login_message VARCHAR(60) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL DEFAULT '',
    UNIQUE KEY uq_presence_character_kind (character_id, kind),
    INDEX idx_presence_recorded (recorded_at),
    CONSTRAINT fk_presence_character FOREIGN KEY (character_id) REFERENCES characters (id) ON DELETE CASCADE
) ENGINE=InnoDB;

INSERT IGNORE INTO schema_migrations (version) VALUES ('116_character_presence');
