package database

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/witchcraze/party2re/internal/eventplaza"
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

func (r *EventPlazaRepository) CountActiveParticipants(ctx context.Context) (int, error) {
	var count int
	err := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, `
		SELECT COUNT(*) FROM characters
	`).Scan(&count)
	if err != nil {
		return 0, err
	}
	return count, nil
}

func (r *EventPlazaRepository) SaveBanquet(ctx context.Context, banquet eventplaza.CelebrationBanquet) error {
	_, err := ExecutorFromContext(ctx, r.db).ExecContext(ctx, `
		INSERT INTO celebration_banquets (
			id, boss_id, boss_name, slayer_character_id, slayer_character_name,
			tier, toast_count, celebrated_at, expires_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		banquet.ID,
		banquet.BossID,
		banquet.BossName,
		banquet.SlayerCharacterID,
		banquet.SlayerCharacterName,
		banquet.Tier,
		banquet.ToastCount,
		banquet.CelebratedAt,
		banquet.ExpiresAt,
	)
	return err
}

func (r *EventPlazaRepository) FindBanquetByID(ctx context.Context, id string) (eventplaza.CelebrationBanquet, error) {
	var b eventplaza.CelebrationBanquet
	err := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, `
		SELECT id, boss_id, boss_name, slayer_character_id, slayer_character_name,
		       tier, toast_count, celebrated_at, expires_at
		FROM celebration_banquets
		WHERE id = ?
	`, id).Scan(
		&b.ID,
		&b.BossID,
		&b.BossName,
		&b.SlayerCharacterID,
		&b.SlayerCharacterName,
		&b.Tier,
		&b.ToastCount,
		&b.CelebratedAt,
		&b.ExpiresAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return eventplaza.CelebrationBanquet{}, eventplaza.ErrBanquetNotFound
		}
		return eventplaza.CelebrationBanquet{}, err
	}
	return b, nil
}

func (r *EventPlazaRepository) ListActiveBanquets(ctx context.Context, now time.Time) ([]eventplaza.CelebrationBanquet, error) {
	rows, err := ExecutorFromContext(ctx, r.db).QueryContext(ctx, `
		SELECT id, boss_id, boss_name, slayer_character_id, slayer_character_name,
		       tier, toast_count, celebrated_at, expires_at
		FROM celebration_banquets
		WHERE expires_at > ?
		ORDER BY celebrated_at DESC
	`, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var banquets []eventplaza.CelebrationBanquet
	for rows.Next() {
		var b eventplaza.CelebrationBanquet
		if err := rows.Scan(
			&b.ID,
			&b.BossID,
			&b.BossName,
			&b.SlayerCharacterID,
			&b.SlayerCharacterName,
			&b.Tier,
			&b.ToastCount,
			&b.CelebratedAt,
			&b.ExpiresAt,
		); err != nil {
			return nil, err
		}
		banquets = append(banquets, b)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return banquets, nil
}

func (r *EventPlazaRepository) RecordToast(ctx context.Context, banquetID string, characterID string, toastedAt time.Time) error {
	return RunInTx(ctx, r.db, func(txCtx context.Context) error {
		exec := ExecutorFromContext(txCtx, r.db)
		_, err := exec.ExecContext(txCtx, `
			INSERT INTO banquet_toasts (banquet_id, character_id, toasted_at)
			VALUES (?, ?, ?)
		`, banquetID, characterID, toastedAt)
		if err != nil {
			return err
		}

		_, err = exec.ExecContext(txCtx, `
			UPDATE celebration_banquets
			SET toast_count = toast_count + 1
			WHERE id = ?
		`, banquetID)
		return err
	})
}

func (r *EventPlazaRepository) HasToasted(ctx context.Context, banquetID string, characterID string) (bool, error) {
	var count int
	err := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM banquet_toasts
		WHERE banquet_id = ? AND character_id = ?
	`, banquetID, characterID).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}
