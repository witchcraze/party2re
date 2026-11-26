package ranking

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	core_scheduling "github.com/witchcraze/party2re/internal/core/scheduling"
	"github.com/witchcraze/party2re/internal/id"
)

const (
	// RankingActionTypeRefresh is the scheduled action type for refreshing ranking snapshots.
	RankingActionTypeRefresh = "party2:ranking:refresh"
	// RankingActionTypeRotateWeekly is the scheduled action type for rotating weekly job change rankings.
	RankingActionTypeRotateWeekly = "party2:ranking:rotate_weekly"

	dailyRefreshActorID = "ranking:daily"
)

// DailyRefreshActionID generates the deterministic JST date prefix for daily refresh wake-up IDs.
func DailyRefreshActionID(t time.Time) string {
	return fmt.Sprintf("ranking:daily_refresh:%s", t.In(jstLocation).Format("2006-01-02"))
}

// WeeklyRotateActionID generates the deterministic JST date prefix for weekly wake-up IDs.
func WeeklyRotateActionID(t time.Time) string {
	return fmt.Sprintf("ranking:weekly_rotate:%s", t.In(jstLocation).Format("2006-01-02"))
}

// RefreshOption configures optional dependencies for RefreshHandler.
type RefreshOption func(*RefreshHandler)

// WithRefreshScheduler configures the scheduler used to maintain recurring daily publication.
func WithRefreshScheduler(scheduler WeeklyRotationScheduler) RefreshOption {
	return func(h *RefreshHandler) {
		h.scheduler = scheduler
	}
}

// RefreshHandler implements scheduling.ActionHandler to execute periodic ranking refreshes.
type RefreshHandler struct {
	reconcileMu      sync.Mutex
	service          *Service
	scheduler        WeeklyRotationScheduler
	verifiedBoundary time.Time
}

// NewRefreshHandler creates a new RefreshHandler.
func NewRefreshHandler(service *Service, opts ...RefreshOption) *RefreshHandler {
	h := &RefreshHandler{
		service: service,
	}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

// Handle processes the ranking refresh scheduled action.
func (h *RefreshHandler) Handle(ctx context.Context, action core_scheduling.ScheduledAction) error {
	rankingTypeStr := strings.TrimSpace(action.Params["ranking_type"])
	if rankingTypeStr == "" || strings.EqualFold(rankingTypeStr, "all") {
		if err := h.service.RefreshAllSnapshots(ctx); err != nil {
			return err
		}
		now := h.service.nowFunc().UTC()
		h.reconcileMu.Lock()
		h.verifiedBoundary = currentDailyBoundaryJST(now)
		err := h.ensureDailyScheduleLocked(ctx, NextMidnightJST(now))
		h.reconcileMu.Unlock()
		return err
	}
	t := RankingType(strings.ToLower(rankingTypeStr))
	if !IsValidRankingType(t) {
		return fmt.Errorf("invalid ranking type %q: %w", rankingTypeStr, ErrInvalidRankingType)
	}
	return h.service.RefreshSnapshot(ctx, t)
}

// Reconcile publishes missing or stale daily boards on startup/recovery and restores the next JST midnight wake-up.
func (h *RefreshHandler) Reconcile(ctx context.Context) error {
	h.reconcileMu.Lock()
	defer h.reconcileMu.Unlock()

	now := h.service.nowFunc().UTC()
	boundary := currentDailyBoundaryJST(now)
	next := NextMidnightJST(now)

	hasPendingNext := false
	if h.scheduler != nil {
		var err error
		hasPendingNext, err = h.hasPendingDailySchedule(ctx, next)
		if err != nil {
			return err
		}
	}

	if h.verifiedBoundary.Before(boundary) || !hasPendingNext {
		snapshots, err := h.service.GetAllSnapshots(ctx)
		if err != nil {
			return fmt.Errorf("read ranking snapshots for daily reconciliation: %w", err)
		}
		if dailySnapshotsNeedRefresh(snapshots, boundary) {
			if err := h.service.RefreshAllSnapshots(ctx); err != nil {
				return err
			}
		}
		h.verifiedBoundary = boundary
	}

	if hasPendingNext {
		return nil
	}
	return h.scheduleDailyRefresh(ctx, next)
}

func (h *RefreshHandler) ensureDailyScheduleLocked(ctx context.Context, next time.Time) error {
	if h.scheduler == nil {
		return nil
	}
	hasPendingNext, err := h.hasPendingDailySchedule(ctx, next)
	if err != nil {
		return err
	}
	if hasPendingNext {
		return nil
	}
	return h.scheduleDailyRefresh(ctx, next)
}

func (h *RefreshHandler) hasPendingDailySchedule(ctx context.Context, next time.Time) (bool, error) {
	actions, err := h.scheduler.FindPendingByActorID(ctx, dailyRefreshActorID)
	if err != nil {
		return false, fmt.Errorf("discover daily ranking refresh reservation: %w", err)
	}
	for _, action := range actions {
		if action.ActionType == RankingActionTypeRefresh && action.ExecuteAt.Equal(next) && action.State == core_scheduling.StatePending {
			return true, nil
		}
	}
	return false, nil
}

func (h *RefreshHandler) scheduleDailyRefresh(ctx context.Context, next time.Time) error {
	if h.scheduler == nil {
		return nil
	}
	actionID := DailyRefreshActionID(next) + ":" + id.New()
	params := map[string]string{"ranking_type": "all"}
	if err := h.scheduler.ScheduleWithID(ctx, actionID, RankingActionTypeRefresh, dailyRefreshActorID, params, next); err != nil {
		return fmt.Errorf("enqueue daily ranking refresh: %w", err)
	}
	return nil
}

func dailySnapshotsNeedRefresh(snapshots map[RankingType]RankingSnapshot, boundary time.Time) bool {
	for _, rt := range dailyRefreshTypes {
		snap, ok := snapshots[rt]
		if !ok || snap.SnapshotData == "" || snap.CalculatedAt.Before(boundary) {
			return true
		}
	}
	return false
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
