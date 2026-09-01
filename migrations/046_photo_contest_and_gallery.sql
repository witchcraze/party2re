-- 046_photo_contest_and_gallery.sql
-- Photo Contest, Character Screenshots / Gallery, Voting and Hall of Fame (photo.cgi / contest.cgi)

CREATE TABLE IF NOT EXISTS character_photos (
    id CHAR(32) NOT NULL PRIMARY KEY,
    character_id CHAR(32) NOT NULL,
    title VARCHAR(128) NOT NULL,
    location VARCHAR(64) NOT NULL DEFAULT '',
    image_url VARCHAR(255) NOT NULL DEFAULT '',
    caption TEXT NOT NULL,
    metadata TEXT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    INDEX idx_character_photos_char (character_id, created_at DESC),
    CONSTRAINT fk_char_photos_character FOREIGN KEY (character_id) REFERENCES characters (id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS contest_rounds (
    round INT NOT NULL PRIMARY KEY,
    status VARCHAR(32) NOT NULL DEFAULT 'preparing',
    start_time TIMESTAMP NOT NULL,
    end_time TIMESTAMP NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    INDEX idx_contest_rounds_status (status)
);

CREATE TABLE IF NOT EXISTS contest_entries (
    id CHAR(32) NOT NULL PRIMARY KEY,
    round INT NOT NULL,
    character_id CHAR(32) NOT NULL,
    character_name VARCHAR(64) NOT NULL,
    guild_name VARCHAR(64) NOT NULL DEFAULT '',
    title VARCHAR(128) NOT NULL,
    photo_id CHAR(32) NOT NULL,
    image_url VARCHAR(255) NOT NULL DEFAULT '',
    caption TEXT NOT NULL,
    votes INT NOT NULL DEFAULT 0,
    ranking INT NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    UNIQUE KEY uq_contest_entries_round_char (round, character_id),
    UNIQUE KEY uq_contest_entries_round_title (round, title),
    INDEX idx_contest_entries_round_votes (round, votes DESC, created_at ASC),
    CONSTRAINT fk_contest_entries_character FOREIGN KEY (character_id) REFERENCES characters (id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS contest_votes (
    id CHAR(32) NOT NULL PRIMARY KEY,
    round INT NOT NULL,
    entry_id CHAR(32) NOT NULL,
    voter_character_id CHAR(32) NOT NULL,
    voter_character_name VARCHAR(64) NOT NULL,
    comment VARCHAR(255) NOT NULL DEFAULT '',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE KEY uq_contest_votes_round_voter (round, voter_character_id),
    INDEX idx_contest_votes_entry (entry_id),
    CONSTRAINT fk_contest_votes_entry FOREIGN KEY (entry_id) REFERENCES contest_entries (id) ON DELETE CASCADE,
    CONSTRAINT fk_contest_votes_voter FOREIGN KEY (voter_character_id) REFERENCES characters (id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS contest_legends (
    round INT NOT NULL PRIMARY KEY,
    entry_id CHAR(32) NOT NULL,
    title VARCHAR(128) NOT NULL,
    character_id CHAR(32) NOT NULL,
    character_name VARCHAR(64) NOT NULL,
    guild_name VARCHAR(64) NOT NULL DEFAULT '',
    votes INT NOT NULL,
    image_url VARCHAR(255) NOT NULL DEFAULT '',
    caption TEXT NOT NULL,
    settled_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_contest_legends_char (character_id)
);

INSERT IGNORE INTO schema_migrations (version) VALUES ('046_photo_contest_and_gallery');
