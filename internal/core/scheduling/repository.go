package scheduling

import (
	"context"
	"time"
)

// ScheduledActionRepository defines the data access methods for scheduled actions.
type ScheduledActionRepository interface {
	// Schedule adds a new action to be executed in the future.
	Schedule(ctx context.Context, action ScheduledAction) error

	// FindPendingByActorID returns unfinished (Pending or Processing) work for an actor,
	// including overdue actions awaiting settlement. Missing payloads are skipped;
	// storage failures or invalid payloads return an error without partial results.
	FindPendingByActorID(ctx context.Context, actorID string) ([]ScheduledAction, error)

	// FetchDue returns actions that are due for execution at or before the given time.
	// It traverses the pending queue in bounded batches, excluding preserved Processing
	// records from returned candidates while retaining their authoritative state and
	// actor discovery. It limits returned actions to prevent overwhelming the worker.
	// Stale entries with genuinely absent payloads are cleaned up from the queue.
	// Transient storage/read failures preserve queued entries and return an error.
	// Queued terminal records may be returned for metadata-only finalization;
	// they must never be dispatched to feature handlers again.
	FetchDue(ctx context.Context, upTo time.Time, limit int) ([]ScheduledAction, error)

	// AcquireLock attempts to lock the action for processing.
	// Returns true if lock was acquired, false if already locked/processed.
	AcquireLock(ctx context.Context, actionID string, lockTTL time.Duration) (bool, error)

	// Save updates an action's state. Terminal payload persistence precedes cleanup;
	// a payload write failure leaves unfinished discovery/coordination untouched.
	// Cleanup failures propagate after the terminal outcome has been persisted.
	// Queue membership is removed last so terminal metadata can be retried without
	// handler replay. Retention is based on the original RetainUntil, not retry time.
	Save(ctx context.Context, action ScheduledAction) error

	// CancelByActorID removes and cancels all pending/scheduled actions for the specified actor.
	// Halts and propagates storage failures without deleting the actor index for unresolved work.
	// Returns the number of successfully removed actions and any storage error encountered.
	CancelByActorID(ctx context.Context, actorID string) (int, error)
}
