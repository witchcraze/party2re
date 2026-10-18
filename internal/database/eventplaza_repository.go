package database

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

type EventPlazaRepository struct {
	db *sql.DB
}

func NewEventPlazaRepository(db *sql.DB) (*EventPlazaRepository, error) {
	if db == nil {
		return nil, errors.New("database is nil")
	}
	return &EventPlazaRepository{db: db}, nil
}

func (r *EventPlazaRepository) CountActiveParticipants(ctx context.Context, cutoff time.Time) (int, error) {
	var count int
	err := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, `
		SELECT COUNT(*) FROM eventplaza_presences WHERE last_seen_at >= ?
	`, cutoff).Scan(&count)
	if err != nil {
		return 0, err
	}
	return count, nil
}

func (r *EventPlazaRepository) RecordPresence(ctx context.Context, characterID string, at time.Time) error {
	_, err := ExecutorFromContext(ctx, r.db).ExecContext(ctx, `
		INSERT INTO eventplaza_presences (character_id, last_seen_at)
		VALUES (?, ?)
		ON DUPLICATE KEY UPDATE last_seen_at = VALUES(last_seen_at)
	`, characterID, at)
	return err
}
