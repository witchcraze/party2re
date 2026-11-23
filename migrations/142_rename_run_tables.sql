-- 142_rename_run_tables.sql: Rename the normal-adventure run tables for the mode-neutral run host (Issue #2649)
RENAME TABLE IF EXISTS normal_adventure_runs TO runs;
RENAME TABLE IF EXISTS normal_adventure_enrollments TO run_enrollments;
RENAME TABLE IF EXISTS normal_adventure_recipients TO run_recipients;
RENAME TABLE IF EXISTS normal_adventure_receipts TO run_receipts;

INSERT IGNORE INTO schema_migrations (version) VALUES ('142_rename_run_tables');
