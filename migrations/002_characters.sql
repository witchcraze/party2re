CREATE TABLE IF NOT EXISTS characters (
    id CHAR(32) NOT NULL PRIMARY KEY,
    name VARCHAR(32) NOT NULL,
    level INT NOT NULL,
    experience BIGINT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT IGNORE INTO schema_migrations (version) VALUES ('002_characters');
