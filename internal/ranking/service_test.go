package ranking_test

import (
	"context"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/ranking"
)

type mockRankingRepository struct {
	levelRankings           []ranking.CharacterRankingEntry
	levelTotal              int
	playerWealthRankings    []ranking.PlayerWealthRankingEntry
	playerWealthTotal       int
	characterWealthRankings []ranking.CharacterRankingEntry
	characterWealthTotal    int
	battleRankings          []ranking.CharacterRankingEntry
	battleTotal             int
	pvpRankings             []ranking.CharacterRankingEntry
	pvpTotal                int
	bossRankings            []ranking.CharacterRankingEntry
	bossTotal               int
	advRankings             []ranking.CharacterRankingEntry
	advTotal                int
	jobMasteryRankings      []ranking.CharacterRankingEntry
	jobMasteryTotal         int
	jobPopularityRankings   []ranking.JobPopularityEntry
	helperRankings          []ranking.CharacterRankingEntry
	helperTotal             int
	rebirthRankings         []ranking.CharacterRankingEntry
	rebirthTotal            int
	smallMedalRankings      []ranking.CharacterRankingEntry
	smallMedalTotal         int
	snapshots               map[ranking.RankingType]ranking.RankingSnapshot
}

func newMockRepo() *mockRankingRepository {
	return &mockRankingRepository{
		snapshots: make(map[ranking.RankingType]ranking.RankingSnapshot),
	}
}

func (m *mockRankingRepository) GetLevelRanking(ctx context.Context, limit, offset int) ([]ranking.CharacterRankingEntry, int, error) {
	return paginateSlice(m.levelRankings, limit, offset), m.levelTotal, nil
}

func (m *mockRankingRepository) GetPlayerWealthRanking(ctx context.Context, limit, offset int) ([]ranking.PlayerWealthRankingEntry, int, error) {
	return paginateSlice(m.playerWealthRankings, limit, offset), m.playerWealthTotal, nil
}

func (m *mockRankingRepository) GetCharacterWealthRanking(ctx context.Context, limit, offset int) ([]ranking.CharacterRankingEntry, int, error) {
	return paginateSlice(m.characterWealthRankings, limit, offset), m.characterWealthTotal, nil
}

func (m *mockRankingRepository) GetBattleVictoryRanking(ctx context.Context, limit, offset int) ([]ranking.CharacterRankingEntry, int, error) {
	return paginateSlice(m.battleRankings, limit, offset), m.battleTotal, nil
}

func (m *mockRankingRepository) GetPvPVictoryRanking(ctx context.Context, limit, offset int) ([]ranking.CharacterRankingEntry, int, error) {
	return paginateSlice(m.pvpRankings, limit, offset), m.pvpTotal, nil
}

func (m *mockRankingRepository) GetBossDefeatRanking(ctx context.Context, limit, offset int) ([]ranking.CharacterRankingEntry, int, error) {
	return paginateSlice(m.bossRankings, limit, offset), m.bossTotal, nil
}

func (m *mockRankingRepository) GetAdventureVictoryRanking(ctx context.Context, limit, offset int) ([]ranking.CharacterRankingEntry, int, error) {
	return paginateSlice(m.advRankings, limit, offset), m.advTotal, nil
}

func (m *mockRankingRepository) GetJobMasteryRanking(ctx context.Context, limit, offset int) ([]ranking.CharacterRankingEntry, int, error) {
	return paginateSlice(m.jobMasteryRankings, limit, offset), m.jobMasteryTotal, nil
}

func (m *mockRankingRepository) GetJobPopularityRanking(ctx context.Context) ([]ranking.JobPopularityEntry, error) {
	return m.jobPopularityRankings, nil
}

func (m *mockRankingRepository) GetHelperRanking(ctx context.Context, limit, offset int) ([]ranking.CharacterRankingEntry, int, error) {
	return paginateSlice(m.helperRankings, limit, offset), m.helperTotal, nil
}

