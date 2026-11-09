-- Each mailbox copy outlives the counterpart's character (#1094). IDs stay as
-- opaque historical references; CharacterRepository.Delete hides the deleted
-- character's copies and purges rows no owner can still see.
ALTER TABLE character_letters
    DROP FOREIGN KEY fk_character_letters_sender,
    DROP FOREIGN KEY fk_character_letters_recipient;

INSERT IGNORE INTO schema_migrations (version) VALUES ('120_letters_outlive_identities');
