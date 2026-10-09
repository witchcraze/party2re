ALTER TABLE characters ADD COLUMN IF NOT EXISTS job_memory_kind VARCHAR(32) NOT NULL DEFAULT 'persistent' AFTER job_memory_old_sp;

INSERT IGNORE INTO schema_migrations (version) VALUES ('094_job_memory_kind');
