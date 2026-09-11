-- Migration 067: Player Stores, Sales, and Interiors
-- Parity with legacy Party2 (store.cgi, _town.cgi):
-- 1. character_stores: town estate store tracking (town_id, store_name, house_style, wallpaper, expires_at)
-- 2. store_sales: active store item listings (gold sale or barter from depot)
-- 3. store_interiors: store furniture items (up to 5 per store, renamed)

CREATE TABLE IF NOT EXISTS character_stores (
    id VARCHAR(64) NOT NULL PRIMARY KEY,
    character_id VARCHAR(64) NOT NULL UNIQUE,
    town_id VARCHAR(32) NOT NULL,
    store_name VARCHAR(64) NOT NULL,
    house_style VARCHAR(32) NOT NULL,
    wallpaper VARCHAR(32) NOT NULL DEFAULT 'none',
    expires_at DATETIME(6) NOT NULL,
    created_at DATETIME(6) NOT NULL,
    updated_at DATETIME(6) NOT NULL,
    INDEX idx_character_stores_town_expires (town_id, expires_at),
    INDEX idx_character_stores_store_name (store_name)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS store_sales (
    id VARCHAR(64) NOT NULL PRIMARY KEY,
    store_id VARCHAR(64) NOT NULL,
    character_id VARCHAR(64) NOT NULL,
    slot_number INT NOT NULL,
    item_definition_id VARCHAR(64) NOT NULL,
    item_name VARCHAR(64) NOT NULL,
    quantity INT NOT NULL DEFAULT 1,
    enhancement_level INT NOT NULL DEFAULT 0,
    sale_type VARCHAR(16) NOT NULL,
    price INT NOT NULL DEFAULT 0,
    wish_item_name VARCHAR(64) NOT NULL DEFAULT '',
    created_at DATETIME(6) NOT NULL,
    INDEX idx_store_sales_store (store_id),
    INDEX idx_store_sales_character (character_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS store_interiors (
    id VARCHAR(64) NOT NULL PRIMARY KEY,
    store_id VARCHAR(64) NOT NULL,
    character_id VARCHAR(64) NOT NULL,
    furniture_id VARCHAR(32) NOT NULL,
    name VARCHAR(32) NOT NULL,
    slot_index INT NOT NULL DEFAULT 0,
    created_at DATETIME(6) NOT NULL,
    INDEX idx_store_interiors_store (store_id),
    INDEX idx_store_interiors_character (character_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

INSERT IGNORE INTO schema_migrations (version) VALUES ('067_player_stores');
