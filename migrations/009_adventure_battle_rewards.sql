ALTER TABLE adventures
    ADD COLUMN reward_experience INT NOT NULL DEFAULT 0 AFTER battle_turns,
    ADD COLUMN reward_currency INT NOT NULL DEFAULT 0 AFTER reward_experience,
    ADD COLUMN reward_item_definition_id VARCHAR(64) NULL AFTER reward_currency,
    ADD COLUMN reward_item_quantity INT NOT NULL DEFAULT 0 AFTER reward_item_definition_id;

INSERT IGNORE INTO schema_migrations (version) VALUES ('009_adventure_battle_rewards');