func (m *mockRankingRepository) GetRebirthRanking(ctx context.Context, limit, offset int) ([]ranking.CharacterRankingEntry, int, error) {
	return paginateSlice(m.rebirthRankings, limit, offset), m.rebirthTotal, nil
}

func (m *mockRankingRepository) GetSmallMedalRanking(ctx context.Context, limit, offset int) ([]ranking.CharacterRankingEntry, int, error) {
	return paginateSlice(m.smallMedalRankings, limit, offset), m.smallMedalTotal, nil
}

func (m *mockRankingRepository) SaveSnapshot(ctx context.Context, snapshot ranking.RankingSnapshot) error {
	m.snapshots[snapshot.RankingType] = snapshot
	return nil
}

func (m *mockRankingRepository) GetSnapshot(ctx context.Context, rankingType ranking.RankingType) (ranking.RankingSnapshot, error) {
	s, ok := m.snapshots[rankingType]
	if !ok {
		return ranking.RankingSnapshot{}, ranking.ErrSnapshotNotFound
	}
	return s, nil
}

func (m *mockRankingRepository) GetAllSnapshots(ctx context.Context) (map[ranking.RankingType]ranking.RankingSnapshot, error) {
	return m.snapshots, nil
}

func paginateSlice[T any](items []T, limit, offset int) []T {
	if offset >= len(items) {
		return []T{}
	}
	end := offset + limit
	if end > len(items) {
		end = len(items)
	}
	return items[offset:end]
}

