-- Migration: 085_drop_casino_rooms_and_members.sql
-- Description: Drop unused casino_rooms and casino_members tables in favor of Valkey Master ephemeral rooms (Issue #635)

DROP TABLE IF EXISTS casino_room_members;
DROP TABLE IF EXISTS casino_members;
DROP TABLE IF EXISTS casino_rooms;

INSERT IGNORE INTO schema_migrations (version) VALUES ('085_drop_casino_rooms_and_members');
