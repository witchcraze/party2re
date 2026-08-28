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

func TestService_GetSnapshot_And_GetAllSnapshots(t *testing.T) {
	repo := newMockRepo()
	fixedTime := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)
	repo.snapshots[ranking.RankingTypeLevel] = ranking.RankingSnapshot{
		RankingType:  ranking.RankingTypeLevel,
		SnapshotData: `[{"rank":1,"character_id":"c1","character_name":"Hero","score":99}]`,
		TotalCount:   1,
		CalculatedAt: fixedTime,
		UpdatedAt:    fixedTime,
	}

	svc, err := ranking.NewService(repo, ranking.WithNowFunc(func() time.Time { return fixedTime }))
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	ctx := context.Background()

	// GetSnapshot
	s, err := svc.GetSnapshot(ctx, ranking.RankingTypeLevel)
	if err != nil {
		t.Fatalf("unexpected error from GetSnapshot: %v", err)
	}
	if s.TotalCount != 1 || s.RankingType != ranking.RankingTypeLevel {
		t.Fatalf("unexpected snapshot: %+v", s)
	}

	// GetSnapshot invalid type
	_, err = svc.GetSnapshot(ctx, ranking.RankingType("invalid_type"))
	if err != ranking.ErrInvalidRankingType {
		t.Fatalf("expected ErrInvalidRankingType, got %v", err)
	}

	// GetAllSnapshots
	all, err := svc.GetAllSnapshots(ctx)
	if err != nil {
		t.Fatalf("unexpected error from GetAllSnapshots: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("expected 1 snapshot, got %d", len(all))
	}
}

func TestService_SnapshotDatabaseFallback_OnCacheMiss(t *testing.T) {
	repo := newMockRepo()
	fixedTime := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)

	// Persist snapshots in repo directly (simulating pre-existing DB snapshots on fresh server start)
	repo.snapshots[ranking.RankingTypeLevel] = ranking.RankingSnapshot{
		RankingType:  ranking.RankingTypeLevel,
		SnapshotData: `[{"rank":1,"character_id":"c1","character_name":"Hero","score":99},{"rank":2,"character_id":"c2","character_name":"Mage","score":85}]`,
		TotalCount:   2,
		CalculatedAt: fixedTime,
		UpdatedAt:    fixedTime,
	}
	repo.snapshots[ranking.RankingTypePlayerWealth] = ranking.RankingSnapshot{
		RankingType:  ranking.RankingTypePlayerWealth,
		SnapshotData: `[{"rank":1,"player_id":"p1","username":"Alice","total_wealth":500000}]`,
		TotalCount:   1,
		CalculatedAt: fixedTime,
		UpdatedAt:    fixedTime,
	}
	repo.snapshots[ranking.RankingTypeJobPopularity] = ranking.RankingSnapshot{
		RankingType:  ranking.RankingTypeJobPopularity,
		SnapshotData: `[{"rank":1,"job_id":"hero","total_count":10,"percentage":100.0}]`,
		TotalCount:   1,
		CalculatedAt: fixedTime,
		UpdatedAt:    fixedTime,
	}

	// Empty live tables in repo to guarantee live query would return empty / different data
	repo.levelRankings = nil
	repo.levelTotal = 0
	repo.playerWealthRankings = nil
	repo.playerWealthTotal = 0
	repo.jobPopularityRankings = nil

	svc, err := ranking.NewService(repo, ranking.WithNowFunc(func() time.Time { return fixedTime }))
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	ctx := context.Background()

	// 1. Character ranking snapshot DB fallback
	levelPage, err := svc.GetLevelRanking(ctx, 10, 0, true)
	if err != nil {
		t.Fatalf("unexpected error for level ranking: %v", err)
	}
	if !levelPage.IsSnapshot || levelPage.Total != 2 || len(levelPage.Entries) != 2 {
		t.Fatalf("expected snapshot fallback for level ranking, got %+v", levelPage)
	}
	if levelPage.Entries[0].CharacterName != "Hero" || levelPage.Entries[1].CharacterName != "Mage" {
		t.Fatalf("unexpected level entries: %+v", levelPage.Entries)
	}

	// 2. Second read hits in-memory cache (modify repo snapshot to verify memory cache is used)
	delete(repo.snapshots, ranking.RankingTypeLevel)
	cachedLevelPage, err := svc.GetLevelRanking(ctx, 10, 0, true)
	if err != nil {
		t.Fatalf("unexpected error for cached level ranking: %v", err)
	}
	if !cachedLevelPage.IsSnapshot || cachedLevelPage.Total != 2 {
		t.Fatalf("expected in-memory cached level page, got %+v", cachedLevelPage)
	}

	// 3. Player wealth ranking snapshot DB fallback
	wealthPage, err := svc.GetPlayerWealthRanking(ctx, 10, 0, true)
	if err != nil {
		t.Fatalf("unexpected error for wealth ranking: %v", err)
	}
	if !wealthPage.IsSnapshot || wealthPage.Total != 1 || wealthPage.Entries[0].Username != "Alice" {
		t.Fatalf("expected snapshot fallback for wealth ranking, got %+v", wealthPage)
	}

	// 4. Job popularity ranking snapshot DB fallback
	jobPopPage, err := svc.GetJobPopularityRanking(ctx, true)
	if err != nil {
		t.Fatalf("unexpected error for job popularity: %v", err)
	}
	if !jobPopPage.IsSnapshot || jobPopPage.Total != 1 || jobPopPage.Entries[0].JobID != "hero" {
		t.Fatalf("expected snapshot fallback for job popularity, got %+v", jobPopPage)
	}
}

