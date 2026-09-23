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
	return h
}

// Handle executes the contest settlement logic and schedules the next settlement check.
func (h *ContestSettlementHandler) Handle(ctx context.Context, action core_scheduling.ScheduledAction) error {
	result, err := h.service.SettleContest(ctx, false)
	if err != nil {
		return err
	}

	if h.scheduler != nil && !result.NextRoundEndTime.IsZero() {
		nextID := SettlementActionID(result.NextRound, result.NextRoundEndTime)
		_ = h.scheduler.ScheduleWithID(ctx, nextID, ActionTypeContestSettlement, "system", nil, result.NextRoundEndTime)
	}

	return nil
}
