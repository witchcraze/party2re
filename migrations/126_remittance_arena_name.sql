-- Arena prize receipts retain the full room name (up to 50 characters).
ALTER TABLE character_pending_remittances MODIFY sender_name VARCHAR(50) NOT NULL;

INSERT IGNORE INTO schema_migrations (version) VALUES ('126_remittance_arena_name');
