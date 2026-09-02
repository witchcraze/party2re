package database

import (
	"context"
	"database/sql"
	"errors"
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

func (r *ParkRepository) CreatePost(ctx context.Context, post park.Post) error {
	_, err := ExecutorFromContext(ctx, r.db).ExecContext(ctx, `
		INSERT INTO park_posts (
			id, character_id, character_name, content, color, recipient_name, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?)
	`, post.ID, post.CharacterID, post.CharacterName, post.Content, post.Color, post.RecipientName, post.CreatedAt.UTC())
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
		ORDER BY created_at DESC
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

func (r *ParkRepository) GetLatestPostTimeByCharacter(ctx context.Context, characterID string) (time.Time, error) {
	var createdAt time.Time
	err := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, `
		SELECT created_at
		FROM park_posts
		WHERE character_id = ?
		ORDER BY created_at DESC
		LIMIT 1
	`, characterID).Scan(&createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return time.Time{}, nil
	}
	if err != nil {
		return time.Time{}, err
	}
	return createdAt, nil
}
