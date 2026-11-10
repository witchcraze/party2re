package database

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/witchcraze/party2re/internal/rescue"
)

type RescueRepository struct {
	db *sql.DB
}

func NewRescueRepository(db *sql.DB) (*RescueRepository, error) {
	if db == nil {
		return nil, errors.New("database is nil")
	}
	return &RescueRepository{db: db}, nil
}

func (r *RescueRepository) Save(ctx context.Context, rec rescue.RescueRecord) error {
	_, err := ExecutorFromContext(ctx, r.db).ExecContext(ctx, `
		INSERT INTO rescue_records (id, character_id, reason, penalty_seconds, created_at, sleep_until)
		VALUES (?, ?, ?, ?, ?, ?)
	`, rec.ID, rec.CharacterID, rec.Reason, rec.PenaltySeconds, rec.CreatedAt, rec.SleepUntil)
	return err
}

func (r *RescueRepository) MarkApplied(ctx context.Context, id string, at time.Time) error {
	_, err := ExecutorFromContext(ctx, r.db).ExecContext(ctx,
		`UPDATE rescue_records SET applied_at = ? WHERE id = ? AND applied_at IS NULL`, at, id)
	return err
}

func (r *RescueRepository) FindLatestByCharacterID(ctx context.Context, characterID string) (rescue.RescueRecord, error) {
	var rec rescue.RescueRecord
	var appliedAt sql.NullTime
	// An unconfirmed record sorts first so rescue always resumes it.
	err := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, `
		SELECT id, character_id, reason, penalty_seconds, created_at, sleep_until, applied_at
		FROM rescue_records
		WHERE character_id = ?
		ORDER BY applied_at IS NULL DESC, created_at DESC
		LIMIT 1
	`, characterID).Scan(&rec.ID, &rec.CharacterID, &rec.Reason, &rec.PenaltySeconds, &rec.CreatedAt, &rec.SleepUntil, &appliedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return rescue.RescueRecord{}, rescue.ErrNoRescueRecord
	}
	if err != nil {
		return rescue.RescueRecord{}, err
	}
	rec.AppliedAt = appliedAt.Time
	return rec, nil
}
