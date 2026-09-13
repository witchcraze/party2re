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

func TestService_GetCharacterWealthRanking(t *testing.T) {
	repo := newMockRepo()
	repo.characterWealthRankings = []ranking.CharacterRankingEntry{
		{Rank: 1, CharacterID: "c1", CharacterName: "Merchant", Score: 999999},
		{Rank: 2, CharacterID: "c2", CharacterName: "Peasant", Score: 100},
	}
	repo.characterWealthTotal = 2

	fixedTime := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)
	svc, err := ranking.NewService(repo, ranking.WithNowFunc(func() time.Time { return fixedTime }))
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}
	ctx := context.Background()

	// Live query
	page, err := svc.GetCharacterWealthRanking(ctx, 10, 0, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if page.Total != 2 || len(page.Entries) != 2 {
		t.Fatalf("expected 2 entries, got total=%d len=%d", page.Total, len(page.Entries))
	}
	if page.IsSnapshot {
		t.Fatalf("expected IsSnapshot=false")
	}
	if page.Entries[0].CharacterName != "Merchant" {
		t.Fatalf("expected top entry 'Merchant', got %s", page.Entries[0].CharacterName)
	}

	// Snapshot cache read
	if err := svc.RefreshSnapshot(ctx, ranking.RankingTypeCharacterWealth); err != nil {
		t.Fatalf("failed to refresh snapshot: %v", err)
	}
	cachedPage, err := svc.GetCharacterWealthRanking(ctx, 10, 0, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cachedPage.IsSnapshot {
		t.Fatalf("expected IsSnapshot=true")
	}
	if cachedPage.Total != 2 {
		t.Fatalf("expected cached total 2, got %d", cachedPage.Total)
	}

	// Error path
	repo.err = errors.New("db failure")
	_, err = svc.GetCharacterWealthRanking(ctx, 10, 0, false)
	if err == nil {
		t.Fatalf("expected error on repo failure")
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

	// Live query
	page, err := svc.GetPvPVictoryRanking(ctx, 10, 0, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if page.Total != 1 || len(page.Entries) != 1 || page.Entries[0].Score != 50 {
		t.Fatalf("unexpected live page: %+v", page)
	}

	// Snapshot cache read
	if err := svc.RefreshSnapshot(ctx, ranking.RankingTypePvPVictory); err != nil {
		t.Fatalf("failed to refresh snapshot: %v", err)
	}
	cachedPage, err := svc.GetPvPVictoryRanking(ctx, 10, 0, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cachedPage.IsSnapshot || cachedPage.Total != 1 {
		t.Fatalf("unexpected cached page: %+v", cachedPage)
	}

	// Error path
	repo.err = errors.New("pvp query error")
	_, err = svc.GetPvPVictoryRanking(ctx, 10, 0, false)
	if err == nil {
		t.Fatalf("expected error on repo failure")
	}
}

func TestService_GetBossDefeatRanking(t *testing.T) {
	repo := newMockRepo()
	repo.bossRankings = []ranking.CharacterRankingEntry{
		{Rank: 1, CharacterID: "c1", CharacterName: "BossSlayer", Score: 12},
	}
	repo.bossTotal = 1

	svc, err := ranking.NewService(repo)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}
	ctx := context.Background()

	// Live query
	page, err := svc.GetBossDefeatRanking(ctx, 10, 0, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if page.Total != 1 || len(page.Entries) != 1 || page.Entries[0].Score != 12 {
		t.Fatalf("unexpected live page: %+v", page)
	}

	// Snapshot cache read
	if err := svc.RefreshSnapshot(ctx, ranking.RankingTypeBossDefeat); err != nil {
		t.Fatalf("failed to refresh snapshot: %v", err)
	}
	cachedPage, err := svc.GetBossDefeatRanking(ctx, 10, 0, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cachedPage.IsSnapshot || cachedPage.Total != 1 {
		t.Fatalf("unexpected cached page: %+v", cachedPage)
	}

	// Error path
	repo.err = errors.New("boss query error")
	_, err = svc.GetBossDefeatRanking(ctx, 10, 0, false)
	if err == nil {
		t.Fatalf("expected error on repo failure")
	}
}

func TestService_GetAdventureVictoryRanking(t *testing.T) {
	repo := newMockRepo()
	repo.advRankings = []ranking.CharacterRankingEntry{
		{Rank: 1, CharacterID: "c1", CharacterName: "Adventurer", Score: 200},
	}
	repo.advTotal = 1

	svc, err := ranking.NewService(repo)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}
	ctx := context.Background()

	// Live query
	page, err := svc.GetAdventureVictoryRanking(ctx, 10, 0, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if page.Total != 1 || len(page.Entries) != 1 || page.Entries[0].Score != 200 {
		t.Fatalf("unexpected live page: %+v", page)
	}

	// Snapshot cache read
	if err := svc.RefreshSnapshot(ctx, ranking.RankingTypeAdventureVictory); err != nil {
		t.Fatalf("failed to refresh snapshot: %v", err)
	}
	cachedPage, err := svc.GetAdventureVictoryRanking(ctx, 10, 0, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cachedPage.IsSnapshot || cachedPage.Total != 1 {
		t.Fatalf("unexpected cached page: %+v", cachedPage)
	}

	// Error path
	repo.err = errors.New("adv query error")
	_, err = svc.GetAdventureVictoryRanking(ctx, 10, 0, false)
	if err == nil {
		t.Fatalf("expected error on repo failure")
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

	// Live query
	page, err := svc.GetHelperRanking(ctx, 10, 0, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if page.Total != 1 || len(page.Entries) != 1 || page.Entries[0].Score != 45 {
		t.Fatalf("unexpected live page: %+v", page)
	}

	// Snapshot cache read
	if err := svc.RefreshSnapshot(ctx, ranking.RankingTypeHelper); err != nil {
		t.Fatalf("failed to refresh snapshot: %v", err)
	}
	cachedPage, err := svc.GetHelperRanking(ctx, 10, 0, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cachedPage.IsSnapshot || cachedPage.Total != 1 {
		t.Fatalf("unexpected cached page: %+v", cachedPage)
	}

	// Error path
	repo.err = errors.New("helper query error")
	_, err = svc.GetHelperRanking(ctx, 10, 0, false)
	if err == nil {
		t.Fatalf("expected error on repo failure")
	}
}

func TestService_GetSmallMedalRanking(t *testing.T) {
	repo := newMockRepo()
	repo.smallMedalRankings = []ranking.CharacterRankingEntry{
		{Rank: 1, CharacterID: "c1", CharacterName: "MedalCollector", Score: 80},
	}
	repo.smallMedalTotal = 1

	svc, err := ranking.NewService(repo)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}
	ctx := context.Background()

	// Live query
	page, err := svc.GetSmallMedalRanking(ctx, 10, 0, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if page.Total != 1 || len(page.Entries) != 1 || page.Entries[0].Score != 80 {
		t.Fatalf("unexpected live page: %+v", page)
	}

	// Snapshot cache read
	if err := svc.RefreshSnapshot(ctx, ranking.RankingTypeSmallMedals); err != nil {
		t.Fatalf("failed to refresh snapshot: %v", err)
	}
	cachedPage, err := svc.GetSmallMedalRanking(ctx, 10, 0, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cachedPage.IsSnapshot || cachedPage.Total != 1 {
		t.Fatalf("unexpected cached page: %+v", cachedPage)
	}

	// Error path
	repo.err = errors.New("medal query error")
	_, err = svc.GetSmallMedalRanking(ctx, 10, 0, false)
	if err == nil {
		t.Fatalf("expected error on repo failure")
	}
}

func TestService_AdditionalCoverageBranches(t *testing.T) {
	repo := newMockRepo()
	repo.battleRankings = []ranking.CharacterRankingEntry{
		{Rank: 1, CharacterID: "c1", Score: 10},
	}
	repo.battleTotal = 1
	repo.jobMasteryRankings = []ranking.CharacterRankingEntry{
		{Rank: 1, CharacterID: "c1", Score: 5},
	}
	repo.jobMasteryTotal = 1
	repo.levelRankings = []ranking.CharacterRankingEntry{
		{Rank: 1, CharacterID: "c1", Score: 20},
	}
	repo.levelTotal = 1
	repo.playerWealthRankings = []ranking.PlayerWealthRankingEntry{
		{Rank: 1, PlayerID: "p1", TotalWealth: 1000},
	}
	repo.playerWealthTotal = 1

	svc, err := ranking.NewService(repo)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}
	ctx := context.Background()

	// Battle victory snapshot cache hit
	if err := svc.RefreshSnapshot(ctx, ranking.RankingTypeBattleVictory); err != nil {
		t.Fatalf("refresh battle snapshot: %v", err)
	}
	battleCached, err := svc.GetBattleVictoryRanking(ctx, 10, 0, true)
	if err != nil || !battleCached.IsSnapshot {
		t.Fatalf("expected cached battle ranking: %+v, %v", battleCached, err)
	}

	// Job mastery snapshot cache hit
	if err := svc.RefreshSnapshot(ctx, ranking.RankingTypeJobMastery); err != nil {
		t.Fatalf("refresh job mastery snapshot: %v", err)
	}
	masteryCached, err := svc.GetJobMasteryRanking(ctx, 10, 0, true)
	if err != nil || !masteryCached.IsSnapshot {
		t.Fatalf("expected cached mastery ranking: %+v, %v", masteryCached, err)
	}

	// Test GetRankingByType for all ranking types
	rankingTypes := []ranking.RankingType{
		ranking.RankingTypeLevel,
		ranking.RankingTypePlayerWealth,
		ranking.RankingTypeCharacterWealth,
		ranking.RankingTypeBattleVictory,
		ranking.RankingTypePvPVictory,
		ranking.RankingTypeBossDefeat,
		ranking.RankingTypeAdventureVictory,
		ranking.RankingTypeJobMastery,
		ranking.RankingTypeJobPopularity,
		ranking.RankingTypeHelper,
		ranking.RankingTypeSmallMedals,
	}

	for _, rt := range rankingTypes {
		res, err := svc.GetRankingByType(ctx, rt, 10, 0, false)
		if err != nil {
			t.Fatalf("GetRankingByType(%s) failed: %v", rt, err)
		}
		if res == nil {
			t.Fatalf("GetRankingByType(%s) returned nil", rt)
		}
	}

	// Error branches
	repo.err = errors.New("generic error")
	if _, err := svc.GetBattleVictoryRanking(ctx, 10, 0, false); err == nil {
		t.Fatalf("expected error for battle victory")
	}
	if _, err := svc.GetJobMasteryRanking(ctx, 10, 0, false); err == nil {
		t.Fatalf("expected error for job mastery")
	}
	if _, err := svc.GetJobPopularityRanking(ctx, false); err == nil {
		t.Fatalf("expected error for job popularity")
	}
	if _, err := svc.GetLevelRanking(ctx, 10, 0, false); err == nil {
		t.Fatalf("expected error for level ranking")
	}
	if _, err := svc.GetPlayerWealthRanking(ctx, 10, 0, false); err == nil {
		t.Fatalf("expected error for player wealth ranking")
	}
}
