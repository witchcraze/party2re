package ranking

import (
	"context"
	"fmt"
	"strings"
	"time"

	core_scheduling "github.com/witchcraze/party2re/internal/core/scheduling"
)

const (
	// RankingActionTypeRefresh is the scheduled action type for refreshing ranking snapshots.
	RankingActionTypeRefresh = "party2:ranking:refresh"
	// RankingActionTypeRotateWeekly is the scheduled action type for rotating weekly job change rankings.
	RankingActionTypeRotateWeekly = "party2:ranking:rotate_weekly"
)

// WeeklyRotateActionID generates a deterministic action ID for Sunday midnight weekly rotation.
func WeeklyRotateActionID(t time.Time) string {
	return fmt.Sprintf("ranking:weekly_rotate:%s", t.Format("2006-01-02"))
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
	service *Service
}

// NewRotateWeeklyHandler creates a new RotateWeeklyHandler.
func NewRotateWeeklyHandler(service *Service) *RotateWeeklyHandler {
	return &RotateWeeklyHandler{
		service: service,
	}
}

// Handle processes the weekly job change ranking rotation action.
func (h *RotateWeeklyHandler) Handle(ctx context.Context, action core_scheduling.ScheduledAction) error {
	return h.service.RotateWeeklyJobChangeRanking(ctx)
}
