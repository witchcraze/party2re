CREATE TABLE IF NOT EXISTS character_jobs (
    character_id CHAR(32) NOT NULL PRIMARY KEY,
    current_job_id VARCHAR(64) NOT NULL,
    CONSTRAINT fk_character_jobs_character
        FOREIGN KEY (character_id) REFERENCES characters (id)
);

CREATE TABLE IF NOT EXISTS character_job_history (
    id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    character_id CHAR(32) NOT NULL,
    from_job_id VARCHAR(64) NOT NULL,
    to_job_id VARCHAR(64) NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT fk_character_job_history_character
        FOREIGN KEY (character_id) REFERENCES characters (id)
);

CREATE INDEX idx_character_job_history_character
    ON character_job_history (character_id, id);

INSERT IGNORE INTO schema_migrations (version) VALUES ('007_character_jobs');