func TestService_GetLevelRanking(t *testing.T) {
	repo := newMockRepo()
	repo.levelRankings = []ranking.CharacterRankingEntry{
		{Rank: 1, CharacterID: "char1", CharacterName: "Hero", Level: 50, Experience: 25000, Score: 50, SecondaryScore: 25000},
		{Rank: 2, CharacterID: "char2", CharacterName: "Mage", Level: 45, Experience: 20250, Score: 45, SecondaryScore: 20250},
		{Rank: 3, CharacterID: "char3", CharacterName: "Warrior", Level: 45, Experience: 20000, Score: 45, SecondaryScore: 20000},
	}
	repo.levelTotal = 3

	fixedTime := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	svc, err := ranking.NewService(repo, ranking.WithNowFunc(func() time.Time { return fixedTime }))
	if err != nil {
		t.Fatalf("unexpected error creating service: %v", err)
	}

	ctx := context.Background()

	// Live query
	page, err := svc.GetLevelRanking(ctx, 2, 0, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(page.Entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(page.Entries))
	}
	if page.Total != 3 {
		t.Fatalf("expected total 3, got %d", page.Total)
	}
	if page.IsSnapshot {
		t.Fatalf("expected IsSnapshot=false")
	}
	if page.Entries[0].CharacterName != "Hero" || page.Entries[1].CharacterName != "Mage" {
		t.Fatalf("unexpected order: %+v", page.Entries)
	}

	// Refresh snapshot and read cached
	if err := svc.RefreshSnapshot(ctx, ranking.RankingTypeLevel); err != nil {
		t.Fatalf("failed to refresh snapshot: %v", err)
	}

	cachedPage, err := svc.GetLevelRanking(ctx, 2, 0, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cachedPage.IsSnapshot {
		t.Fatalf("expected IsSnapshot=true")
	}
	if len(cachedPage.Entries) != 2 {
		t.Fatalf("expected 2 cached entries, got %d", len(cachedPage.Entries))
	}
}

func TestService_GetPlayerWealthRanking(t *testing.T) {
	repo := newMockRepo()
	repo.playerWealthRankings = []ranking.PlayerWealthRankingEntry{
		{Rank: 1, PlayerID: "p1", Username: "rich_player", TotalWealth: 1000000, BankBalance: 800000, CharactersMoney: 200000, CharacterCount: 2},
		{Rank: 2, PlayerID: "p2", Username: "normal_player", TotalWealth: 50000, BankBalance: 40000, CharactersMoney: 10000, CharacterCount: 1},
	}
	repo.playerWealthTotal = 2

	svc, _ := ranking.NewService(repo)
	ctx := context.Background()

	page, err := svc.GetPlayerWealthRanking(ctx, 10, 0, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if page.Total != 2 || len(page.Entries) != 2 {
		t.Fatalf("expected 2 entries, got total=%d len=%d", page.Total, len(page.Entries))
	}
	if page.Entries[0].TotalWealth != 1000000 {
		t.Fatalf("expected top wealth 1000000, got %d", page.Entries[0].TotalWealth)
	}
}

func TestService_GetBattleVictoryRanking(t *testing.T) {
	repo := newMockRepo()
	repo.battleRankings = []ranking.CharacterRankingEntry{
		{Rank: 1, CharacterID: "c1", CharacterName: "Champion", Score: 150, Level: 30},
		{Rank: 2, CharacterID: "c2", CharacterName: "Challenger", Score: 95, Level: 28},
	}
	repo.battleTotal = 2

	svc, _ := ranking.NewService(repo)
	ctx := context.Background()

	page, err := svc.GetBattleVictoryRanking(ctx, 10, 0, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if page.Total != 2 || page.Entries[0].Score != 150 {
		t.Fatalf("unexpected result: %+v", page)
	}
}

func TestService_GetJobMasteryAndPopularity(t *testing.T) {
	repo := newMockRepo()
	repo.jobMasteryRankings = []ranking.CharacterRankingEntry{
		{Rank: 1, CharacterID: "c1", CharacterName: "Grandmaster", Score: 8, Level: 99},
	}
	repo.jobMasteryTotal = 1

	repo.jobPopularityRankings = []ranking.JobPopularityEntry{
		{Rank: 1, JobID: "warrior", TotalCount: 40, MaleCount: 25, FemaleCount: 15, Percentage: 40.0},
		{Rank: 2, JobID: "mage", TotalCount: 30, MaleCount: 10, FemaleCount: 20, Percentage: 30.0},
	}

	svc, _ := ranking.NewService(repo)
	ctx := context.Background()

	masteryPage, err := svc.GetJobMasteryRanking(ctx, 10, 0, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if masteryPage.Total != 1 || masteryPage.Entries[0].Score != 8 {
		t.Fatalf("unexpected mastery page: %+v", masteryPage)
	}

	popPage, err := svc.GetJobPopularityRanking(ctx, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if popPage.Total != 2 || popPage.Entries[0].JobID != "warrior" {
		t.Fatalf("unexpected popularity page: %+v", popPage)
	}
}

func TestService_GetRankingByType(t *testing.T) {
	repo := newMockRepo()
	repo.levelRankings = []ranking.CharacterRankingEntry{
		{Rank: 1, CharacterID: "c1", CharacterName: "Hero", Score: 99},
	}
	repo.levelTotal = 1

	svc, _ := ranking.NewService(repo)
	ctx := context.Background()

	res, err := svc.GetRankingByType(ctx, ranking.RankingTypeLevel, 10, 0, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	page, ok := res.(ranking.RankingPage[ranking.CharacterRankingEntry])
	if !ok || page.Total != 1 {
		t.Fatalf("unexpected type/content: %+v", res)
	}

	_, err = svc.GetRankingByType(ctx, ranking.RankingType("non_existent"), 10, 0, false)
	if err != ranking.ErrInvalidRankingType {
		t.Fatalf("expected ErrInvalidRankingType, got %v", err)
	}
}

func TestService_RefreshAllSnapshots(t *testing.T) {
	repo := newMockRepo()
	svc, _ := ranking.NewService(repo)
	ctx := context.Background()

	if err := svc.RefreshAllSnapshots(ctx); err != nil {
		t.Fatalf("unexpected error refreshing all snapshots: %v", err)
	}

	if len(repo.snapshots) != 12 {
		t.Fatalf("expected 12 snapshots saved in repository, got %d", len(repo.snapshots))
	}
}
