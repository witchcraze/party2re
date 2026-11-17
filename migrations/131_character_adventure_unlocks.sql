CREATE TABLE IF NOT EXISTS character_adventure_unlocks (
    character_id CHAR(32) NOT NULL PRIMARY KEY,
    try_mode BOOLEAN NOT NULL DEFAULT FALSE,
    calm_mode BOOLEAN NOT NULL DEFAULT FALSE,
    hard_mode BOOLEAN NOT NULL DEFAULT FALSE,
    image BOOLEAN NOT NULL DEFAULT FALSE,
    avatar BOOLEAN NOT NULL DEFAULT FALSE,
    FOREIGN KEY (character_id) REFERENCES characters(id) ON DELETE CASCADE
);

INSERT IGNORE INTO schema_migrations (version) VALUES ('131_character_adventure_unlocks');
