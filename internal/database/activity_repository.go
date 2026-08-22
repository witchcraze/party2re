package database

import (
	"context"
	"database/sql"
	"errors"

	"github.com/witchcraze/party2re/internal/activity"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

type ActivityRepository struct {
	db *sql.DB
}

func NewActivityRepository(db *sql.DB) (*ActivityRepository, error) {
	if db == nil {
		return nil, errors.New("database is nil")
	}
	return &ActivityRepository{db: db}, nil
}

func (r *ActivityRepository) Save(ctx context.Context, value activity.Activity) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO activities
			(id, character_id, activity_type, started_at, available_at, experience_reward, claimed)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, value.ID, value.CharacterID, value.Type, value.StartedAt, value.AvailableAt, value.ExperienceReward, value.Claimed)
	return err
}

func (r *ActivityRepository) FindByID(ctx context.Context, id string) (activity.Activity, error) {
	var value activity.Activity
	err := r.db.QueryRowContext(ctx, `
		SELECT id, character_id, activity_type, started_at, available_at, experience_reward, claimed
		FROM activities
		WHERE id = ?
	`, id).Scan(
		&value.ID,
		&value.CharacterID,
		&value.Type,
		&value.StartedAt,
		&value.AvailableAt,
		&value.ExperienceReward,
		&value.Claimed,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return activity.Activity{}, activity.ErrNotFound
	}
	if err != nil {
		return activity.Activity{}, err
	}
	return value, nil
}

func (r *ActivityRepository) ClaimAndApply(ctx context.Context, id string, character corecharacter.Character) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	result, err := tx.ExecContext(ctx, `
		UPDATE activities
		SET claimed = TRUE
		WHERE id = ? AND claimed = FALSE
	`, id)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return claimFailure(ctx, tx, "activities", id, activity.ErrNotFound, activity.ErrAlreadyClaimed)
	}
	if err := updateCharacterAtomically(ctx, tx, character); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	committed = true
	return nil
}
