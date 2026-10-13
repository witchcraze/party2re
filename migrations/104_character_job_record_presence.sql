ALTER TABLE character_job_points
    ADD COLUMN IF NOT EXISTS is_recorded BOOLEAN NOT NULL DEFAULT FALSE;

-- Known current/previous SP alone is not evidence of a legacy job record.
UPDATE character_job_points p
SET p.is_recorded = TRUE
WHERE EXISTS (
    SELECT 1 FROM character_job_history h
    WHERE h.character_id = p.character_id AND h.from_job_id = p.job_id
) OR EXISTS (
    SELECT 1 FROM character_job_masteries m
    WHERE m.character_id = p.character_id AND m.job_id = p.job_id
);

INSERT IGNORE INTO schema_migrations (version) VALUES ('104_character_job_record_presence');
