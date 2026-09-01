package database

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/witchcraze/party2re/internal/maintenance"
)

type MaintenanceRepository struct {
	db *sql.DB
}

func NewMaintenanceRepository(db *sql.DB) (*MaintenanceRepository, error) {
	if db == nil {
		return nil, errors.New("database is nil")
	}
	return &MaintenanceRepository{db: db}, nil
}

func (r *MaintenanceRepository) GetStatus(ctx context.Context) (maintenance.Status, error) {
	var (
		id               string
		isEnabled        bool
		message          string
		estimatedEndTime sql.NullTime
		updatedAt        time.Time
	)
	err := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, `
		SELECT id, is_enabled, message, estimated_end_time, updated_at
		FROM system_maintenance
		WHERE id = 'global'
	`).Scan(&id, &isEnabled, &message, &estimatedEndTime, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return maintenance.Status{
			Enabled:   false,
			Message:   "System is operating normally.",
			UpdatedAt: time.Now().UTC(),
		}, nil
	}
	if err != nil {
		return maintenance.Status{}, err
	}
	var endTime *time.Time
	if estimatedEndTime.Valid {
		t := estimatedEndTime.Time.UTC()
		endTime = &t
	}
	return maintenance.Status{
		Enabled:          isEnabled,
		Message:          message,
		EstimatedEndTime: endTime,
		UpdatedAt:        updatedAt.UTC(),
	}, nil
}

func (r *MaintenanceRepository) SetStatus(ctx context.Context, status maintenance.Status) error {
	var estimatedEndTime sql.NullTime
	if status.EstimatedEndTime != nil {
		estimatedEndTime = sql.NullTime{Time: status.EstimatedEndTime.UTC(), Valid: true}
	}
	_, err := ExecutorFromContext(ctx, r.db).ExecContext(ctx, `
		INSERT INTO system_maintenance (id, is_enabled, message, estimated_end_time, updated_at)
		VALUES ('global', ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
			is_enabled = VALUES(is_enabled),
			message = VALUES(message),
			estimated_end_time = VALUES(estimated_end_time),
			updated_at = VALUES(updated_at)
	`, status.Enabled, status.Message, estimatedEndTime, status.UpdatedAt.UTC())
	return err
}
