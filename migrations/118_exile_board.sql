-- Player-run exile vote board (legacy violator.cgi).
-- exile_board.updated_at is the board-wide last write that gates withdrawals.
CREATE TABLE IF NOT EXISTS exile_board (
    id VARCHAR(16) NOT NULL PRIMARY KEY,
    updated_at DATETIME(6) NULL DEFAULT NULL
) ENGINE=InnoDB;

INSERT IGNORE INTO exile_board (id, updated_at) VALUES ('global', NULL);

-- Applicant and voter IDs keep no foreign key: legacy rows and ballots survive
-- their deletion, while a deleted target ends the petition.
CREATE TABLE IF NOT EXISTS exile_petitions (
    seq BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    id CHAR(32) NOT NULL,
    applicant_id CHAR(32) NOT NULL,
    applicant_name VARCHAR(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
    target_id CHAR(32) NOT NULL,
    target_name VARCHAR(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
    reason VARCHAR(500) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
    created_at DATETIME(6) NOT NULL,
    UNIQUE KEY uq_exile_petition_id (id),
    UNIQUE KEY uq_exile_petition_target (target_id),
    INDEX idx_exile_petition_applicant (applicant_id),
    CONSTRAINT fk_exile_petition_target FOREIGN KEY (target_id) REFERENCES characters (id) ON DELETE CASCADE
) ENGINE=InnoDB;

CREATE TABLE IF NOT EXISTS exile_votes (
    seq BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    petition_id CHAR(32) NOT NULL,
    voter_id CHAR(32) NOT NULL,
    voter_name VARCHAR(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
    approve BOOLEAN NOT NULL,
    voted_at DATETIME(6) NOT NULL,
    UNIQUE KEY uq_exile_vote (petition_id, voter_id),
    INDEX idx_exile_vote_voter (voter_id),
    CONSTRAINT fk_exile_vote_petition FOREIGN KEY (petition_id) REFERENCES exile_petitions (id) ON DELETE CASCADE
) ENGINE=InnoDB;

INSERT IGNORE INTO schema_migrations (version) VALUES ('118_exile_board');
