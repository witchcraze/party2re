package database

import (
	"context"
	"database/sql"
	"errors"

	corejob "github.com/witchcraze/party2re/internal/core/job"
)

var ErrCharacterJobNotFound = errors.New("character job not found")

type CharacterJobRepository struct {
	db *sql.DB
}

func NewCharacterJobRepository(db *sql.DB) (*CharacterJobRepository, error) {
	if db == nil {
		return nil, errors.New("database is nil")
	}
	return &CharacterJobRepository{db: db}, nil
}

func (r *CharacterJobRepository) Save(ctx context.Context, value corejob.CharacterJob) error {
	return RunInTx(ctx, r.db, func(txCtx context.Context) error {
		executor := ExecutorFromContext(txCtx, r.db)

		if _, err := executor.ExecContext(txCtx, `
			INSERT INTO character_jobs (character_id, current_job_id)
			VALUES (?, ?)
			ON DUPLICATE KEY UPDATE current_job_id = VALUES(current_job_id)
		`, value.CharacterID, value.CurrentJobID); err != nil {
			return err
		}
		if _, err := executor.ExecContext(txCtx, "DELETE FROM character_job_history WHERE character_id = ?", value.CharacterID); err != nil {
			return err
		}
		for _, change := range value.History {
			if _, err := executor.ExecContext(txCtx, `
				INSERT INTO character_job_history (character_id, from_job_id, to_job_id)
				VALUES (?, ?, ?)
			`, value.CharacterID, change.FromJobID, change.ToJobID); err != nil {
				return err
			}
		}
		if _, err := executor.ExecContext(txCtx, "DELETE FROM character_job_masteries WHERE character_id = ?", value.CharacterID); err != nil {
			return err
		}
		for _, jobID := range value.MasteredJobs {
			if _, err := executor.ExecContext(txCtx, `
				INSERT INTO character_job_masteries (character_id, job_id)
				VALUES (?, ?)
			`, value.CharacterID, jobID); err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *CharacterJobRepository) FindByCharacterID(ctx context.Context, characterID string) (corejob.CharacterJob, error) {
	var value corejob.CharacterJob
	executor := ExecutorFromContext(ctx, r.db)
	err := executor.QueryRowContext(ctx, `
		SELECT character_id, current_job_id
		FROM character_jobs
		WHERE character_id = ?
	`, characterID).Scan(&value.CharacterID, &value.CurrentJobID)
	if errors.Is(err, sql.ErrNoRows) {
		return corejob.CharacterJob{}, ErrCharacterJobNotFound
	}
	if err != nil {
		return corejob.CharacterJob{}, err
	}
	rows, err := executor.QueryContext(ctx, `
		SELECT from_job_id, to_job_id
		FROM character_job_history
		WHERE character_id = ?
		ORDER BY id
	`, characterID)
	if err != nil {
		return corejob.CharacterJob{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var change corejob.Change
		if err := rows.Scan(&change.FromJobID, &change.ToJobID); err != nil {
			return corejob.CharacterJob{}, err
		}
		value.History = append(value.History, change)
	}
	if err := rows.Err(); err != nil {
		return corejob.CharacterJob{}, err
	}

	masteryRows, err := executor.QueryContext(ctx, `
		SELECT job_id
		FROM character_job_masteries
		WHERE character_id = ?
		ORDER BY job_id
	`, characterID)
	if err != nil {
		return corejob.CharacterJob{}, err
	}
	defer masteryRows.Close()
	for masteryRows.Next() {
		var jobID string
		if err := masteryRows.Scan(&jobID); err != nil {
			return corejob.CharacterJob{}, err
		}
		value.MasteredJobs = append(value.MasteredJobs, jobID)
	}
	if err := masteryRows.Err(); err != nil {
		return corejob.CharacterJob{}, err
	}

	return value, nil
}
