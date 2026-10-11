package database

import (
	"context"
	"database/sql"
	"time"
)

// GetWeeklyRotationDue reads durable rotation progress under the caller's weekly snapshot lock.
// It deliberately does not use calculated_at, which ordinary snapshot refreshes also update.
func (r *RankingRepository) GetWeeklyRotationDue(ctx context.Context) (time.Time, error) {
	var due sql.NullTime
	err := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx,
		`SELECT weekly_rotation_due FROM ranking_snapshots WHERE ranking_type = 'weekly_job_change'`).Scan(&due)
	return due.Time.UTC(), err
}

// SaveWeeklyRotationDue advances progress in the same transaction as snapshot persistence and reset.
func (r *RankingRepository) SaveWeeklyRotationDue(ctx context.Context, due time.Time) error {
	_, err := ExecutorFromContext(ctx, r.db).ExecContext(ctx,
		`UPDATE ranking_snapshots SET weekly_rotation_due = ? WHERE ranking_type = 'weekly_job_change'`, due.UTC())
	return err
}
