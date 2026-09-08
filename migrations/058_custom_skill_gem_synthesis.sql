ALTER TABLE character_custom_skills
    ADD COLUMN skill_name VARCHAR(255) NOT NULL DEFAULT '',
    ADD COLUMN skill_comment VARCHAR(255) NOT NULL DEFAULT '',
    ADD COLUMN need_cmp INT NOT NULL DEFAULT 0,
    ADD COLUMN gem1 VARCHAR(64) NOT NULL DEFAULT '',
    ADD COLUMN gem2 VARCHAR(64) NOT NULL DEFAULT '',
    ADD COLUMN gem3 VARCHAR(64) NOT NULL DEFAULT '',
    DROP COLUMN max_slots,
    DROP COLUMN equipped_skills_json;

INSERT IGNORE INTO schema_migrations (version) VALUES ('058_custom_skill_gem_synthesis');
