package database

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/witchcraze/party2re/internal/chapel"
)

type ChapelRepository struct {
	db      *sql.DB
	nowFunc func() time.Time
}

func (r *ChapelRepository) now() time.Time {
	if r.nowFunc != nil {
		return r.nowFunc()
	}
	return time.Now()
}

// ChapelRepoOption configures a ChapelRepository.
type ChapelRepoOption func(*ChapelRepository)

// WithChapelNowFunc sets a custom clock for deterministic testing.
func WithChapelNowFunc(fn func() time.Time) ChapelRepoOption {
	return func(r *ChapelRepository) {
		r.nowFunc = fn
	}
}

func NewChapelRepository(db *sql.DB, opts ...ChapelRepoOption) (*ChapelRepository, error) {
	if db == nil {
		return nil, errors.New("database is nil")
	}
	r := &ChapelRepository{
		db:      db,
		nowFunc: time.Now,
	}
	for _, opt := range opts {
		opt(r)
	}
	return r, nil
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
			PrayedAt:       time.Time{},
			UpdatedAt:      time.Time{},
		}, nil
	}
	if err != nil {
		return chapel.CharacterBlessing{}, err
	}
	return b, nil
}

func (r *ChapelRepository) SelectBlessing(ctx context.Context, characterID string, blessing chapel.BlessingType) (chapel.CharacterBlessing, error) {
	now := r.now().UTC()
	err := RunInTx(ctx, r.db, func(txCtx context.Context) error {
		executor := ExecutorFromContext(txCtx, r.db)

		// 1. Lock the character primary entity (Rank 2) to serialize concurrent operations
		// and eliminate gap-lock deadlocks on subsequent character_blessings queries/inserts.
		var charID string
		err := executor.QueryRowContext(txCtx, `
			SELECT id
			FROM characters
			WHERE id = ?
			FOR UPDATE
		`, characterID).Scan(&charID)
		if errors.Is(err, sql.ErrNoRows) {
			return chapel.ErrInvalidCharacterID
		}
		if err != nil {
			return err
		}

		var active chapel.BlessingType
		var prayedAt time.Time
		err = executor.QueryRowContext(txCtx, `
			SELECT active_blessing, prayed_at
			FROM character_blessings
			WHERE character_id = ?
		`, characterID).Scan(&active, &prayedAt)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if err == nil {
			if (active != chapel.BlessingNone && active != "") || !prayedAt.IsZero() {
				return chapel.ErrAlreadyPrayed
			}
			_, err = executor.ExecContext(txCtx, `
				UPDATE character_blessings
				SET active_blessing = ?, prayed_at = ?, updated_at = ?
				WHERE character_id = ?
			`, blessing, now, now, characterID)
			return err
		}

		_, err = executor.ExecContext(txCtx, `
			INSERT INTO character_blessings (
				character_id, active_blessing, prayed_at, updated_at
			) VALUES (?, ?, ?, ?)
		`, characterID, blessing, now, now)
		if err != nil {
			var mysqlErr *mysql.MySQLError
			if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
				return chapel.ErrAlreadyPrayed
			}
			if strings.Contains(err.Error(), "Duplicate entry") {
				return chapel.ErrAlreadyPrayed
			}
			return err
		}
		return nil
	})
	if err != nil {
		return chapel.CharacterBlessing{}, err
	}
	return r.GetBlessing(ctx, characterID)
}

func (r *ChapelRepository) ClearBlessing(ctx context.Context, characterID string) error {
	now := r.now().UTC()
	return RunInTx(ctx, r.db, func(txCtx context.Context) error {
		executor := ExecutorFromContext(txCtx, r.db)

		// 1. Lock character primary entity (Rank 2)
		var charID string
		err := executor.QueryRowContext(txCtx, `
			SELECT id
			FROM characters
			WHERE id = ?
			FOR UPDATE
		`, characterID).Scan(&charID)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}

		var active chapel.BlessingType
		var prayedAt time.Time
		err = executor.QueryRowContext(txCtx, `
			SELECT active_blessing, prayed_at
			FROM character_blessings
			WHERE character_id = ?
		`, characterID).Scan(&active, &prayedAt)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}

		// If prayed on the same JST calendar day, clear active blessing effect
		// but keep the prayed_at record to prevent re-praying on the same day.
		if chapel.IsSameJSTDay(prayedAt, now) {
			_, err = executor.ExecContext(txCtx, `
				UPDATE character_blessings
				SET active_blessing = 'NONE', updated_at = ?
				WHERE character_id = ?
			`, now, characterID)
			return err
		}

		// If prayed on an earlier day, clear both the effect and the quota record on sleep.
		_, err = executor.ExecContext(txCtx, `
			DELETE FROM character_blessings
			WHERE character_id = ?
		`, characterID)
		return err
	})
}
