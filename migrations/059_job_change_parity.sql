ALTER TABLE characters
    ADD COLUMN IF NOT EXISTS job_level INT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS old_job_id VARCHAR(64) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS old_sp INT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS job_memory_job_id VARCHAR(64) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS job_memory_sp INT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS job_memory_old_job_id VARCHAR(64) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS job_memory_old_sp INT NOT NULL DEFAULT 0;

ALTER TABLE character_jobs
    ADD COLUMN IF NOT EXISTS all_jobs_mastered BOOLEAN NOT NULL DEFAULT FALSE;

ALTER TABLE character_job_masteries
    ADD COLUMN IF NOT EXISTS mastered_sp INT NOT NULL DEFAULT 0;

CREATE TABLE IF NOT EXISTS character_future_memories (
    id VARCHAR(64) PRIMARY KEY,
    character_id CHAR(32) NOT NULL,
    slot INT NOT NULL DEFAULT 1,
    job_id VARCHAR(64) NOT NULL,
    old_job_id VARCHAR(64) NOT NULL,
    level INT NOT NULL,
    experience INT NOT NULL,
    max_hp INT NOT NULL,
    max_mp INT NOT NULL,
    attack INT NOT NULL,
    defense INT NOT NULL,
    agility INT NOT NULL,
    gender VARCHAR(32) NOT NULL,
    over_level BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_future_memories_char (character_id, slot),
    CONSTRAINT fk_character_future_memories_character
        FOREIGN KEY (character_id) REFERENCES characters (id) ON DELETE CASCADE
);

INSERT IGNORE INTO schema_migrations (version) VALUES ('059_job_change_parity');
