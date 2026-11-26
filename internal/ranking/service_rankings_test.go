package ranking_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/ranking"
)

func TestService_WithCacheTTL(t *testing.T) {
	repo := newMockRepo()

	// Default / zero / negative TTL
	svcDef, err := ranking.NewService(repo, ranking.WithCacheTTL(0))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if svcDef == nil {
		t.Fatalf("expected non-nil service")
	}

	svcNeg, err := ranking.NewService(repo, ranking.WithCacheTTL(-5*time.Minute))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if svcNeg == nil {
		t.Fatalf("expected non-nil service")
	}

	// Positive TTL
	svcCustom, err := ranking.NewService(repo, ranking.WithCacheTTL(15*time.Minute))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if svcCustom == nil {
		t.Fatalf("expected non-nil service")
	}
}

func TestService_GetPvPVictoryRanking(t *testing.T) {
	repo := newMockRepo()
	repo.pvpRankings = []ranking.CharacterRankingEntry{
		{Rank: 1, CharacterID: "c1", CharacterName: "Duelist", Score: 50},
	}
	repo.pvpTotal = 1

	svc, err := ranking.NewService(repo)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}
	ctx := context.Background()

	// Snapshot cache read
	if err := svc.RefreshSnapshot(ctx, ranking.RankingTypePvPVictory); err != nil {
		t.Fatalf("failed to refresh snapshot: %v", err)
	}
	cachedPage, err := svc.GetPvPVictoryRanking(ctx, 10, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cachedPage.IsSnapshot || cachedPage.Total != 1 || cachedPage.Entries[0].Score != 50 {
		t.Fatalf("unexpected cached page: %+v", cachedPage)
	}

	// Refresh error path
	repo.err = errors.New("pvp query error")
	if err := svc.RefreshSnapshot(ctx, ranking.RankingTypePvPVictory); err == nil {
		t.Fatalf("expected error on repo failure during refresh")
	}
}

func TestService_GetHelperRanking(t *testing.T) {
	repo := newMockRepo()
	repo.helperRankings = []ranking.CharacterRankingEntry{
		{Rank: 1, CharacterID: "c1", CharacterName: "GoodSamaritan", Score: 45},
	}
	repo.helperTotal = 1

	svc, err := ranking.NewService(repo)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}
	ctx := context.Background()

	// Snapshot cache read
	if err := svc.RefreshSnapshot(ctx, ranking.RankingTypeHelper); err != nil {
		t.Fatalf("failed to refresh snapshot: %v", err)
	}
	cachedPage, err := svc.GetHelperRanking(ctx, 10, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cachedPage.IsSnapshot || cachedPage.Total != 1 || cachedPage.Entries[0].Score != 45 {
		t.Fatalf("unexpected cached page: %+v", cachedPage)
	}

	// Refresh error path
	repo.err = errors.New("helper query error")
	if err := svc.RefreshSnapshot(ctx, ranking.RankingTypeHelper); err == nil {
		t.Fatalf("expected error on repo failure during refresh")
	}
}

func TestService_AdditionalCoverageBranches(t *testing.T) {
	repo := newMockRepo()
	repo.monsterKillsRankings = []ranking.CharacterRankingEntry{
		{Rank: 1, CharacterID: "c1", Score: 10},
	}
	repo.monsterKillsTotal = 1

	svc, err := ranking.NewService(repo)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}
	ctx := context.Background()

	// Monster kills snapshot cache hit
	if err := svc.RefreshSnapshot(ctx, ranking.RankingTypeMonsterKills); err != nil {
		t.Fatalf("refresh monster kills snapshot: %v", err)
	}
	mkCached, err := svc.GetMonsterKillsRanking(ctx, 10, 0)
	if err != nil || !mkCached.IsSnapshot {
		t.Fatalf("expected cached monster kills ranking: %+v, %v", mkCached, err)
	}

	// Test GetRankingByType for all kept ranking types
	rankingTypes := []ranking.RankingType{
		ranking.RankingTypeMonsterKills,
		ranking.RankingTypeMaoCount,
		ranking.RankingTypeHeroCount,
		ranking.RankingTypePvPVictory,
		ranking.RankingTypeJobPopularity,
		ranking.RankingTypeHelper,
		ranking.RankingTypeCasinoWins,
		ranking.RankingTypeAlchemy,
		ranking.RankingTypeWeeklyJobChange,
	}

	for _, rt := range rankingTypes {
		res, err := svc.GetRankingByType(ctx, rt, 10, 0)
		if err != nil {
			t.Fatalf("GetRankingByType(%s) failed: %v", rt, err)
		}
		if res == nil {
			t.Fatalf("GetRankingByType(%s) returned nil", rt)
		}
	}

	// Removed non-legacy types must be rejected
	removedTypes := []ranking.RankingType{
		"level",
		"player_wealth",
		"character_wealth",
		"boss_defeat",
		"adventure_victory",
		"job_mastery",
		"small_medals",
	}
	for _, rt := range removedTypes {
		if _, err := svc.GetRankingByType(ctx, rt, 10, 0); !errors.Is(err, ranking.ErrInvalidRankingType) {
			t.Fatalf("GetRankingByType(%s): expected ErrInvalidRankingType, got %v", rt, err)
		}
		if err := svc.RefreshSnapshot(ctx, rt); !errors.Is(err, ranking.ErrInvalidRankingType) {
			t.Fatalf("RefreshSnapshot(%s): expected ErrInvalidRankingType, got %v", rt, err)
		}
	}

	// Error branches during refresh
	repo.err = errors.New("generic error")
	for _, rt := range []ranking.RankingType{
		ranking.RankingTypeMonsterKills,
		ranking.RankingTypeMaoCount,
		ranking.RankingTypeHeroCount,
		ranking.RankingTypeJobPopularity,
	} {
		if err := svc.RefreshSnapshot(ctx, rt); err == nil {
			t.Fatalf("expected error refreshing %s", rt)
		}
	}
}
