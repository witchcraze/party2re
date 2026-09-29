ALTER TABLE character_profiles ADD COLUMN IF NOT EXISTS aura_effect INT NOT NULL DEFAULT 0;
INSERT IGNORE INTO schema_migrations (version) VALUES ('092_character_profile_aura_effect');
