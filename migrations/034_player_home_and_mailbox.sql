CREATE TABLE IF NOT EXISTS character_homes (
    character_id VARCHAR(32) NOT NULL PRIMARY KEY,
    theme VARCHAR(32) NOT NULL DEFAULT '#ffffff',
    motto VARCHAR(255) NOT NULL DEFAULT '',
    companion_name VARCHAR(64) NOT NULL DEFAULT 'ペット',
    visitor_count INT NOT NULL DEFAULT 0,
    last_visited_at DATETIME(6) NULL,
    updated_at DATETIME(6) NOT NULL,
    CONSTRAINT fk_character_homes_char FOREIGN KEY (character_id) REFERENCES characters(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS character_letters (
    id VARCHAR(32) NOT NULL PRIMARY KEY,
    sender_character_id VARCHAR(32) NOT NULL,
    sender_name VARCHAR(64) NOT NULL,
    recipient_character_id VARCHAR(32) NOT NULL,
    recipient_name VARCHAR(64) NOT NULL,
    content VARCHAR(1000) NOT NULL,
    color VARCHAR(32) NOT NULL DEFAULT '#000000',
    is_read BOOLEAN NOT NULL DEFAULT FALSE,
    read_at DATETIME(6) NULL,
    created_at DATETIME(6) NOT NULL,
    CONSTRAINT fk_character_letters_sender FOREIGN KEY (sender_character_id) REFERENCES characters(id) ON DELETE CASCADE,
    CONSTRAINT fk_character_letters_recipient FOREIGN KEY (recipient_character_id) REFERENCES characters(id) ON DELETE CASCADE,
    INDEX idx_character_letters_recipient_unread (recipient_character_id, is_read, created_at DESC),
    INDEX idx_character_letters_recipient_created (recipient_character_id, created_at DESC),
    INDEX idx_character_letters_sender_created (sender_character_id, created_at DESC)
);

CREATE TABLE IF NOT EXISTS character_companion_phrases (
    id VARCHAR(32) NOT NULL PRIMARY KEY,
    character_id VARCHAR(32) NOT NULL,
    phrase VARCHAR(200) NOT NULL,
    created_at DATETIME(6) NOT NULL,
    CONSTRAINT fk_companion_phrases_char FOREIGN KEY (character_id) REFERENCES characters(id) ON DELETE CASCADE,
    INDEX idx_companion_phrases_char (character_id, created_at ASC)
);

CREATE TABLE IF NOT EXISTS character_delivery_notices (
    id VARCHAR(32) NOT NULL PRIMARY KEY,
    character_id VARCHAR(32) NOT NULL,
    notice_type VARCHAR(32) NOT NULL DEFAULT 'item_transfer',
    message VARCHAR(500) NOT NULL,
    is_cleared BOOLEAN NOT NULL DEFAULT FALSE,
    created_at DATETIME(6) NOT NULL,
    CONSTRAINT fk_delivery_notices_char FOREIGN KEY (character_id) REFERENCES characters(id) ON DELETE CASCADE,
    INDEX idx_delivery_notices_char_cleared (character_id, is_cleared, created_at DESC)
);

INSERT IGNORE INTO schema_migrations (version) VALUES ('034_player_home_and_mailbox');
