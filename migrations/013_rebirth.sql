ALTER TABLE characters ADD COLUMN rebirth_count INT NOT NULL DEFAULT 0;

CREATE TABLE IF NOT EXISTS character_job_masteries (
    character_id CHAR(32) NOT NULL,
    job_id VARCHAR(64) NOT NULL,
    mastered_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (character_id, job_id),
    CONSTRAINT fk_character_job_masteries_character
        FOREIGN KEY (character_id) REFERENCES characters (id)
);

INSERT IGNORE INTO schema_migrations (version) VALUES ('013_rebirth');
