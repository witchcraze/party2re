package scheduling

import (
	"errors"
	"time"
)

type State string

const (
	StatePending    State = "pending"
	StateProcessing State = "processing"
	StateCompleted  State = "completed"
	StateFailed     State = "failed"
)

type ScheduledAction struct {
	ID          string
	ActionType  string
	ActorID     string
	Params      map[string]string
	ScheduledAt time.Time
	ExecuteAt   time.Time
	State       State
	AttemptedAt *time.Time
	CompletedAt *time.Time
	RetainUntil time.Time
}

var (
	ErrInvalidStateTransition = errors.New("invalid state transition")
)

func (a *ScheduledAction) MarkProcessing() error {
	if a.State != StatePending {
		return ErrInvalidStateTransition
	}
	a.State = StateProcessing
	now := time.Now()
	a.AttemptedAt = &now
	return nil
}

func (a *ScheduledAction) MarkCompleted(retention time.Duration) error {
	if a.State != StateProcessing {
		return ErrInvalidStateTransition
	}
	a.State = StateCompleted
	now := time.Now()
	a.CompletedAt = &now
	a.RetainUntil = now.Add(retention)
	return nil
}

func (a *ScheduledAction) MarkFailed() error {
	if a.State != StateProcessing {
		return ErrInvalidStateTransition
	}
	a.State = StateFailed
	// Could also set CompletedAt or RetainUntil for failures if we want to retain failed actions
	return nil
}
