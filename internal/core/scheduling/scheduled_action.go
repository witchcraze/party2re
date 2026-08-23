package scheduling

import (
	"errors"
	"fmt"
	"time"
)

type State string

const (
	StatePending    State = "pending"
	StateProcessing State = "processing"
	StateCompleted  State = "completed"
	StateFailed     State = "failed"

	// MaxIDLength caps the ID field to prevent oversized keys in Valkey.
	MaxIDLength = 128
	// MaxActionTypeLength caps the action type used as a dispatch key.
	MaxActionTypeLength = 64
	// MaxActorIDLength caps the actor identifier.
	MaxActorIDLength = 128
	// MaxParamCount caps the number of parameters to limit memory usage.
	MaxParamCount = 32
	// MaxParamKeyLength caps each parameter key.
	MaxParamKeyLength = 64
	// MaxParamValueLength caps each parameter value.
	MaxParamValueLength = 512
)

// knownStates is the allow-list of valid State values.
var knownStates = map[State]struct{}{
	StatePending:    {},
	StateProcessing: {},
	StateCompleted:  {},
	StateFailed:     {},
}

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
	ErrInvalidAction          = errors.New("invalid scheduled action")
)

// Validate checks that a ScheduledAction loaded from external storage is
// internally consistent and within field-size limits. It returns a wrapped
// ErrInvalidAction on any violation so callers can skip or quarantine the
// record without panicking.
func (a *ScheduledAction) Validate() error {
	if a.ID == "" {
		return fmt.Errorf("%w: empty ID", ErrInvalidAction)
	}
	if len(a.ID) > MaxIDLength {
		return fmt.Errorf("%w: ID exceeds %d characters", ErrInvalidAction, MaxIDLength)
	}
	if a.ActionType == "" {
		return fmt.Errorf("%w: empty ActionType", ErrInvalidAction)
	}
	if len(a.ActionType) > MaxActionTypeLength {
		return fmt.Errorf("%w: ActionType exceeds %d characters", ErrInvalidAction, MaxActionTypeLength)
	}
	if a.ActorID == "" {
		return fmt.Errorf("%w: empty ActorID", ErrInvalidAction)
	}
	if len(a.ActorID) > MaxActorIDLength {
		return fmt.Errorf("%w: ActorID exceeds %d characters", ErrInvalidAction, MaxActorIDLength)
	}
	if _, ok := knownStates[a.State]; !ok {
		return fmt.Errorf("%w: unknown state %q", ErrInvalidAction, a.State)
	}
	if a.ExecuteAt.IsZero() {
		return fmt.Errorf("%w: zero ExecuteAt", ErrInvalidAction)
	}
	if len(a.Params) > MaxParamCount {
		return fmt.Errorf("%w: Params count %d exceeds maximum %d", ErrInvalidAction, len(a.Params), MaxParamCount)
	}
	for k, v := range a.Params {
		if len(k) > MaxParamKeyLength {
			return fmt.Errorf("%w: Params key exceeds %d characters", ErrInvalidAction, MaxParamKeyLength)
		}
		if len(v) > MaxParamValueLength {
			return fmt.Errorf("%w: Params value for key %q exceeds %d characters", ErrInvalidAction, k, MaxParamValueLength)
		}
	}
	return nil
}

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

func (a *ScheduledAction) MarkFailed(retention time.Duration) error {
	if a.State != StateProcessing {
		return ErrInvalidStateTransition
	}
	a.State = StateFailed
	now := time.Now()
	a.CompletedAt = &now
	a.RetainUntil = now.Add(retention)
	return nil
}

