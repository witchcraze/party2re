-- Migration: 070_alchemy_overnight_depot.sql
-- Description: Create character_alchemy and character_alchemy_recipes tables for overnight depot-linked alchemy and recipe compendium (Issue #487)

CREATE TABLE IF NOT EXISTS character_alchemy (
    character_id CHAR(32) NOT NULL PRIMARY KEY,
    recipe_id VARCHAR(64) DEFAULT NULL,
    state VARCHAR(16) NOT NULL DEFAULT 'none',
    started_at TIMESTAMP NULL DEFAULT NULL,
    matures_at TIMESTAMP NULL DEFAULT NULL,
    total_crafts INT NOT NULL DEFAULT 0,
    comp_alc BOOLEAN NOT NULL DEFAULT FALSE,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    CONSTRAINT fk_character_alchemy_character FOREIGN KEY (character_id) REFERENCES characters (id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS character_alchemy_recipes (
    character_id CHAR(32) NOT NULL,
    recipe_id VARCHAR(64) NOT NULL,
    is_crafted BOOLEAN NOT NULL DEFAULT FALSE,
    discovered_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    crafted_at TIMESTAMP NULL DEFAULT NULL,
    PRIMARY KEY (character_id, recipe_id),
    CONSTRAINT fk_character_alchemy_recipes_character FOREIGN KEY (character_id) REFERENCES characters (id) ON DELETE CASCADE
);

INSERT IGNORE INTO schema_migrations (version) VALUES ('070_alchemy_overnight_depot');
