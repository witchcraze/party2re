package scheduling

import (
	"context"
	"time"
)

// ScheduledActionRepository defines the data access methods for scheduled actions.
type ScheduledActionRepository interface {
	// Schedule adds a new action to be executed in the future.
	Schedule(ctx context.Context, action ScheduledAction) error

	// FetchDue returns actions that are due for execution at or before the given time.
	// It may limit the number of returned actions to prevent overwhelming the worker.
	FetchDue(ctx context.Context, upTo time.Time, limit int) ([]ScheduledAction, error)

	// AcquireLock attempts to lock the action for processing.
	// Returns true if lock was acquired, false if already locked/processed.
	AcquireLock(ctx context.Context, actionID string, lockTTL time.Duration) (bool, error)

	// Save updates an action's state (e.g. after processing).
	// If it's completed/failed, it might move it to a different storage or set a TTL.
	Save(ctx context.Context, action ScheduledAction) error
}
