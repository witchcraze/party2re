package ranking

import (
	"context"
	"fmt"
	"strings"

	core_scheduling "github.com/witchcraze/party2re/internal/core/scheduling"
)

const (
	// RankingActionTypeRefresh is the scheduled action type for refreshing ranking snapshots.
	RankingActionTypeRefresh = "party2:ranking:refresh"
)

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
