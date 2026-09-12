-- Migration: 069_drop_fictional_delivery_tables.sql
-- Description: Drop fictional delivery quests and parcel courier tables (Issue #475)

DROP TABLE IF EXISTS character_deliveries;
DROP TABLE IF EXISTS delivery_parcels;
DROP TABLE IF EXISTS delivery_quests;

INSERT IGNORE INTO schema_migrations (version) VALUES ('069_drop_fictional_delivery_tables');
