CREATE TABLE IF NOT EXISTS fleamarket_listings (
    id CHAR(32) NOT NULL PRIMARY KEY,
    seller_character_id CHAR(32) NOT NULL,
    seller_name VARCHAR(64) NOT NULL,
    item_id VARCHAR(64) NOT NULL,
    item_name VARCHAR(64) NOT NULL,
    item_category VARCHAR(32) NOT NULL DEFAULT 'misc',
    price INT NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'active',
    buyer_character_id CHAR(32) NULL,
    buyer_name VARCHAR(64) NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    sold_at TIMESTAMP NULL,
    INDEX idx_fleamarket_seller_status (seller_character_id, status),
    INDEX idx_fleamarket_status_created (status, created_at),
    CONSTRAINT fk_fleamarket_seller FOREIGN KEY (seller_character_id) REFERENCES characters (id) ON DELETE CASCADE,
    CONSTRAINT fk_fleamarket_buyer FOREIGN KEY (buyer_character_id) REFERENCES characters (id) ON DELETE SET NULL,
    CONSTRAINT chk_fleamarket_price CHECK (price > 0 AND price <= 999999)
);

INSERT IGNORE INTO schema_migrations (version) VALUES ('043_fleamarket');
