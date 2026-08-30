-- Delivery Quests, Character Deliveries, and Parcel Courier Service
CREATE TABLE IF NOT EXISTS delivery_quests (
    id CHAR(32) NOT NULL PRIMARY KEY,
    client_name VARCHAR(64) NOT NULL,
    client_message TEXT NOT NULL,
    target_item_id VARCHAR(64) NOT NULL,
    target_item_name VARCHAR(64) NOT NULL,
    required_quantity INT NOT NULL DEFAULT 1,
    recipient_name VARCHAR(64) NOT NULL,
    destination VARCHAR(64) NOT NULL,
    reward_gold INT NOT NULL DEFAULT 0,
    reward_exp INT NOT NULL DEFAULT 0,
    reward_item_id VARCHAR(64) NOT NULL DEFAULT '',
    expires_at TIMESTAMP NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_delivery_quests_expires (expires_at)
);

CREATE TABLE IF NOT EXISTS character_deliveries (
    id CHAR(32) NOT NULL PRIMARY KEY,
    character_id CHAR(32) NOT NULL,
    quest_id CHAR(32) NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'in_progress',
    accepted_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    completed_at TIMESTAMP NULL,
    INDEX idx_char_deliveries_char (character_id, status),
    INDEX idx_char_deliveries_quest (quest_id),
    CONSTRAINT fk_char_deliveries_character FOREIGN KEY (character_id) REFERENCES characters (id) ON DELETE CASCADE,
    CONSTRAINT fk_char_deliveries_quest FOREIGN KEY (quest_id) REFERENCES delivery_quests (id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS delivery_parcels (
    id CHAR(32) NOT NULL PRIMARY KEY,
    sender_character_id CHAR(32) NOT NULL,
    sender_character_name VARCHAR(64) NOT NULL,
    recipient_character_id CHAR(32) NOT NULL,
    item_id VARCHAR(64) NOT NULL DEFAULT '',
    item_name VARCHAR(64) NOT NULL DEFAULT '',
    item_quantity INT NOT NULL DEFAULT 0,
    gold_amount INT NOT NULL DEFAULT 0,
    message VARCHAR(255) NOT NULL DEFAULT '',
    courier_fee INT NOT NULL DEFAULT 50,
    status VARCHAR(32) NOT NULL DEFAULT 'pending',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    claimed_at TIMESTAMP NULL,
    INDEX idx_delivery_parcels_recipient (recipient_character_id, status),
    INDEX idx_delivery_parcels_sender (sender_character_id, status),
    CONSTRAINT fk_delivery_parcels_sender FOREIGN KEY (sender_character_id) REFERENCES characters (id) ON DELETE CASCADE,
    CONSTRAINT fk_delivery_parcels_recipient FOREIGN KEY (recipient_character_id) REFERENCES characters (id) ON DELETE CASCADE
);

INSERT IGNORE INTO schema_migrations (version) VALUES ('041_delivery');
