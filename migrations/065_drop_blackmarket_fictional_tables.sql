-- Migration: 065_drop_blackmarket_fictional_tables.sql
-- Description: Drop fictional black market purchase quotas and market state tables (Issue #463)

DROP TABLE IF EXISTS blackmarket_character_purchases;
DROP TABLE IF EXISTS blackmarket_market_state;

INSERT IGNORE INTO schema_migrations (version) VALUES ('065_drop_blackmarket_fictional_tables');
