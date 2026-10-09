package chapel

import (
	"context"
	"time"

	core_scheduling "github.com/witchcraze/party2re/internal/core/scheduling"
)

const (
	// ActionTypeChapelReset is the scheduled action type for resetting character blessings.
	ActionTypeChapelReset = "chapel_reset"
)

// Scheduler defines the scheduling capability needed for recurring actions.
type Scheduler interface {
	ScheduleWithID(ctx context.Context, id, actionType, actorID string, params map[string]string, executeAt time.Time) error
}

// NextMidnightJST returns the next 00:00:00 in Japan Standard Time (UTC+9) after t.
func NextMidnightJST(t time.Time) time.Time {
	jst := time.FixedZone("JST", 9*60*60)
	tJST := t.In(jst)
	return time.Date(tJST.Year(), tJST.Month(), tJST.Day()+1, 0, 0, 0, 0, jst)
}

// DailyResetActionID returns the deterministic scheduled action ID for a given JST midnight time.
func DailyResetActionID(targetTime time.Time) string {
	jst := time.FixedZone("JST", 9*60*60)
	return "chapel_reset:" + targetTime.In(jst).Format("2006-01-02")
}

// ResetOption configures a ResetHandler.
type ResetOption func(*ResetHandler)

// WithScheduler configures the scheduler for a ResetHandler (retained for backward compatibility).
func WithScheduler(scheduler Scheduler) ResetOption {
	return func(h *ResetHandler) {
	}
}

// ResetHandler implements scheduling.ActionHandler for residual chapel_reset actions.
// Under Issue #1163, daily midnight reset of blessings has been eliminated in favor
// of sleep-based lifecycle clearing. Any residual scheduled action dispatched here
// is handled as a no-op migration without clearing blessings or re-enqueuing.
type ResetHandler struct{}

// NewResetHandler creates a new ResetHandler.
func NewResetHandler(_ *Service, _ ...ResetOption) *ResetHandler {
	return &ResetHandler{}
}

// Handle executes residual scheduled actions safely without modifying blessings or re-enqueuing.
func (h *ResetHandler) Handle(ctx context.Context, action core_scheduling.ScheduledAction) error {
	return nil
}
