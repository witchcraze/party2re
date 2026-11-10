-- Rescue commits the owed sleep deadline before clearing Valkey work;
-- applied_at stays NULL until the deadline reaches the sleep timer, so a later
-- rescue can resume an interrupted penalty instead of losing or stacking it.
ALTER TABLE rescue_records
    ADD COLUMN sleep_until DATETIME(6) NULL,
    ADD COLUMN applied_at DATETIME(6) NULL;

UPDATE rescue_records
SET sleep_until = TIMESTAMPADD(SECOND, penalty_seconds, created_at),
    applied_at = created_at;

ALTER TABLE rescue_records MODIFY sleep_until DATETIME(6) NOT NULL;

INSERT IGNORE INTO schema_migrations (version) VALUES ('121_rescue_penalty_recovery');
