CREATE TABLE IF NOT EXISTS activities (
    id CHAR(32) NOT NULL PRIMARY KEY,
    character_id CHAR(32) NOT NULL,
    activity_type VARCHAR(32) NOT NULL,
    started_at DATETIME(6) NOT NULL,
    available_at DATETIME(6) NOT NULL,
    experience_reward INT NOT NULL,
    claimed BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT fk_activities_character
        FOREIGN KEY (character_id) REFERENCES characters (id)
);

INSERT IGNORE INTO schema_migrations (version) VALUES ('003_activities');
