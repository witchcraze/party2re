package ranking_test

import (
	"context"
	"testing"

	core_scheduling "github.com/witchcraze/party2re/internal/core/scheduling"
	"github.com/witchcraze/party2re/internal/ranking"
)

func TestRefreshHandler_HandleAll(t *testing.T) {
	ctx := context.Background()
	repo := newMockRepo()
	svc, err := ranking.NewService(repo)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	handler := ranking.NewRefreshHandler(svc)

	// Refresh ALL
	action := core_scheduling.ScheduledAction{
		ID:         "act-1",
		ActionType: ranking.RankingActionTypeRefresh,
		Params:     map[string]string{"ranking_type": "ALL"},
	}

	if err := handler.Handle(ctx, action); err != nil {
		t.Fatalf("Handle(ALL): %v", err)
	}

	// Verify all snapshots were saved
	snapshots, err := svc.GetAllSnapshots(ctx)
	if err != nil {
		t.Fatalf("GetAllSnapshots: %v", err)
	}
	if len(snapshots) != 8 {
		t.Errorf("expected 8 snapshots, got %d", len(snapshots))
	}
}

func TestRefreshHandler_HandleSingleType(t *testing.T) {
	ctx := context.Background()
	repo := newMockRepo()
	svc, err := ranking.NewService(repo)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	handler := ranking.NewRefreshHandler(svc)

	// Refresh MONSTER_KILLS only
	action := core_scheduling.ScheduledAction{
		ID:         "act-2",
		ActionType: ranking.RankingActionTypeRefresh,
		Params:     map[string]string{"ranking_type": "MONSTER_KILLS"},
	}

	if err := handler.Handle(ctx, action); err != nil {
		t.Fatalf("Handle(MONSTER_KILLS): %v", err)
	}

	snap, err := svc.GetSnapshot(ctx, ranking.RankingTypeMonsterKills)
	if err != nil {
		t.Fatalf("GetSnapshot(MONSTER_KILLS): %v", err)
	}
	if snap.RankingType != ranking.RankingTypeMonsterKills {
		t.Errorf("expected snapshot monster_kills, got %s", snap.RankingType)
	}
}

func TestRefreshHandler_HandleInvalidType(t *testing.T) {
	ctx := context.Background()
	repo := newMockRepo()
	svc, _ := ranking.NewService(repo)
	handler := ranking.NewRefreshHandler(svc)

	action := core_scheduling.ScheduledAction{
		ID:         "act-3",
		ActionType: ranking.RankingActionTypeRefresh,
		Params:     map[string]string{"ranking_type": "INVALID_TYPE"},
	}

	if err := handler.Handle(ctx, action); err == nil {
		t.Errorf("expected error for invalid ranking type, got nil")
	}
}
