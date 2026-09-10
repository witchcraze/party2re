-- Migration 064: Character Bank Deposit & Fictional Bank Transfers Purge
-- Parity with legacy Party2 (bank.cgi):
-- 1. Adds characters.deposit for character-scoped bank gold savings (up to 99兆G / 99,999,999,999,999 G).
-- 2. Migrates existing player bank account balances into character deposit if applicable.
-- 3. Purges fictional bank_transfers audit table and bank_accounts table.

ALTER TABLE characters
    ADD COLUMN IF NOT EXISTS deposit BIGINT NOT NULL DEFAULT 0;

-- Migrate existing bank account balances if any character exists for the player
UPDATE characters c
JOIN bank_accounts b ON c.player_id = b.player_id
SET c.deposit = b.balance
WHERE c.id IN (
    SELECT min_id FROM (
        SELECT MIN(id) AS min_id FROM characters GROUP BY player_id
    ) sub
);

DROP TABLE IF EXISTS bank_transfers;
DROP TABLE IF EXISTS bank_accounts;

INSERT IGNORE INTO schema_migrations (version) VALUES ('064_character_bank_deposit');
