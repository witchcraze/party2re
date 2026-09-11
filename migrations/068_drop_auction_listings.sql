-- Migration: 068_drop_auction_listings.sql
-- Description: Drop fictional auction listings table (Issue #474)

DROP TABLE IF EXISTS auction_listings;

INSERT IGNORE INTO schema_migrations (version) VALUES ('068_drop_auction_listings');
