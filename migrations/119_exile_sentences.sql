-- Committed sleep sentences not yet confirmed in the Valkey sleep timer.
-- One row per character: a later verdict replaces the earlier deadline.
CREATE TABLE IF NOT EXISTS exile_sentences (
    character_id CHAR(32) NOT NULL PRIMARY KEY,
    sleep_until DATETIME(6) NOT NULL,
    CONSTRAINT fk_exile_sentence_character FOREIGN KEY (character_id) REFERENCES characters (id) ON DELETE CASCADE
) ENGINE=InnoDB;

INSERT IGNORE INTO schema_migrations (version) VALUES ('119_exile_sentences');
