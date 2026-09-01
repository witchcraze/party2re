-- Migration: 049_player_deletion_and_maintenance.sql
-- Description: System maintenance mode persistence table

CREATE TABLE IF NOT EXISTS system_maintenance (
    id VARCHAR(64) PRIMARY KEY,
    is_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    message VARCHAR(500) NOT NULL DEFAULT '',
    estimated_end_time DATETIME NULL,
    updated_at DATETIME NOT NULL
);

INSERT IGNORE INTO system_maintenance (id, is_enabled, message, estimated_end_time, updated_at)
VALUES ('global', FALSE, 'System is operating normally.', NULL, UTC_TIMESTAMP());
