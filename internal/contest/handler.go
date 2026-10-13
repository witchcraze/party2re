package contest

import (
	"context"
	"fmt"
	"time"

	core_scheduling "github.com/witchcraze/party2re/internal/core/scheduling"
)

const (
	// ActionTypeContestSettlement is the scheduled action type for executing periodic contest settlements.
	ActionTypeContestSettlement = "contest_settlement"
)

// Scheduler defines the scheduling capability needed for recurring actions.
type Scheduler interface {
	ScheduleWithID(ctx context.Context, id, actionType, actorID string, params map[string]string, executeAt time.Time) error
}

// SettlementActionID returns the deterministic scheduled action ID for a contest round and execution time.
func SettlementActionID(round int, targetTime time.Time) string {
	return fmt.Sprintf("contest_settlement:%d:%d", round, targetTime.Unix())
}

// ParseSettlementActionID parses a deterministic contest settlement action ID into its round number and target execution time.
func ParseSettlementActionID(actionID string) (int, time.Time, bool) {
	var round int
	var unixSec int64
	n, err := fmt.Sscanf(actionID, "contest_settlement:%d:%d", &round, &unixSec)
	if err != nil || n != 2 || round <= 0 || unixSec <= 0 {
		return 0, time.Time{}, false
	}
	return round, time.Unix(unixSec, 0).UTC(), true
}

// SettlementOption configures a ContestSettlementHandler.
type SettlementOption func(*ContestSettlementHandler)

// WithScheduler configures the scheduler used to re-enqueue recurring contest settlements.
func WithScheduler(scheduler Scheduler) SettlementOption {
	return func(h *ContestSettlementHandler) {
		h.scheduler = scheduler
	}
}

// ContestSettlementHandler implements scheduling.ActionHandler to execute periodic contest settlements.
type ContestSettlementHandler struct {
	service   *Service
	scheduler Scheduler
}

// NewContestSettlementHandler creates a new ContestSettlementHandler.
func NewContestSettlementHandler(service *Service, opts ...SettlementOption) *ContestSettlementHandler {
	h := &ContestSettlementHandler{
		service: service,
	}
	for _, opt := range opts {
		opt(h)
	}
	if h.scheduler != nil && h.service != nil && h.service.scheduler == nil {
		h.service.scheduler = h.scheduler
	}
	return h
}

// Reconcile reconciles the recurring contest settlement schedule from current SQL round state.
func (h *ContestSettlementHandler) Reconcile(ctx context.Context) error {
	if h.service == nil {
		return nil
	}
	return h.service.ReconcileSettlementSchedule(ctx)
}

// Handle executes the contest settlement logic and schedules the next settlement check.
// If the action references an already-settled or already-postponed round, it avoids replaying
// completed settlements and ensures the current active/preparing round schedule is maintained.
func (h *ContestSettlementHandler) Handle(ctx context.Context, action core_scheduling.ScheduledAction) error {
	if round, targetTime, ok := ParseSettlementActionID(action.ID); ok && h.service != nil {
		r, err := h.service.GetRoundByNumber(ctx, round)
		if err == nil {
			if r.Status == StatusSettled {
				return h.service.ReconcileSettlementSchedule(ctx)
			}
			if (r.Status == StatusPreparing || r.Status == StatusActive) && targetTime.Before(r.EndTime) {
				return h.service.ReconcileSettlementSchedule(ctx)
			}
		}
	}

	result, err := h.service.SettleContest(ctx, false)
	if err != nil {
		return err
	}

	if h.scheduler != nil && !result.NextRoundEndTime.IsZero() {
		nextID := SettlementActionID(result.NextRound, result.NextRoundEndTime)
		if err := h.scheduler.ScheduleWithID(ctx, nextID, ActionTypeContestSettlement, "system", nil, result.NextRoundEndTime); err != nil {
			return fmt.Errorf("%w: %v", ErrScheduleFailed, err)
		}
	}

	return nil
}
