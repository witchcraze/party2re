CREATE TABLE IF NOT EXISTS players (
    id VARCHAR(32) NOT NULL PRIMARY KEY,
    username VARCHAR(64) NOT NULL UNIQUE,
    password_hash VARCHAR(255) NOT NULL,
    created_at DATETIME(6) NOT NULL
);

CREATE TABLE IF NOT EXISTS player_sessions (
    id VARCHAR(32) NOT NULL PRIMARY KEY,
    player_id VARCHAR(32) NOT NULL,
    created_at DATETIME(6) NOT NULL,
    expires_at DATETIME(6) NOT NULL,
    revoked_at DATETIME(6) NULL,
    CONSTRAINT fk_player_sessions_player FOREIGN KEY (player_id) REFERENCES players(id)
);

INSERT IGNORE INTO schema_migrations (version) VALUES ('010_players_sessions');
