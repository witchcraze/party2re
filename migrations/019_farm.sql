CREATE TABLE IF NOT EXISTS farm_plots (
    id CHAR(32) NOT NULL PRIMARY KEY,
    character_id CHAR(32) NOT NULL,
    plot_index INT NOT NULL,
    seed_type VARCHAR(32) NOT NULL,
    status VARCHAR(16) NOT NULL DEFAULT 'EMPTY',
    planted_at TIMESTAMP NULL,
    matures_at TIMESTAMP NULL,
    wither_at TIMESTAMP NULL,
    watered BOOLEAN NOT NULL DEFAULT FALSE,
    fertilized BOOLEAN NOT NULL DEFAULT FALSE,
    yield INT NOT NULL DEFAULT 1,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    CONSTRAINT uq_farm_plots_char_idx UNIQUE (character_id, plot_index),
    CONSTRAINT fk_farm_plots_character FOREIGN KEY (character_id) REFERENCES characters (id) ON DELETE CASCADE
);

CREATE INDEX idx_farm_plots_char ON farm_plots (character_id);

INSERT IGNORE INTO schema_migrations (version) VALUES ('019_farm');
