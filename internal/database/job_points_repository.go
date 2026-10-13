package database

import (
	"context"
	"slices"

	corejob "github.com/witchcraze/party2re/internal/core/job"
)

func (r *CharacterJobRepository) saveJobPoints(ctx context.Context, value corejob.CharacterJob) error {
	ids := make([]string, 0, len(value.RecordedJobSP))
	for id, sp := range value.RecordedJobSP {
		if id == "" || sp < 0 {
			return corejob.ErrInvalidCharacter
		}
		ids = append(ids, id)
	}
	slices.Sort(ids)
	for _, id := range ids {
		if _, err := ExecutorFromContext(ctx, r.db).ExecContext(ctx, `
			INSERT INTO character_job_points (character_id, job_id, earned_sp)
			VALUES (?, ?, ?)
			ON DUPLICATE KEY UPDATE earned_sp = VALUES(earned_sp)
		`, value.CharacterID, id, value.RecordedJobSP[id]); err != nil {
			return err
		}
	}
	return nil
}

func (r *CharacterJobRepository) loadJobPoints(ctx context.Context, value *corejob.CharacterJob) error {
	rows, err := ExecutorFromContext(ctx, r.db).QueryContext(ctx, `
		SELECT job_id, earned_sp FROM character_job_points
		WHERE character_id = ? ORDER BY job_id
	`, value.CharacterID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var sp int
		if err := rows.Scan(&id, &sp); err != nil {
			return err
		}
		if value.RecordedJobSP == nil {
			value.RecordedJobSP = make(map[string]int)
		}
		value.RecordedJobSP[id] = sp
	}
	return rows.Err()
}
