ALTER TABLE characters
    ADD COLUMN over_level BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN over_depot INT NOT NULL DEFAULT 0,
    ADD COLUMN over_monster INT NOT NULL DEFAULT 0,
    ADD COLUMN over_future INT NOT NULL DEFAULT 0,
    ADD COLUMN over_flea INT NOT NULL DEFAULT 0,
    ADD COLUMN over_store INT NOT NULL DEFAULT 0;

INSERT IGNORE INTO schema_migrations (version) VALUES ('044_god_wishes_and_limit_breaks');
