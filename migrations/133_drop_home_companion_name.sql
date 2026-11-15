-- Drop the companion name setting, which has no legacy counterpart or gameplay effect (#2161).
ALTER TABLE character_homes
    DROP COLUMN IF EXISTS companion_name;

INSERT IGNORE INTO schema_migrations (version) VALUES ('133_drop_home_companion_name');
