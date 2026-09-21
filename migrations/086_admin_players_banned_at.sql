-- Migration: 086_admin_players_banned_at.sql
-- Description: Add banned_at, updated_at, and last_ip to players table for admin player management and soft-ban (Issue #200)

ALTER TABLE players
    ADD COLUMN banned_at DATETIME(6) NULL DEFAULT NULL AFTER password_hash,
    ADD COLUMN updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6) AFTER created_at,
    ADD COLUMN last_ip VARCHAR(45) NOT NULL DEFAULT '' AFTER updated_at;

INSERT IGNORE INTO schema_migrations (version) VALUES ('086_admin_players_banned_at');
