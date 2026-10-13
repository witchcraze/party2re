CREATE TABLE IF NOT EXISTS character_job_points (
    character_id CHAR(32) NOT NULL,
    job_id VARCHAR(64) NOT NULL,
    earned_sp INT NOT NULL CHECK (earned_sp >= 0),
    PRIMARY KEY (character_id, job_id),
    CONSTRAINT fk_character_job_points_character
        FOREIGN KEY (character_id) REFERENCES characters (id) ON DELETE CASCADE
);

-- Preserve known values. Transition records cannot recover missing historical SP.
-- Active/previous values take precedence over possibly older mastery snapshots.
INSERT IGNORE INTO character_job_points (character_id, job_id, earned_sp)
    SELECT id, job_id, sp FROM characters WHERE job_id <> '';
INSERT IGNORE INTO character_job_points (character_id, job_id, earned_sp)
    SELECT id, old_job_id, old_sp FROM characters WHERE old_job_id <> '';
INSERT IGNORE INTO character_job_points (character_id, job_id, earned_sp)
    SELECT character_id, job_id, mastered_sp FROM character_job_masteries;

INSERT IGNORE INTO schema_migrations (version) VALUES ('103_character_job_points');
