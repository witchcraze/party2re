-- Black Market & Underground Illicit Trade Tables
CREATE TABLE IF NOT EXISTS blackmarket_character_purchases (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    character_id VARCHAR(64) NOT NULL,
    item_id VARCHAR(64) NOT NULL,
    purchase_date DATE NOT NULL,
    quantity INT NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    UNIQUE KEY uk_char_item_date (character_id, item_id, purchase_date),
    INDEX idx_char_date (character_id, purchase_date)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS blackmarket_market_state (
    id INT PRIMARY KEY DEFAULT 1,
    condition_name VARCHAR(32) NOT NULL DEFAULT 'Quiet',
    price_multiplier DECIMAL(5,2) NOT NULL DEFAULT 1.00,
    sell_multiplier DECIMAL(5,2) NOT NULL DEFAULT 1.00,
    risk_level VARCHAR(16) NOT NULL DEFAULT 'Low',
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

INSERT INTO blackmarket_market_state (id, condition_name, price_multiplier, sell_multiplier, risk_level)
VALUES (1, 'Quiet', 1.00, 1.00, 'Low')
ON DUPLICATE KEY UPDATE id=id;
