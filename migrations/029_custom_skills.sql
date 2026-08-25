CREATE TABLE IF NOT EXISTS character_custom_skills (
    character_id VARCHAR(64) NOT NULL PRIMARY KEY,
    max_slots INT NOT NULL DEFAULT 4,
    equipped_skills_json JSON NOT NULL,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
);

INSERT IGNORE INTO schema_migrations (version) VALUES ('029_custom_skills');
