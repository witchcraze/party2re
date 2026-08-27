CREATE TABLE IF NOT EXISTS news_articles (
    id VARCHAR(32) NOT NULL PRIMARY KEY,
    category VARCHAR(50) NOT NULL DEFAULT 'announcement',
    title VARCHAR(200) NOT NULL,
    content TEXT NOT NULL,
    author VARCHAR(100) NOT NULL DEFAULT 'System',
    published_at DATETIME(6) NOT NULL,
    created_at DATETIME(6) NOT NULL,
    INDEX idx_news_published_at (published_at DESC),
    INDEX idx_news_category_published (category, published_at DESC)
);

CREATE TABLE IF NOT EXISTS player_notifications (
    id VARCHAR(32) NOT NULL PRIMARY KEY,
    player_id VARCHAR(32) NOT NULL,
    category VARCHAR(50) NOT NULL DEFAULT 'system',
    title VARCHAR(200) NOT NULL,
    body TEXT NOT NULL,
    link VARCHAR(255) NOT NULL DEFAULT '',
    is_read BOOLEAN NOT NULL DEFAULT FALSE,
    read_at DATETIME(6) NULL,
    created_at DATETIME(6) NOT NULL,
    CONSTRAINT fk_player_notifications_player FOREIGN KEY (player_id) REFERENCES players(id) ON DELETE CASCADE,
    INDEX idx_player_notifications_player_unread (player_id, is_read, created_at DESC),
    INDEX idx_player_notifications_player_created (player_id, created_at DESC)
);

INSERT IGNORE INTO schema_migrations (version) VALUES ('033_news_and_notifications');
