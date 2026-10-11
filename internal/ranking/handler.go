package ranking

import (
	"context"
	"fmt"
	"strings"
	"time"

	core_scheduling "github.com/witchcraze/party2re/internal/core/scheduling"
	"github.com/witchcraze/party2re/internal/id"
)

const (
	// RankingActionTypeRefresh is the scheduled action type for refreshing ranking snapshots.
	RankingActionTypeRefresh = "party2:ranking:refresh"
	// RankingActionTypeRotateWeekly is the scheduled action type for rotating weekly job change rankings.
	RankingActionTypeRotateWeekly = "party2:ranking:rotate_weekly"
)

// WeeklyRotateActionID generates the deterministic JST date prefix for weekly wake-up IDs.
func WeeklyRotateActionID(t time.Time) string {
	return fmt.Sprintf("ranking:weekly_rotate:%s", t.In(jstLocation).Format("2006-01-02"))
}

// RefreshHandler implements scheduling.ActionHandler to execute periodic ranking refreshes.
type RefreshHandler struct {
	service *Service
}

// NewRefreshHandler creates a new RefreshHandler.
func NewRefreshHandler(service *Service) *RefreshHandler {
	return &RefreshHandler{
		service: service,
	}
}

// Handle processes the ranking refresh scheduled action.
func (h *RefreshHandler) Handle(ctx context.Context, action core_scheduling.ScheduledAction) error {
	rankingTypeStr := strings.TrimSpace(action.Params["ranking_type"])
	if rankingTypeStr == "" || strings.EqualFold(rankingTypeStr, "all") {
		return h.service.RefreshAllSnapshots(ctx)
	}
	t := RankingType(strings.ToLower(rankingTypeStr))
	if !IsValidRankingType(t) {
		return fmt.Errorf("invalid ranking type %q: %w", rankingTypeStr, ErrInvalidRankingType)
	}
	return h.service.RefreshSnapshot(ctx, t)
}

// RotateWeeklyHandler implements scheduling.ActionHandler to execute weekly job change ranking rotation.
type RotateWeeklyHandler struct {
	service   *Service
	scheduler WeeklyRotationScheduler
}

// WeeklyRotationScheduler manages the ranking wake-up reservation; SQL owns settlement progress.
type WeeklyRotationScheduler interface {
	ScheduleWithID(context.Context, string, string, string, map[string]string, time.Time) error
	FindPendingByActorID(context.Context, string) ([]core_scheduling.ScheduledAction, error)
}

// NewRotateWeeklyHandler creates a new RotateWeeklyHandler.
func NewRotateWeeklyHandler(service *Service, scheduler WeeklyRotationScheduler) *RotateWeeklyHandler {
	return &RotateWeeklyHandler{
		service:   service,
		scheduler: scheduler,
	}
}

// Handle processes the weekly job change ranking rotation action.
func (h *RotateWeeklyHandler) Handle(ctx context.Context, action core_scheduling.ScheduledAction) error {
	// An action is only a wake-up: stale or uncertain deliveries defer to durable SQL progress.
	return h.Reconcile(ctx)
}

// Reconcile settles overdue counters once, then restores the next reservation, also on startup.
// Enqueue errors occur after SQL commit; retrying reconciliation never repeats that settlement.
func (h *RotateWeeklyHandler) Reconcile(ctx context.Context) error {
	next, err := h.service.RotateWeeklyJobChangeRanking(ctx)
	if err != nil {
		return err
	}
	const actorID = "ranking:weekly"
	actions, err := h.scheduler.FindPendingByActorID(ctx, actorID)
	if err != nil {
		return fmt.Errorf("discover weekly rotation reservation after SQL reconciliation: %w", err)
	}
	for _, action := range actions {
		if action.ActionType == RankingActionTypeRotateWeekly && action.ExecuteAt.Equal(next) && action.State == core_scheduling.StatePending {
			return nil
		}
	}
	// Discovery and enqueue are not atomic: a fresh ID cannot overwrite another worker's Processing payload.
	actionID := WeeklyRotateActionID(next) + ":" + id.New()
	if err := h.scheduler.ScheduleWithID(ctx, actionID, RankingActionTypeRotateWeekly, actorID, nil, next); err != nil {
		return fmt.Errorf("enqueue weekly rotation after SQL reconciliation: %w", err)
	}
	return nil
}
