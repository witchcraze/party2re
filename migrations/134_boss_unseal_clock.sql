CREATE TABLE IF NOT EXISTS boss_unseal_clock (
    id TINYINT NOT NULL PRIMARY KEY,
    unsealed_at DATETIME(6) NOT NULL,
    CONSTRAINT chk_boss_unseal_clock_singleton CHECK (id = 1)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

INSERT IGNORE INTO schema_migrations (version) VALUES ('134_boss_unseal_clock');
