package database

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"time"

	"github.com/witchcraze/party2re/internal/park"
)

type ParkRepository struct {
	db *sql.DB
}

func NewParkRepository(db *sql.DB) (*ParkRepository, error) {
	if db == nil {
		return nil, errors.New("database is nil")
	}
	return &ParkRepository{db: db}, nil
}

// CreatePost commits a post, then trims the board to the newest logLimit posts. The trim runs after the
// commit so the last trim to run sees every committed post, keeping the board within the limit without a
// shared lock. A failed trim keeps the saved post; the next post trims again.
func (r *ParkRepository) CreatePost(ctx context.Context, post park.Post, logLimit int) error {
	if _, err := ExecutorFromContext(ctx, r.db).ExecContext(ctx, `
		INSERT INTO park_posts (
			id, character_id, character_name, content, color, recipient_name, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?)
	`, post.ID, post.CharacterID, post.CharacterName, post.Content, post.Color, post.RecipientName, post.CreatedAt.UTC()); err != nil {
		return err
	}

	err := RunInTx(ctx, r.db, func(txCtx context.Context) error {
		_, err := ExecutorFromContext(txCtx, r.db).ExecContext(txCtx, `
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
	})
	if err != nil {
		slog.Warn("park board trim failed", slog.Any("error", err))
	}
	return nil
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
