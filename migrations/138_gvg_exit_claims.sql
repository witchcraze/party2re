-- Durable per-character exit claim for GvG rooms to keep departure side-effects idempotent across retries (#2533).
CREATE TABLE IF NOT EXISTS gvg_exit_claims (
    room_id VARCHAR(36) NOT NULL,
    character_id CHAR(32) NOT NULL,
    delivery_done BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (room_id, character_id),
    CONSTRAINT fk_gvg_exit_claims_character
        FOREIGN KEY (character_id) REFERENCES characters (id) ON DELETE CASCADE
) ENGINE=InnoDB;

INSERT IGNORE INTO schema_migrations (version) VALUES ('138_gvg_exit_claims');
