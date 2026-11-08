package database

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/witchcraze/party2re/internal/park"
)

const (
	maxTrimRetries   = 3
	trimRetryBackoff = 5 * time.Millisecond
)

type ParkRepository struct {
	db     *sql.DB
	trimFn func(ctx context.Context, logLimit int) error
}

func NewParkRepository(db *sql.DB) (*ParkRepository, error) {
	if db == nil {
		return nil, errors.New("database is nil")
	}
	return &ParkRepository{db: db}, nil
}

func isDeadlock(err error) bool {
	if err == nil {
		return false
	}
	var mysqlErr *mysql.MySQLError
	if errors.As(err, &mysqlErr) {
		return mysqlErr.Number == 1213 || mysqlErr.Number == 1205
	}
	errStr := strings.ToLower(err.Error())
	return strings.Contains(errStr, "1213") ||
		strings.Contains(errStr, "deadlock") ||
		strings.Contains(errStr, "1205") ||
		strings.Contains(errStr, "lock wait timeout")
}

// CreatePost commits a post, then trims the board to the newest logLimit posts in a separate transaction.
// If concurrent trims encounter a transient deadlock or lock wait timeout, the trim is retried up to a
// bounded number of attempts so the board stays within the limit without a shared lock.
// A failed trim keeps the saved post; subsequent posts will trim again.
func (r *ParkRepository) CreatePost(ctx context.Context, post park.Post, logLimit int) error {
	if _, err := ExecutorFromContext(ctx, r.db).ExecContext(ctx, `
		INSERT INTO park_posts (
			id, character_id, character_name, content, color, recipient_name, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?)
	`, post.ID, post.CharacterID, post.CharacterName, post.Content, post.Color, post.RecipientName, post.CreatedAt.UTC()); err != nil {
		return err
	}

	var trimErr error
	for attempt := 0; attempt <= maxTrimRetries; attempt++ {
		trimErr = RunInTx(ctx, r.db, func(txCtx context.Context) error {
			if r.trimFn != nil {
				return r.trimFn(txCtx, logLimit)
			}
			return r.trimBoard(txCtx, logLimit)
		})
		if trimErr == nil {
			break
		}
		if !isDeadlock(trimErr) || ctx.Err() != nil || attempt == maxTrimRetries {
			break
		}

		timer := time.NewTimer(time.Duration(attempt+1) * trimRetryBackoff)
		select {
		case <-ctx.Done():
			timer.Stop()
		case <-timer.C:
		}
		if ctx.Err() != nil {
			break
		}
	}
	if trimErr != nil {
		slog.Warn("park board trim failed", slog.Any("error", trimErr))
	}
	return nil
}

func (r *ParkRepository) trimBoard(ctx context.Context, logLimit int) error {
	_, err := ExecutorFromContext(ctx, r.db).ExecContext(ctx, `
		DELETE FROM park_posts
		WHERE id NOT IN (
			SELECT id FROM (
				SELECT id FROM park_posts
				ORDER BY created_at DESC, id DESC
				LIMIT ?
			) AS newest
		)
	`, logLimit)
	return err
}

func (r *ParkRepository) GetRecentPosts(ctx context.Context, limit int, offset int) ([]park.Post, int, error) {
	var total int
	executor := ExecutorFromContext(ctx, r.db)
	err := executor.QueryRowContext(ctx, `SELECT COUNT(*) FROM park_posts`).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	rows, err := executor.QueryContext(ctx, `
		SELECT id, character_id, character_name, content, color, recipient_name, created_at
		FROM park_posts
		ORDER BY created_at DESC, id DESC
		LIMIT ? OFFSET ?
	`, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	posts := make([]park.Post, 0)
	for rows.Next() {
		var p park.Post
		if err := rows.Scan(
			&p.ID,
			&p.CharacterID,
			&p.CharacterName,
			&p.Content,
			&p.Color,
			&p.RecipientName,
			&p.CreatedAt,
		); err != nil {
			return nil, 0, err
		}
		posts = append(posts, p)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	return posts, total, nil
}

func (r *ParkRepository) GetRecentPostsByCursor(ctx context.Context, limit int, beforeTime time.Time, beforeID string) ([]park.Post, error) {
	executor := ExecutorFromContext(ctx, r.db)
	var rows *sql.Rows
	var err error

	if beforeTime.IsZero() && beforeID == "" {
		rows, err = executor.QueryContext(ctx, `
			SELECT id, character_id, character_name, content, color, recipient_name, created_at
			FROM park_posts
			ORDER BY created_at DESC, id DESC
			LIMIT ?
		`, limit)
	} else if !beforeTime.IsZero() && beforeID != "" {
		rows, err = executor.QueryContext(ctx, `
			SELECT id, character_id, character_name, content, color, recipient_name, created_at
			FROM park_posts
			WHERE created_at < ? OR (created_at = ? AND id < ?)
			ORDER BY created_at DESC, id DESC
			LIMIT ?
		`, beforeTime.UTC(), beforeTime.UTC(), beforeID, limit)
	} else if !beforeTime.IsZero() {
		rows, err = executor.QueryContext(ctx, `
			SELECT id, character_id, character_name, content, color, recipient_name, created_at
			FROM park_posts
			WHERE created_at < ?
			ORDER BY created_at DESC, id DESC
			LIMIT ?
		`, beforeTime.UTC(), limit)
	} else {
		rows, err = executor.QueryContext(ctx, `
			SELECT id, character_id, character_name, content, color, recipient_name, created_at
			FROM park_posts
			WHERE id < ?
			ORDER BY id DESC
			LIMIT ?
		`, beforeID, limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	posts := make([]park.Post, 0)
	for rows.Next() {
		var p park.Post
		if err := rows.Scan(
			&p.ID,
			&p.CharacterID,
			&p.CharacterName,
			&p.Content,
			&p.Color,
			&p.RecipientName,
			&p.CreatedAt,
		); err != nil {
			return nil, err
		}
		posts = append(posts, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return posts, nil
}
