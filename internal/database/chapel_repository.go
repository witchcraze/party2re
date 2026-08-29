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
		SELECT character_id, active_blessing, donation_gold_total, prayed_at, updated_at
		FROM character_blessings
		WHERE character_id = ?
	`, characterID).Scan(&b.CharacterID, &b.ActiveBlessing, &b.DonationGoldTotal, &b.PrayedAt, &b.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return chapel.CharacterBlessing{
			CharacterID:       characterID,
			ActiveBlessing:    chapel.BlessingNone,
			DonationGoldTotal: 0,
			PrayedAt:          time.Now().UTC(),
			UpdatedAt:         time.Now().UTC(),
		}, nil
	}
	if err != nil {
		return chapel.CharacterBlessing{}, err
	}
	return b, nil
}

func (r *ChapelRepository) SelectBlessing(ctx context.Context, characterID string, blessing chapel.BlessingType) (chapel.CharacterBlessing, error) {
	now := time.Now().UTC()
	_, err := ExecutorFromContext(ctx, r.db).ExecContext(ctx, `
		INSERT INTO character_blessings (
			character_id, active_blessing, donation_gold_total, prayed_at, updated_at
		) VALUES (?, ?, 0, ?, ?)
		ON DUPLICATE KEY UPDATE
			active_blessing = VALUES(active_blessing),
			prayed_at = VALUES(prayed_at),
			updated_at = VALUES(updated_at)
	`, characterID, blessing, now, now)
	if err != nil {
		return chapel.CharacterBlessing{}, err
	}
	return r.GetBlessing(ctx, characterID)
}

func (r *ChapelRepository) Donate(ctx context.Context, characterID string, goldAmount int) (chapel.CharacterBlessing, error) {
	err := RunInTx(ctx, r.db, func(txCtx context.Context) error {
		executor := ExecutorFromContext(txCtx, r.db)

		// 1. Deduct gold from character
		res, err := executor.ExecContext(txCtx, `
			UPDATE characters
			SET money = money - ?
			WHERE id = ? AND money >= ?
		`, goldAmount, characterID, goldAmount)
		if err != nil {
			return err
		}
		affected, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if affected == 0 {
			return chapel.ErrInsufficientGold
		}

		// 2. Increment donation total
		now := time.Now().UTC()
		_, err = executor.ExecContext(txCtx, `
			INSERT INTO character_blessings (
				character_id, active_blessing, donation_gold_total, prayed_at, updated_at
			) VALUES (?, 'NONE', ?, ?, ?)
			ON DUPLICATE KEY UPDATE
				donation_gold_total = donation_gold_total + VALUES(donation_gold_total),
				updated_at = VALUES(updated_at)
		`, characterID, goldAmount, now, now)
		if err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return chapel.CharacterBlessing{}, err
	}

	return r.GetBlessing(ctx, characterID)
}
