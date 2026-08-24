CREATE TABLE IF NOT EXISTS auction_listings (
    id CHAR(32) NOT NULL PRIMARY KEY,
    seller_character_id CHAR(32) NOT NULL,
    item_id VARCHAR(64) NOT NULL,
    item_name VARCHAR(64) NOT NULL,
    item_category VARCHAR(32) NOT NULL,
    enhancement_level INT NOT NULL DEFAULT 0,
    start_bid INT NOT NULL,
    current_bid INT NOT NULL,
    buyout_price INT NOT NULL DEFAULT 0,
    highest_bidder_id CHAR(32) NULL,
    status VARCHAR(16) NOT NULL DEFAULT 'ACTIVE',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at TIMESTAMP NOT NULL,
    settled_at TIMESTAMP NULL,
    CONSTRAINT fk_auction_seller FOREIGN KEY (seller_character_id) REFERENCES characters (id) ON DELETE CASCADE,
    CONSTRAINT fk_auction_bidder FOREIGN KEY (highest_bidder_id) REFERENCES characters (id) ON DELETE SET NULL,
    CONSTRAINT chk_auction_bids CHECK (start_bid > 0 AND current_bid >= 0 AND buyout_price >= 0)
);

CREATE INDEX idx_auction_status ON auction_listings (status);
CREATE INDEX idx_auction_seller ON auction_listings (seller_character_id);
CREATE INDEX idx_auction_expires ON auction_listings (expires_at);

INSERT IGNORE INTO schema_migrations (version) VALUES ('020_auctions');
