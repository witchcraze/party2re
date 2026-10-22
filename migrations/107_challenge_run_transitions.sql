CREATE TABLE IF NOT EXISTS challenge_run_transitions (
    run_id VARCHAR(64) NOT NULL PRIMARY KEY,
    revision INT NOT NULL DEFAULT 0,
    pending BOOLEAN NOT NULL DEFAULT FALSE,
    publication_json JSON NULL
) ENGINE=InnoDB;

CREATE TABLE IF NOT EXISTS challenge_participants (
    run_id VARCHAR(64) NOT NULL,
    character_id VARCHAR(64) NOT NULL,
    settled BOOLEAN NOT NULL DEFAULT FALSE,
    settlement_json JSON NULL,
    PRIMARY KEY (run_id, character_id)
) ENGINE=InnoDB;

CREATE TABLE IF NOT EXISTS challenge_character_claims (
    character_id VARCHAR(64) NOT NULL PRIMARY KEY,
    run_id VARCHAR(64) NOT NULL,
    INDEX idx_challenge_claim_run (run_id)
) ENGINE=InnoDB;

CREATE TABLE IF NOT EXISTS challenge_command_results (
    run_id VARCHAR(64) NOT NULL,
    actor_id VARCHAR(64) NOT NULL,
    expected_revision INT NOT NULL,
    result_json JSON NOT NULL,
    PRIMARY KEY (run_id, actor_id, expected_revision)
) ENGINE=InnoDB;

-- Old binaries cannot enroll or finalize a migrated run after cutover.
ALTER TABLE challenge_sessions ADD COLUMN IF NOT EXISTS write_protocol BIGINT NOT NULL DEFAULT 0;
DELIMITER $$
CREATE OR REPLACE TRIGGER challenge_writer_insert BEFORE INSERT ON challenge_sessions FOR EACH ROW
BEGIN
    IF NEW.write_protocol < 1 THEN
        SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'Challenge writers must use run coordination';
    END IF;
END$$
CREATE OR REPLACE TRIGGER challenge_writer_update BEFORE UPDATE ON challenge_sessions FOR EACH ROW
BEGIN
    IF OLD.write_protocol > 0 AND NEW.write_protocol <= OLD.write_protocol THEN
        SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'Challenge writers must use run coordination';
    END IF;
END$$
DELIMITER ;

INSERT IGNORE INTO schema_migrations (version) VALUES ('107_challenge_run_transitions');
