CREATE TABLE IF NOT EXISTS character_dungeon_records (
    character_id CHAR(32) NOT NULL PRIMARY KEY,
    highest_dungeon_cleared INT NOT NULL DEFAULT 0,
    total_expeditions INT NOT NULL DEFAULT 0,
    total_floors_cleared INT NOT NULL DEFAULT 0,
    total_chests_opened INT NOT NULL DEFAULT 0,
    total_monsters_slain INT NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    CONSTRAINT fk_dungeon_records_character
        FOREIGN KEY (character_id) REFERENCES characters (id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS dungeon_active_expeditions (
    id VARCHAR(64) NOT NULL PRIMARY KEY,
    character_id CHAR(32) NOT NULL UNIQUE,
    dungeon_id VARCHAR(64) NOT NULL,
    current_floor INT NOT NULL DEFAULT 1,
    pos_x INT NOT NULL DEFAULT 0,
    pos_y INT NOT NULL DEFAULT 0,
    current_hp INT NOT NULL,
    turns_remaining INT NOT NULL,
    accumulated_exp INT NOT NULL DEFAULT 0,
    accumulated_gold INT NOT NULL DEFAULT 0,
    accumulated_items_json JSON NOT NULL,
    status VARCHAR(16) NOT NULL DEFAULT 'exploring',
    started_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    CONSTRAINT fk_dungeon_active_character
        FOREIGN KEY (character_id) REFERENCES characters (id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS dungeon_expedition_history (
    id VARCHAR(64) NOT NULL PRIMARY KEY,
    character_id CHAR(32) NOT NULL,
    dungeon_id VARCHAR(64) NOT NULL,
    floors_reached INT NOT NULL DEFAULT 1,
    outcome VARCHAR(16) NOT NULL,
    exp_reward INT NOT NULL DEFAULT 0,
    gold_reward INT NOT NULL DEFAULT 0,
    items_reward_count INT NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT fk_dungeon_history_character
        FOREIGN KEY (character_id) REFERENCES characters (id) ON DELETE CASCADE
);

CREATE INDEX idx_dungeon_history_character ON dungeon_expedition_history (character_id, created_at DESC);
CREATE INDEX idx_dungeon_history_dungeon ON dungeon_expedition_history (dungeon_id, created_at DESC);

INSERT IGNORE INTO schema_migrations (version) VALUES ('026_dungeon_exploration');