func TestService_WarmupCache(t *testing.T) {
	repo := newMockRepo()
	fixedTime := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)

	repo.snapshots[ranking.RankingTypeLevel] = ranking.RankingSnapshot{
		RankingType:  ranking.RankingTypeLevel,
		SnapshotData: `[{"rank":1,"character_id":"c1","character_name":"Hero","score":99}]`,
		TotalCount:   1,
		CalculatedAt: fixedTime,
		UpdatedAt:    fixedTime,
	}
	repo.snapshots[ranking.RankingTypePlayerWealth] = ranking.RankingSnapshot{
		RankingType:  ranking.RankingTypePlayerWealth,
		SnapshotData: `[{"rank":1,"player_id":"p1","username":"Alice","total_wealth":500000}]`,
		TotalCount:   1,
		CalculatedAt: fixedTime,
		UpdatedAt:    fixedTime,
	}
	repo.snapshots[ranking.RankingTypeJobPopularity] = ranking.RankingSnapshot{
		RankingType:  ranking.RankingTypeJobPopularity,
		SnapshotData: `[{"rank":1,"job_id":"hero","total_count":10,"percentage":100.0}]`,
		TotalCount:   1,
		CalculatedAt: fixedTime,
		UpdatedAt:    fixedTime,
	}

	svc, err := ranking.NewService(repo, ranking.WithNowFunc(func() time.Time { return fixedTime }))
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	ctx := context.Background()

	// Warmup cache
	if err := svc.WarmupCache(ctx); err != nil {
		t.Fatalf("failed to warmup cache: %v", err)
	}

	// Delete from repo to verify all snapshots are served purely from in-memory cache
	repo.snapshots = make(map[ranking.RankingType]ranking.RankingSnapshot)

	lvl, err := svc.GetLevelRanking(ctx, 10, 0, true)
	if err != nil || !lvl.IsSnapshot || lvl.Total != 1 {
		t.Fatalf("expected level ranking from prewarmed cache, got %+v (err: %v)", lvl, err)
	}

	wealth, err := svc.GetPlayerWealthRanking(ctx, 10, 0, true)
	if err != nil || !wealth.IsSnapshot || wealth.Total != 1 {
		t.Fatalf("expected player wealth ranking from prewarmed cache, got %+v (err: %v)", wealth, err)
	}

	pop, err := svc.GetJobPopularityRanking(ctx, true)
	if err != nil || !pop.IsSnapshot || pop.Total != 1 {
		t.Fatalf("expected job popularity ranking from prewarmed cache, got %+v (err: %v)", pop, err)
	}
}
