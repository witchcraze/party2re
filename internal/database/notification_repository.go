package database

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/witchcraze/party2re/internal/notification"
)

type NotificationRepository struct {
	db *sql.DB
}

func NewNotificationRepository(db *sql.DB) (*NotificationRepository, error) {
	if db == nil {
		return nil, errors.New("database is nil")
	}
	return &NotificationRepository{db: db}, nil
}

// CreateNews inserts a new news article.
func (r *NotificationRepository) CreateNews(ctx context.Context, article notification.NewsArticle) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO news_articles (
			id, category, title, content, author, published_at, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?)
	`, article.ID, article.Category, article.Title, article.Content, article.Author, article.PublishedAt.UTC(), article.CreatedAt.UTC())
	return err
}

// GetNewsByID fetches a single news article by ID.
func (r *NotificationRepository) GetNewsByID(ctx context.Context, id string) (notification.NewsArticle, error) {
	var a notification.NewsArticle
	err := r.db.QueryRowContext(ctx, `
		SELECT id, category, title, content, author, published_at, created_at
		FROM news_articles
		WHERE id = ?
	`, id).Scan(&a.ID, &a.Category, &a.Title, &a.Content, &a.Author, &a.PublishedAt, &a.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return notification.NewsArticle{}, notification.ErrNewsNotFound
	}
	if err != nil {
		return notification.NewsArticle{}, err
	}
	return a, nil
}

// ListNews returns a paginated list of news articles ordered by publication date descending.
func (r *NotificationRepository) ListNews(ctx context.Context, limit, offset int) ([]notification.NewsArticle, int, error) {
	var total int
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM news_articles`).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	rows, err := r.db.QueryContext(ctx, `
		SELECT id, category, title, content, author, published_at, created_at
		FROM news_articles
		ORDER BY published_at DESC, created_at DESC
		LIMIT ? OFFSET ?
	`, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	articles := make([]notification.NewsArticle, 0)
	for rows.Next() {
		var a notification.NewsArticle
		if err := rows.Scan(&a.ID, &a.Category, &a.Title, &a.Content, &a.Author, &a.PublishedAt, &a.CreatedAt); err != nil {
			return nil, 0, err
		}
		articles = append(articles, a)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	return articles, total, nil
}

// CreateNotification inserts a single player notification.
func (r *NotificationRepository) CreateNotification(ctx context.Context, notif notification.PlayerNotification) error {
	var readAt sql.NullTime
	if notif.ReadAt != nil {
		readAt = sql.NullTime{Time: notif.ReadAt.UTC(), Valid: true}
	}

	_, err := r.db.ExecContext(ctx, `
		INSERT INTO player_notifications (
			id, player_id, category, title, body, link, is_read, read_at, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, notif.ID, notif.PlayerID, notif.Category, notif.Title, notif.Body, notif.Link, notif.IsRead, readAt, notif.CreatedAt.UTC())
	return err
}

// CreateBatchNotifications inserts multiple player notifications in a single transaction.
func (r *NotificationRepository) CreateBatchNotifications(ctx context.Context, notifs []notification.PlayerNotification) error {
	if len(notifs) == 0 {
		return nil
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO player_notifications (
			id, player_id, category, title, body, link, is_read, read_at, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, notif := range notifs {
		var readAt sql.NullTime
		if notif.ReadAt != nil {
			readAt = sql.NullTime{Time: notif.ReadAt.UTC(), Valid: true}
		}
		_, err := stmt.ExecContext(ctx, notif.ID, notif.PlayerID, notif.Category, notif.Title, notif.Body, notif.Link, notif.IsRead, readAt, notif.CreatedAt.UTC())
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

// GetNotificationByID retrieves a notification by ID.
func (r *NotificationRepository) GetNotificationByID(ctx context.Context, id string) (notification.PlayerNotification, error) {
	var n notification.PlayerNotification
	var readAt sql.NullTime

	err := r.db.QueryRowContext(ctx, `
		SELECT id, player_id, category, title, body, link, is_read, read_at, created_at
		FROM player_notifications
		WHERE id = ?
	`, id).Scan(&n.ID, &n.PlayerID, &n.Category, &n.Title, &n.Body, &n.Link, &n.IsRead, &readAt, &n.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return notification.PlayerNotification{}, notification.ErrNotificationNotFound
	}
	if err != nil {
		return notification.PlayerNotification{}, err
	}

	if readAt.Valid {
		t := readAt.Time
		n.ReadAt = &t
	}

	return n, nil
}

// ListNotificationsByPlayer retrieves notifications for a specific player.
func (r *NotificationRepository) ListNotificationsByPlayer(ctx context.Context, playerID string, unreadOnly bool, limit, offset int) ([]notification.PlayerNotification, int, error) {
	var totalQuery string
	var listQuery string
	var args []any

	if unreadOnly {
		totalQuery = `SELECT COUNT(*) FROM player_notifications WHERE player_id = ? AND is_read = FALSE`
		listQuery = `
			SELECT id, player_id, category, title, body, link, is_read, read_at, created_at
			FROM player_notifications
			WHERE player_id = ? AND is_read = FALSE
			ORDER BY created_at DESC
			LIMIT ? OFFSET ?
		`
		args = []any{playerID, limit, offset}
	} else {
		totalQuery = `SELECT COUNT(*) FROM player_notifications WHERE player_id = ?`
		listQuery = `
			SELECT id, player_id, category, title, body, link, is_read, read_at, created_at
			FROM player_notifications
			WHERE player_id = ?
			ORDER BY created_at DESC
			LIMIT ? OFFSET ?
		`
		args = []any{playerID, limit, offset}
	}

	var total int
	err := r.db.QueryRowContext(ctx, totalQuery, playerID).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	rows, err := r.db.QueryContext(ctx, listQuery, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	notifs := make([]notification.PlayerNotification, 0)
	for rows.Next() {
		var n notification.PlayerNotification
		var readAt sql.NullTime
		if err := rows.Scan(&n.ID, &n.PlayerID, &n.Category, &n.Title, &n.Body, &n.Link, &n.IsRead, &readAt, &n.CreatedAt); err != nil {
			return nil, 0, err
		}
		if readAt.Valid {
			t := readAt.Time
			n.ReadAt = &t
		}
		notifs = append(notifs, n)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	return notifs, total, nil
}

// GetUnreadCount retrieves the number of unread notifications for a player.
func (r *NotificationRepository) GetUnreadCount(ctx context.Context, playerID string) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM player_notifications
		WHERE player_id = ? AND is_read = FALSE
	`, playerID).Scan(&count)
	if err != nil {
		return 0, err
	}
	return count, nil
}

// MarkAsRead marks a notification as read for a player.
func (r *NotificationRepository) MarkAsRead(ctx context.Context, id, playerID string, readAt time.Time) error {
	res, err := r.db.ExecContext(ctx, `
		UPDATE player_notifications
		SET is_read = TRUE, read_at = ?
		WHERE id = ? AND player_id = ?
	`, readAt.UTC(), id, playerID)
	if err != nil {
		return err
	}

	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		// Check if notification exists under another player or not at all
		var actualPlayerID string
		err = r.db.QueryRowContext(ctx, `SELECT player_id FROM player_notifications WHERE id = ?`, id).Scan(&actualPlayerID)
		if errors.Is(err, sql.ErrNoRows) {
			return notification.ErrNotificationNotFound
		}
		if err != nil {
			return err
		}
		if actualPlayerID != playerID {
			return notification.ErrForbidden
		}
	}
	return nil
}

