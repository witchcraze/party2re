-- Migration 080: Event Plaza real-time presence tracking
CREATE TABLE IF NOT EXISTS eventplaza_presences (
    character_id CHAR(32) NOT NULL PRIMARY KEY,
    last_seen_at TIMESTAMP NOT NULL,
    INDEX idx_eventplaza_presences_seen (last_seen_at),
    CONSTRAINT fk_eventplaza_presences_character FOREIGN KEY (character_id) REFERENCES characters (id) ON DELETE CASCADE
);

INSERT IGNORE INTO schema_migrations (version) VALUES ('080_eventplaza_presences');
