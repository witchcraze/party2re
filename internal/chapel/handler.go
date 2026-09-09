package chapel

import (
	"context"
	"strings"
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

// WithScheduler configures the scheduler used to re-enqueue daily recurring resets.
func WithScheduler(scheduler Scheduler) ResetOption {
	return func(h *ResetHandler) {
		h.scheduler = scheduler
	}
}

// ResetHandler implements scheduling.ActionHandler to reset character blessings.
type ResetHandler struct {
	service   *Service
	scheduler Scheduler
}

// NewResetHandler creates a new ResetHandler.
func NewResetHandler(service *Service, opts ...ResetOption) *ResetHandler {
	h := &ResetHandler{
		service: service,
	}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

// Handle executes the blessing reset logic.
func (h *ResetHandler) Handle(ctx context.Context, action core_scheduling.ScheduledAction) error {
	characterID := strings.TrimSpace(action.Params["character_id"])
	if characterID == "" && action.ActorID != "" && !strings.EqualFold(action.ActorID, "system") && !strings.EqualFold(action.ActorID, "all") {
		characterID = strings.TrimSpace(action.ActorID)
	}

	var resetErr error
	if characterID != "" && !strings.EqualFold(characterID, "all") && !strings.EqualFold(characterID, "system") {
		resetErr = h.service.ClearBlessing(ctx, characterID)
	} else {
		resetErr = h.service.ClearAllBlessings(ctx)
	}

	if resetErr != nil {
		return resetErr
	}

	// When scheduler is present and it is a system-wide reset, schedule the next day's reset.
	if h.scheduler != nil && (characterID == "" || strings.EqualFold(characterID, "all") || strings.EqualFold(characterID, "system")) {
		next := NextMidnightJST(time.Now())
		_ = h.scheduler.ScheduleWithID(ctx, DailyResetActionID(next), ActionTypeChapelReset, "system", nil, next)
	}

	return nil
}