// MarkAllAsRead marks all unread notifications for a player as read.
func (r *NotificationRepository) MarkAllAsRead(ctx context.Context, playerID string, readAt time.Time) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE player_notifications
		SET is_read = TRUE, read_at = ?
		WHERE player_id = ? AND is_read = FALSE
	`, readAt.UTC(), playerID)
	return err
}

// DeleteNotification deletes a notification belonging to a player.
func (r *NotificationRepository) DeleteNotification(ctx context.Context, id, playerID string) error {
	res, err := r.db.ExecContext(ctx, `
		DELETE FROM player_notifications
		WHERE id = ? AND player_id = ?
	`, id, playerID)
	if err != nil {
		return err
	}

	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		var actualPlayerID string
		err = r.db.QueryRowContext(ctx, `SELECT player_id FROM player_notifications WHERE id = ?`, id).Scan(&actualPlayerID)
		if errors.Is(err, sql.ErrNoRows) {
			return notification.ErrNotificationNotFound
		}
		if err != nil {
			return err
		}
		if actualPlayerID != playerID {
			return notification.ErrForbidden
		}
	}
	return nil
}

// DeleteExpiredNotifications deletes notifications created before the threshold date.
func (r *NotificationRepository) DeleteExpiredNotifications(ctx context.Context, olderThan time.Time) (int64, error) {
	res, err := r.db.ExecContext(ctx, `
		DELETE FROM player_notifications
		WHERE created_at < ?
	`, olderThan.UTC())
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
