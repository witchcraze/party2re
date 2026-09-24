package ranking_test

import (
	"context"
	"os"
	"testing"

	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/ranking"
)

func TestRankingServiceIntegration(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()

	rankingRepo, err := database.NewRankingRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	svc, err := ranking.NewService(rankingRepo)
	if err != nil {
		t.Fatal(err)
	}

	// 1. Live queries
	lvlPage, err := svc.GetLevelRanking(ctx, 10, 0, false)
	if err != nil {
		t.Fatalf("GetLevelRanking failed: %v", err)
	}
	if lvlPage.RankingType != ranking.RankingTypeLevel {
		t.Errorf("expected level ranking type, got %s", lvlPage.RankingType)
	}

	wealthPage, err := svc.GetPlayerWealthRanking(ctx, 10, 0, false)
	if err != nil {
		t.Fatalf("GetPlayerWealthRanking failed: %v", err)
	}
	if wealthPage.RankingType != ranking.RankingTypePlayerWealth {
		t.Errorf("expected player_wealth ranking type, got %s", wealthPage.RankingType)
	}

	mkPage, err := svc.GetMonsterKillsRanking(ctx, 10, 0, false)
	if err != nil {
		t.Fatalf("GetMonsterKillsRanking failed: %v", err)
	}
	if mkPage.RankingType != ranking.RankingTypeMonsterKills {
		t.Errorf("expected monster_kills ranking type, got %s", mkPage.RankingType)
	}

	maoPage, err := svc.GetMaoCountRanking(ctx, 10, 0, false)
	if err != nil {
		t.Fatalf("GetMaoCountRanking failed: %v", err)
	}
	if maoPage.RankingType != ranking.RankingTypeMaoCount {
		t.Errorf("expected mao_count ranking type, got %s", maoPage.RankingType)
	}

	heroPage, err := svc.GetHeroCountRanking(ctx, 10, 0, false)
	if err != nil {
		t.Fatalf("GetHeroCountRanking failed: %v", err)
	}
	if heroPage.RankingType != ranking.RankingTypeHeroCount {
		t.Errorf("expected hero_count ranking type, got %s", heroPage.RankingType)
	}

	jobPopPage, err := svc.GetJobPopularityRanking(ctx, false)
	if err != nil {
		t.Fatalf("GetJobPopularityRanking failed: %v", err)
	}
	if jobPopPage.RankingType != ranking.RankingTypeJobPopularity {
		t.Errorf("expected job_popularity ranking type, got %s", jobPopPage.RankingType)
	}

	// 2. Snapshot refresh and cached queries
	if err := svc.RefreshAllSnapshots(ctx); err != nil {
		t.Fatalf("RefreshAllSnapshots failed: %v", err)
	}

	cachedLvlPage, err := svc.GetLevelRanking(ctx, 10, 0, true)
	if err != nil {
		t.Fatalf("GetLevelRanking (cached) failed: %v", err)
	}
	if !cachedLvlPage.IsSnapshot {
		t.Errorf("expected IsSnapshot=true for cached page")
	}

	// 3. Snapshot retrieval
	snap, err := svc.GetSnapshot(ctx, ranking.RankingTypeLevel)
	if err != nil {
		t.Fatalf("GetSnapshot failed: %v", err)
	}
	if snap.RankingType != ranking.RankingTypeLevel {
		t.Errorf("expected ranking type level, got %s", snap.RankingType)
	}

	allSnaps, err := svc.GetAllSnapshots(ctx)
	if err != nil {
		t.Fatalf("GetAllSnapshots failed: %v", err)
	}
	if len(allSnaps) < 13 {
		t.Errorf("expected at least 13 snapshots, got %d", len(allSnaps))
	}

	// 3b. Casino wins and Alchemy rankings
	casPage, err := svc.GetCasinoWinsRanking(ctx, 10, 0, false)
	if err != nil {
		t.Fatalf("GetCasinoWinsRanking failed: %v", err)
	}
	if casPage.RankingType != ranking.RankingTypeCasinoWins {
		t.Errorf("expected casino_wins ranking type, got %s", casPage.RankingType)
	}

	alcPage, err := svc.GetAlchemyRanking(ctx, 10, 0, false)
	if err != nil {
		t.Fatalf("GetAlchemyRanking failed: %v", err)
	}
	if alcPage.RankingType != ranking.RankingTypeAlchemy {
		t.Errorf("expected alchemy ranking type, got %s", alcPage.RankingType)
	}

	// 3c. Weekly job changes and rotation
	if err := svc.RotateWeeklyJobChangeRanking(ctx); err != nil {
		t.Fatalf("RotateWeeklyJobChangeRanking failed: %v", err)
	}
	weeklyPage, err := svc.GetWeeklyJobChangeRanking(ctx, 10, 0, true)
	if err != nil {
		t.Fatalf("GetWeeklyJobChangeRanking failed: %v", err)
	}
	if weeklyPage.RankingType != ranking.RankingTypeWeeklyJobChange {
		t.Errorf("expected weekly_job_change ranking type, got %s", weeklyPage.RankingType)
	}

	// 3d. Hall of Fame
	legends, err := svc.GetLegends(ctx)
	if err != nil {
		t.Fatalf("GetLegends failed: %v", err)
	}
	if len(legends) != 6 {
		t.Fatalf("expected 6 legend categories, got %d", len(legends))
	}
	legPage, err := svc.GetLegendCategory(ctx, ranking.LegendCategoryJobMastery)
	if err != nil {
		t.Fatalf("GetLegendCategory failed: %v", err)
	}
	if legPage.Category != ranking.LegendCategoryJobMastery {
		t.Errorf("expected comp_job category, got %s", legPage.Category)
	}

	// 4. Cold-start Service instance (clean in-memory cache) falls back to database snapshot
	freshSvc, err := ranking.NewService(rankingRepo)
	if err != nil {
		t.Fatal(err)
	}

	coldLvlPage, err := freshSvc.GetLevelRanking(ctx, 10, 0, true)
	if err != nil {
		t.Fatalf("GetLevelRanking (cold snapshot fallback) failed: %v", err)
	}
	if !coldLvlPage.IsSnapshot {
		t.Errorf("expected IsSnapshot=true for cold start snapshot query")
	}

	// 5. Warmup cache on fresh instance
	warmupSvc, err := ranking.NewService(rankingRepo)
	if err != nil {
		t.Fatal(err)
	}
	if err := warmupSvc.WarmupCache(ctx); err != nil {
		t.Fatalf("WarmupCache failed: %v", err)
	}
	warmLvlPage, err := warmupSvc.GetLevelRanking(ctx, 10, 0, true)
	if err != nil || !warmLvlPage.IsSnapshot {
		t.Fatalf("expected cached ranking after warmup: %v, %+v", err, warmLvlPage)
	}
}
