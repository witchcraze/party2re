-- 140_drop_character_profile_comment.sql: Remove fictional character_profiles.comment column (Issue #2333)

ALTER TABLE character_profiles DROP COLUMN IF EXISTS comment;

INSERT IGNORE INTO schema_migrations (version) VALUES ('140_drop_character_profile_comment');
