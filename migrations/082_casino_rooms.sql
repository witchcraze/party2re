CREATE TABLE IF NOT EXISTS casino_rooms (
    id VARCHAR(64) NOT NULL PRIMARY KEY,
    name VARCHAR(50) NOT NULL UNIQUE,
    game_type VARCHAR(32) NOT NULL,
    leader_character_id CHAR(32) NOT NULL,
    speed INT NOT NULL DEFAULT 18,
    max_players INT NOT NULL DEFAULT 8,
    rate BIGINT NOT NULL,
    password_hash VARCHAR(255) NOT NULL DEFAULT '',
    has_password BOOLEAN NOT NULL DEFAULT FALSE,
    allow_spectators BOOLEAN NOT NULL DEFAULT TRUE,
    status VARCHAR(32) NOT NULL DEFAULT 'waiting',
    round INT NOT NULL DEFAULT 0,
    current_bet BIGINT NOT NULL DEFAULT 0,
    max_bet BIGINT NOT NULL DEFAULT 0,
    pot BIGINT NOT NULL DEFAULT 0,
    winner_character_id CHAR(32) NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    CONSTRAINT fk_casino_rooms_leader
        FOREIGN KEY (leader_character_id) REFERENCES characters (id) ON DELETE CASCADE,
    INDEX idx_casino_rooms_status (status)
);

CREATE TABLE IF NOT EXISTS casino_members (
    room_id VARCHAR(64) NOT NULL,
    character_id CHAR(32) NOT NULL,
    is_spectator BOOLEAN NOT NULL DEFAULT FALSE,
    action VARCHAR(32) NOT NULL DEFAULT '',
    card INT NOT NULL DEFAULT -1,
    joined_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (room_id, character_id),
    CONSTRAINT fk_casino_members_room
        FOREIGN KEY (room_id) REFERENCES casino_rooms (id) ON DELETE CASCADE,
    CONSTRAINT fk_casino_members_character
        FOREIGN KEY (character_id) REFERENCES characters (id) ON DELETE CASCADE,
    INDEX idx_casino_members_character (character_id)
);

DROP TABLE IF EXISTS casino_poker_sessions;

INSERT IGNORE INTO schema_migrations (version) VALUES ('082_casino_rooms');
