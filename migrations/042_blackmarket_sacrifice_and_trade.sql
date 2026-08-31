-- Black Market Rare Point Sacrifice and Prize Trade Exchange Table
CREATE TABLE IF NOT EXISTS blackmarket_character_points (
    character_id CHAR(32) NOT NULL PRIMARY KEY,
    rare_points INT NOT NULL DEFAULT 0,
    u_rare_points INT NOT NULL DEFAULT 0,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    CONSTRAINT fk_blackmarket_points_character FOREIGN KEY (character_id) REFERENCES characters (id) ON DELETE CASCADE
);

INSERT IGNORE INTO schema_migrations (version) VALUES ('042_blackmarket_sacrifice_and_trade');
