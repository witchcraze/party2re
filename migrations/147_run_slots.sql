-- 147_run_slots.sql: Unique live slot for system-opened runs (Issue #2782)
ALTER TABLE runs ADD COLUMN IF NOT EXISTS slot VARCHAR(128) NULL;
CREATE UNIQUE INDEX IF NOT EXISTS uq_runs_slot ON runs (slot);

INSERT IGNORE INTO schema_migrations (version) VALUES ('147_run_slots');
