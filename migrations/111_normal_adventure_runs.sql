CREATE TABLE IF NOT EXISTS normal_adventure_runs (
    run_id VARCHAR(64) NOT NULL PRIMARY KEY,
    version INT NOT NULL DEFAULT 0,
    admission_json JSON NULL,
    state_json JSON NULL
) ENGINE=InnoDB;

CREATE TABLE IF NOT EXISTS normal_adventure_enrollments (
    character_id VARCHAR(64) NOT NULL PRIMARY KEY,
    run_id VARCHAR(64) NOT NULL,
    INDEX idx_normal_enrollment_run (run_id)
) ENGINE=InnoDB;

CREATE TABLE IF NOT EXISTS normal_adventure_recipients (
    run_id VARCHAR(64) NOT NULL,
    character_id VARCHAR(64) NOT NULL,
    roster_order INT NOT NULL,
    disposition VARCHAR(16) NOT NULL DEFAULT '',
    penalty_until DATETIME(6) NULL,
    recipient_json JSON NOT NULL,
    PRIMARY KEY (run_id, character_id),
    INDEX idx_normal_recipient_penalty (character_id, disposition, penalty_until)
) ENGINE=InnoDB;

-- Receipts must survive active-state disposal; deliberately no cascading FK.
CREATE TABLE IF NOT EXISTS normal_adventure_receipts (
    run_id VARCHAR(64) NOT NULL,
    command_id VARBINARY(128) NOT NULL,
    actor_id VARCHAR(64) NOT NULL,
    player_id VARCHAR(64) NOT NULL,
    receipt_json JSON NOT NULL,
    PRIMARY KEY (run_id, command_id)
) ENGINE=InnoDB;

INSERT IGNORE INTO schema_migrations (version) VALUES ('111_normal_adventure_runs');
