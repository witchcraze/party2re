package main

import (
	"context"
	"net/http"
	"testing"

	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/ranking"
	"github.com/witchcraze/party2re/internal/scheduling"
)

func TestRankingProductionHTTPWiring(t *testing.T) {
	app := newScenarioApp(t)
	app.call("", http.MethodGet, "/legends/comp_mon", "", http.StatusOK, nil)
}

func TestRankingDailyPublicationAndSnapshotOnlyHTTPWiring(t *testing.T) {
	app := newIsolatedScenarioApp(t)
	ctx := context.Background()

	// 1. Startup reconciliation publishes all 8 daily snapshots and schedules ranking:daily.
	repo, err := database.NewRankingRepository(app.db)
	if err != nil {
		t.Fatal(err)
	}
	snaps, err := repo.GetAllSnapshots(ctx)
	if err != nil || len(snaps) < 8 {
		t.Fatalf("startup snapshots = %d, err = %v", len(snaps), err)
	}
	schedRepo := scheduling.NewValkeyRepository(app.valkey)
	pending, err := schedRepo.FindPendingByActorID(ctx, "ranking:daily")
	if err != nil || len(pending) != 1 || pending[0].ActionType != ranking.RankingActionTypeRefresh {
		t.Fatalf("pending daily refresh actions = %+v, err = %v", pending, err)
	}
	// Executing the daily scheduled action after Reconcile must not republish today's snapshots.
	calcBefore := snaps[ranking.RankingTypeMonsterKills].CalculatedAt
	app.worker.ProcessAction(ctx, pending[0])
	snapAfter, err := repo.GetSnapshot(ctx, ranking.RankingTypeMonsterKills)
	if err != nil || !snapAfter.CalculatedAt.Equal(calcBefore) {
		t.Fatalf("scheduled daily action re-refreshed after Reconcile: before=%v after=%v err=%v", calcBefore, snapAfter.CalculatedAt, err)
	}

	// 2. Live counter mutations do not leak through ?snapshot=false on public boards.
	char, err := database.CreateTestCharacter(ctx, app.db, "LiveHide")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.db.ExecContext(ctx, `UPDATE characters SET monster_kills = 999 WHERE id = ?`, char.ID); err != nil {
		t.Fatal(err)
	}
	var mkPage ranking.RankingPage[ranking.CharacterRankingEntry]
	app.call("", http.MethodGet, "/rankings/monster-kills?snapshot=false", "", http.StatusOK, &mkPage)
	if !mkPage.IsSnapshot {
		t.Fatalf("expected published snapshot even with snapshot=false, got %+v", mkPage)
	}
	for _, entry := range mkPage.Entries {
		if entry.CharacterID == char.ID {
			t.Fatalf("unpublished monster_kills leaked into public read: %+v", mkPage)
		}
	}

	// 3. Active weekly job changes never leak before first Sunday rotation.
	if err := repo.IncrementJobChangeCount(ctx, char.ID); err != nil {
		t.Fatal(err)
	}
	var weeklyPage ranking.RankingPage[ranking.CharacterRankingEntry]
	app.call("", http.MethodGet, "/rankings/weekly-job-change?snapshot=false", "", http.StatusOK, &weeklyPage)
	if weeklyPage.Total != 0 || len(weeklyPage.Entries) != 0 {
		t.Fatalf("active weekly counters leaked before first rotation: %+v", weeklyPage)
	}
}
