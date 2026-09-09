package database

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/witchcraze/party2re/internal/chapel"
)

type ChapelRepository struct {
	db *sql.DB
}

func NewChapelRepository(db *sql.DB) (*ChapelRepository, error) {
	if db == nil {
		return nil, errors.New("database is nil")
	}
	return &ChapelRepository{db: db}, nil
}

func (r *ChapelRepository) GetBlessing(ctx context.Context, characterID string) (chapel.CharacterBlessing, error) {
	var b chapel.CharacterBlessing
	err := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, `
		SELECT character_id, active_blessing, prayed_at, updated_at
		FROM character_blessings
		WHERE character_id = ?
	`, characterID).Scan(&b.CharacterID, &b.ActiveBlessing, &b.PrayedAt, &b.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return chapel.CharacterBlessing{
			CharacterID:    characterID,
			ActiveBlessing: chapel.BlessingNone,
			PrayedAt:       time.Now().UTC(),
			UpdatedAt:      time.Now().UTC(),
		}, nil
	}
	if err != nil {
		return chapel.CharacterBlessing{}, err
	}
	return b, nil
}

func (r *ChapelRepository) SelectBlessing(ctx context.Context, characterID string, blessing chapel.BlessingType) (chapel.CharacterBlessing, error) {
	now := time.Now().UTC()
	err := RunInTx(ctx, r.db, func(txCtx context.Context) error {
		executor := ExecutorFromContext(txCtx, r.db)

		var active chapel.BlessingType
		err := executor.QueryRowContext(txCtx, `
			SELECT active_blessing
			FROM character_blessings
			WHERE character_id = ?
			FOR UPDATE
		`, characterID).Scan(&active)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if err == nil && active != chapel.BlessingNone && active != "" {
			return chapel.ErrAlreadyPrayed
		}

		_, err = executor.ExecContext(txCtx, `
			INSERT INTO character_blessings (
				character_id, active_blessing, prayed_at, updated_at
			) VALUES (?, ?, ?, ?)
			ON DUPLICATE KEY UPDATE
				active_blessing = VALUES(active_blessing),
				prayed_at = VALUES(prayed_at),
				updated_at = VALUES(updated_at)
		`, characterID, blessing, now, now)
		return err
	})
	if err != nil {
		return chapel.CharacterBlessing{}, err
	}
	return r.GetBlessing(ctx, characterID)
}

func (r *ChapelRepository) ClearBlessing(ctx context.Context, characterID string) error {
	now := time.Now().UTC()
	_, err := ExecutorFromContext(ctx, r.db).ExecContext(ctx, `
		UPDATE character_blessings
		SET active_blessing = 'NONE', updated_at = ?
		WHERE character_id = ?
	`, now, characterID)
	return err
}

func (r *ChapelRepository) ClearAllBlessings(ctx context.Context) error {
	now := time.Now().UTC()
	_, err := ExecutorFromContext(ctx, r.db).ExecContext(ctx, `
		UPDATE character_blessings
		SET active_blessing = 'NONE', updated_at = ?
		WHERE active_blessing != 'NONE'
	`, now)
	return err
}
