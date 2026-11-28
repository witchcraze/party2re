-- The recall keeps the current gender (job_change.cgi job_call_future), and
-- snapshots are ordered by created_at; neither column was read.
ALTER TABLE character_future_memories
    DROP COLUMN IF EXISTS gender,
    DROP COLUMN IF EXISTS slot;

INSERT IGNORE INTO schema_migrations (version) VALUES ('149_future_memory_unused_columns');
