DELIMITER //
BEGIN NOT ATOMIC
    IF EXISTS (
        SELECT 1 FROM characters
        GROUP BY CONVERT(name USING utf8mb4) COLLATE utf8mb4_nopad_bin
        HAVING COUNT(*) > 1
    ) THEN
        SELECT CONVERT(name USING utf8mb4) COLLATE utf8mb4_nopad_bin AS duplicate_name,
               COUNT(*) AS character_count, GROUP_CONCAT(id ORDER BY id) AS character_ids
        FROM characters GROUP BY duplicate_name HAVING COUNT(*) > 1;
        SIGNAL SQLSTATE '45000'
            SET MESSAGE_TEXT = 'Duplicate character names: resolve the listed character IDs explicitly, then rerun migration 110. No rows changed.';
    END IF;
END//

ALTER TABLE characters
    MODIFY name VARCHAR(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_nopad_bin NOT NULL,
    DROP INDEX IF EXISTS idx_characters_name,
    ADD UNIQUE INDEX IF NOT EXISTS uq_characters_name (name)//

INSERT IGNORE INTO schema_migrations (version) VALUES ('110_character_name_uniqueness')//
DELIMITER ;
