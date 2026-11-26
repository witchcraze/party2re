package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// BossRepository keeps the demon king unseal clock (boss_unseal_clock).
type BossRepository struct {
	db *sql.DB
}

func NewBossRepository(db *sql.DB) (*BossRepository, error) {
	if db == nil {
		return nil, errors.New("database is nil")
	}
	return &BossRepository{db: db}, nil
}

// SaveUnsealedAt upserts the single-row demon king unseal clock inside the active transaction or DB.
func (r *BossRepository) SaveUnsealedAt(ctx context.Context, unsealedAt time.Time) error {
	const query = `
		INSERT INTO boss_unseal_clock (id, unsealed_at)
		VALUES (1, ?)
		ON DUPLICATE KEY UPDATE unsealed_at = VALUES(unsealed_at)
	`
	if _, err := ExecutorFromContext(ctx, r.db).ExecContext(ctx, query, unsealedAt.UTC()); err != nil {
		return fmt.Errorf("failed to save boss unsealed_at: %w", err)
	}
	return nil
}

// GetUnsealedAt reads the single-row demon king unseal clock, returning (nil, nil) if never unsealed.
func (r *BossRepository) GetUnsealedAt(ctx context.Context) (*time.Time, error) {
	const query = `SELECT unsealed_at FROM boss_unseal_clock WHERE id = 1`
	var unsealedAt time.Time
	if err := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, query).Scan(&unsealedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get boss unsealed_at: %w", err)
	}
	t := unsealedAt.UTC()
	return &t, nil
}
