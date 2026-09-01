-- Migration 048: Multiplayer Co-op Party System and Group Quests (quest.cgi, party.cgi)

CREATE TABLE IF NOT EXISTS parties (
    id VARCHAR(64) NOT NULL PRIMARY KEY,
    leader_character_id CHAR(32) NOT NULL,
    name VARCHAR(64) NOT NULL,
    password_hash VARCHAR(255) NULL,
    stage_id VARCHAR(64) NOT NULL,
    speed INT NOT NULL DEFAULT 3,
    max_members INT NOT NULL DEFAULT 4,
    min_level INT NOT NULL DEFAULT 1,
    max_level INT NOT NULL DEFAULT 999,
    min_hp INT NOT NULL DEFAULT 0,
    status VARCHAR(32) NOT NULL DEFAULT 'recruiting',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    INDEX idx_parties_status_created (status, created_at),
    INDEX idx_parties_leader (leader_character_id),
    CONSTRAINT fk_parties_leader FOREIGN KEY (leader_character_id) REFERENCES characters(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS party_members (
    party_id VARCHAR(64) NOT NULL,
    character_id CHAR(32) NOT NULL,
    is_leader BOOLEAN NOT NULL DEFAULT FALSE,
    ready_state BOOLEAN NOT NULL DEFAULT FALSE,
    joined_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (party_id, character_id),
    UNIQUE KEY uk_party_members_character (character_id),
    INDEX idx_party_members_party (party_id),
    CONSTRAINT fk_party_members_party FOREIGN KEY (party_id) REFERENCES parties(id) ON DELETE CASCADE,
    CONSTRAINT fk_party_members_character FOREIGN KEY (character_id) REFERENCES characters(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS party_adventure_logs (
    id VARCHAR(64) NOT NULL PRIMARY KEY,
    party_id VARCHAR(64) NOT NULL,
    stage_id VARCHAR(64) NOT NULL,
    outcome VARCHAR(32) NOT NULL,
    turns INT NOT NULL DEFAULT 0,
    total_exp INT NOT NULL DEFAULT 0,
    total_gold INT NOT NULL DEFAULT 0,
    synergy_bonus_percent INT NOT NULL DEFAULT 0,
    details_json JSON NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_party_adventure_logs_party (party_id, created_at)
);

INSERT IGNORE INTO schema_migrations (version) VALUES ('048_party_and_coop_quests');
