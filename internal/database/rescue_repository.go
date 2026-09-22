package database

import (
	"context"
	"database/sql"
	"errors"

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
		INSERT INTO rescue_records (id, character_id, reason, penalty_seconds, created_at)
		VALUES (?, ?, ?, ?, ?)
	`, rec.ID, rec.CharacterID, rec.Reason, rec.PenaltySeconds, rec.CreatedAt)
	return err
}

func (r *RescueRepository) FindLatestByCharacterID(ctx context.Context, characterID string) (rescue.RescueRecord, error) {
	var rec rescue.RescueRecord
	err := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, `
		SELECT id, character_id, reason, penalty_seconds, created_at
		FROM rescue_records
		WHERE character_id = ?
		ORDER BY created_at DESC
		LIMIT 1
	`, characterID).Scan(&rec.ID, &rec.CharacterID, &rec.Reason, &rec.PenaltySeconds, &rec.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return rescue.RescueRecord{}, rescue.ErrNoRescueRecord
	}
	if err != nil {
		return rescue.RescueRecord{}, err
	}
	return rec, nil
}
