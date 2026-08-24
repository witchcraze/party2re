INSERT IGNORE INTO players (id, username, password_hash, created_at)
VALUES ('system', 'system', 'system_hash', NOW());

ALTER TABLE characters
    ADD COLUMN player_id VARCHAR(32) NOT NULL DEFAULT 'system' AFTER id;

ALTER TABLE characters
    ADD CONSTRAINT fk_characters_player FOREIGN KEY (player_id) REFERENCES players(id);

ALTER TABLE characters
    ALTER COLUMN player_id DROP DEFAULT;

INSERT IGNORE INTO schema_migrations (version) VALUES ('015_character_player_id');
