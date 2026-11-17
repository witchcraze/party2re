-- Durable room-log activity clock for PvP inactivity cleanup (#2046).
ALTER TABLE pvp_room_transitions
    ADD COLUMN IF NOT EXISTS last_activity_at DATETIME(6) NULL;

INSERT IGNORE INTO schema_migrations (version) VALUES ('136_pvp_inactivity_cleanup');
