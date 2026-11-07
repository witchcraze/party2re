-- Operation-driven Replay scenes and personal latest slots. Sources outlive active runs: no FK to run tables.
CREATE TABLE IF NOT EXISTS replay_scene_sources (
    run_id VARCHAR(64) NOT NULL PRIMARY KEY,
    encoded_bytes BIGINT NOT NULL DEFAULT 0,
    scene_count INT NOT NULL DEFAULT 0
) ENGINE=InnoDB;

-- Immutable scenes in committed operation order; encoded_bytes is recorded once at append.
CREATE TABLE IF NOT EXISTS replay_scenes (
    run_id VARCHAR(64) NOT NULL,
    seq INT NOT NULL,
    command_id VARBINARY(128) NOT NULL,
    actor_id VARCHAR(64) NOT NULL,
    scene_json MEDIUMTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
    encoded_bytes INT NOT NULL,
    PRIMARY KEY (run_id, seq),
    UNIQUE KEY uq_replay_scene_command (run_id, command_id)
) ENGINE=InnoDB;

-- Frozen exit-prefix copy intent; seq orders accepted exits per recipient across runs.
CREATE TABLE IF NOT EXISTS replay_archive_copies (
    seq BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    id VARCHAR(64) NOT NULL,
    recipient_id VARCHAR(64) NOT NULL,
    run_id VARCHAR(64) NOT NULL,
    command_id VARBINARY(128) NOT NULL,
    prefix_len INT NOT NULL,
    status VARCHAR(16) NOT NULL DEFAULT 'pending',
    UNIQUE KEY uq_replay_copy_id (id),
    UNIQUE KEY uq_replay_copy_command (run_id, command_id),
    INDEX idx_replay_copy_recipient (recipient_id, status, seq)
) ENGINE=InnoDB;

CREATE TABLE IF NOT EXISTS replay_latest_slots (
    recipient_id VARCHAR(64) NOT NULL PRIMARY KEY,
    copy_id VARCHAR(64) NOT NULL,
    copy_seq BIGINT NOT NULL
) ENGINE=InnoDB;

INSERT IGNORE INTO schema_migrations (version) VALUES ('115_replay_scene_archives');
