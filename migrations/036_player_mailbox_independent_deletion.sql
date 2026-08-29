-- Add independent deletion flags to character_letters
ALTER TABLE character_letters
    ADD COLUMN is_deleted_by_sender BOOLEAN NOT NULL DEFAULT FALSE AFTER read_at,
    ADD COLUMN is_deleted_by_recipient BOOLEAN NOT NULL DEFAULT FALSE AFTER is_deleted_by_sender;

-- Add updated active indexes
ALTER TABLE character_letters
    ADD INDEX idx_character_letters_recipient_active_unread (recipient_character_id, is_deleted_by_recipient, is_read, created_at DESC),
    ADD INDEX idx_character_letters_recipient_active (recipient_character_id, is_deleted_by_recipient, created_at DESC),
    ADD INDEX idx_character_letters_sender_active (sender_character_id, is_deleted_by_sender, created_at DESC);

-- Drop obsolete indexes
ALTER TABLE character_letters
    DROP INDEX idx_character_letters_recipient_unread,
    DROP INDEX idx_character_letters_recipient_created,
    DROP INDEX idx_character_letters_sender_created;

INSERT IGNORE INTO schema_migrations (version) VALUES ('036_player_mailbox_independent_deletion');
