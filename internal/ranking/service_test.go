package ranking_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/ranking"
)

type mockRankingRepository struct {
	monsterKillsRankings     []ranking.CharacterRankingEntry
	monsterKillsTotal        int
	maoCountRankings         []ranking.CharacterRankingEntry
	maoCountTotal            int
	heroCountRankings        []ranking.CharacterRankingEntry
	heroCountTotal           int
	pvpRankings              []ranking.CharacterRankingEntry
	pvpTotal                 int
	jobPopularityRankings    []ranking.JobPopularityEntry
	helperRankings           []ranking.CharacterRankingEntry
	helperTotal              int
	casinoWinsRankings       []ranking.CharacterRankingEntry
	casinoWinsTotal          int
	alchemyRankings          []ranking.CharacterRankingEntry
	alchemyTotal             int
	weeklyJobChangeRankings  []ranking.CharacterRankingEntry
	weeklyJobChangeTotal     int
	activeWeeklyJobChanges   map[string]int
	legends                  map[ranking.LegendCategory][]ranking.LegendEntry
	snapshots                map[ranking.RankingType]ranking.RankingSnapshot
	err                      error
	getSnapshotErr           error
	saveSnapshotErr          error
	resetWeeklyJobChangesErr error
	lockWeeklyJobChangesErr  error
	weeklyRotationDue        time.Time
	getWeeklyRotationDueErr  error
	saveWeeklyRotationDueErr error
}

func newMockRepo() *mockRankingRepository {
	return &mockRankingRepository{
		snapshots:              make(map[ranking.RankingType]ranking.RankingSnapshot),
		activeWeeklyJobChanges: make(map[string]int),
		legends:                make(map[ranking.LegendCategory][]ranking.LegendEntry),
	}
}

func (m *mockRankingRepository) RunInTx(ctx context.Context, fn func(ctx context.Context) error) error {
	savedDue := m.weeklyRotationDue
	savedSnapshots := make(map[ranking.RankingType]ranking.RankingSnapshot)
	for k, v := range m.snapshots {
		savedSnapshots[k] = v
	}
	savedActive := make(map[string]int)
	for k, v := range m.activeWeeklyJobChanges {
		savedActive[k] = v
	}

	if err := fn(ctx); err != nil {
		m.weeklyRotationDue = savedDue
		m.snapshots = savedSnapshots
		m.activeWeeklyJobChanges = savedActive
		return err
	}
	return nil
}

func (m *mockRankingRepository) GetMonsterKillsRanking(ctx context.Context, limit, offset int) ([]ranking.CharacterRankingEntry, int, error) {
	if m.err != nil {
		return nil, 0, m.err
	}
	return paginateSlice(m.monsterKillsRankings, limit, offset), m.monsterKillsTotal, nil
}

func (m *mockRankingRepository) GetMaoCountRanking(ctx context.Context, limit, offset int) ([]ranking.CharacterRankingEntry, int, error) {
	if m.err != nil {
		return nil, 0, m.err
	}
	return paginateSlice(m.maoCountRankings, limit, offset), m.maoCountTotal, nil
}

func (m *mockRankingRepository) GetHeroCountRanking(ctx context.Context, limit, offset int) ([]ranking.CharacterRankingEntry, int, error) {
	if m.err != nil {
		return nil, 0, m.err
	}
	return paginateSlice(m.heroCountRankings, limit, offset), m.heroCountTotal, nil
}

func (m *mockRankingRepository) GetPvPVictoryRanking(ctx context.Context, limit, offset int) ([]ranking.CharacterRankingEntry, int, error) {
	if m.err != nil {
		return nil, 0, m.err
	}
	return paginateSlice(m.pvpRankings, limit, offset), m.pvpTotal, nil
}

func (m *mockRankingRepository) GetJobPopularityRanking(ctx context.Context) ([]ranking.JobPopularityEntry, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.jobPopularityRankings, nil
}

func (m *mockRankingRepository) RecordJobPopularity(ctx context.Context, jobID string, gender string, points int) error {
	if m.err != nil {
		return m.err
	}
	for i := range m.jobPopularityRankings {
		if m.jobPopularityRankings[i].JobID == jobID {
			if gender == "m" || gender == "male" {
				m.jobPopularityRankings[i].MaleCount += points
			} else {
				m.jobPopularityRankings[i].FemaleCount += points
			}
			m.jobPopularityRankings[i].TotalCount += points
			return nil
		}
	}
	var maleCount, femaleCount int
	if gender == "m" || gender == "male" {
		maleCount = points
	} else {
		femaleCount = points
	}
	m.jobPopularityRankings = append(m.jobPopularityRankings, ranking.JobPopularityEntry{
		JobID:       jobID,
		TotalCount:  points,
		MaleCount:   maleCount,
		FemaleCount: femaleCount,
	})
	return nil
}

func (m *mockRankingRepository) GetHelperRanking(ctx context.Context, limit, offset int) ([]ranking.CharacterRankingEntry, int, error) {
	if m.err != nil {
		return nil, 0, m.err
	}
	return paginateSlice(m.helperRankings, limit, offset), m.helperTotal, nil
}

func (m *mockRankingRepository) SaveSnapshot(ctx context.Context, snapshot ranking.RankingSnapshot) error {
	if m.saveSnapshotErr != nil {
		return m.saveSnapshotErr
	}
	if m.err != nil {
		return m.err
	}
	m.snapshots[snapshot.RankingType] = snapshot
	return nil
}

func (m *mockRankingRepository) GetSnapshot(ctx context.Context, rankingType ranking.RankingType) (ranking.RankingSnapshot, error) {
	if m.getSnapshotErr != nil {
		return ranking.RankingSnapshot{}, m.getSnapshotErr
	}
	if m.err != nil {
		return ranking.RankingSnapshot{}, m.err
	}
	s, ok := m.snapshots[rankingType]
	if !ok {
		return ranking.RankingSnapshot{}, ranking.ErrSnapshotNotFound
	}
	return s, nil
}

func (m *mockRankingRepository) GetAllSnapshots(ctx context.Context) (map[ranking.RankingType]ranking.RankingSnapshot, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.snapshots, nil
}

func (m *mockRankingRepository) GetCasinoWinsRanking(ctx context.Context, limit, offset int) ([]ranking.CharacterRankingEntry, int, error) {
	if m.err != nil {
		return nil, 0, m.err
	}
	return paginateSlice(m.casinoWinsRankings, limit, offset), m.casinoWinsTotal, nil
}

func (m *mockRankingRepository) GetAlchemyRanking(ctx context.Context, limit, offset int) ([]ranking.CharacterRankingEntry, int, error) {
	if m.err != nil {
		return nil, 0, m.err
	}
	return paginateSlice(m.alchemyRankings, limit, offset), m.alchemyTotal, nil
}

func (m *mockRankingRepository) IncrementJobChangeCount(ctx context.Context, characterID string) error {
	if m.err != nil {
		return m.err
	}
	m.activeWeeklyJobChanges[characterID]++
	return nil
}

func (m *mockRankingRepository) GetActiveWeeklyJobChangeRanking(ctx context.Context, limit, offset int) ([]ranking.CharacterRankingEntry, int, error) {
	if m.err != nil {
		return nil, 0, m.err
	}
	return paginateSlice(m.weeklyJobChangeRankings, limit, offset), m.weeklyJobChangeTotal, nil
}

func (m *mockRankingRepository) ResetWeeklyJobChanges(ctx context.Context) error {
	if m.resetWeeklyJobChangesErr != nil {
		return m.resetWeeklyJobChangesErr
	}
	if m.err != nil {
		return m.err
	}
	m.activeWeeklyJobChanges = make(map[string]int)
	return nil
}

func (m *mockRankingRepository) LockWeeklyJobChangesForUpdate(ctx context.Context) error {
	if m.lockWeeklyJobChangesErr != nil {
		return m.lockWeeklyJobChangesErr
	}
	if m.err != nil {
		return m.err
	}
	return nil
}

func (m *mockRankingRepository) GetWeeklyRotationDue(ctx context.Context) (time.Time, error) {
	return m.weeklyRotationDue, m.getWeeklyRotationDueErr
}

func (m *mockRankingRepository) SaveWeeklyRotationDue(ctx context.Context, due time.Time) error {
	if m.saveWeeklyRotationDueErr != nil {
		return m.saveWeeklyRotationDueErr
	}
	m.weeklyRotationDue = due
	return nil
}

func (m *mockRankingRepository) RecordLegend(ctx context.Context, entry ranking.LegendEntry) (bool, error) {
	if m.err != nil {
		return false, m.err
	}
	for _, existing := range m.legends[entry.Category] {
		if existing.CharacterID == entry.CharacterID {
			return false, nil
		}
	}
	entry.ID = int64(len(m.legends[entry.Category]) + 1)
	m.legends[entry.Category] = append(m.legends[entry.Category], entry)
	return true, nil
}

func (m *mockRankingRepository) GetLegendInductees(ctx context.Context, category ranking.LegendCategory) ([]ranking.LegendEntry, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.legends[category], nil
}

func (m *mockRankingRepository) GetLegendCategoryCounts(ctx context.Context) (map[ranking.LegendCategory]int, error) {
	if m.err != nil {
		return nil, m.err
	}
	counts := make(map[ranking.LegendCategory]int)
	for cat, entries := range m.legends {
		counts[cat] = len(entries)
	}
	return counts, nil
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

func TestService_GetAuthenticCombatRankings(t *testing.T) {
	repo := newMockRepo()
	repo.monsterKillsRankings = []ranking.CharacterRankingEntry{
		{Rank: 1, CharacterID: "c1", CharacterName: "MonsterSlayer", Score: 150, Level: 30},
		{Rank: 2, CharacterID: "c2", CharacterName: "Hunter", Score: 95, Level: 28},
	}
	repo.monsterKillsTotal = 2

	repo.maoCountRankings = []ranking.CharacterRankingEntry{
		{Rank: 1, CharacterID: "c3", CharacterName: "DemonBane", Score: 5, Level: 50},
	}
	repo.maoCountTotal = 1

	repo.heroCountRankings = []ranking.CharacterRankingEntry{
		{Rank: 1, CharacterID: "c4", CharacterName: "LegendaryHero", Score: 12, Level: 45},
	}
	repo.heroCountTotal = 1

	svc, _ := ranking.NewService(repo)
	ctx := context.Background()

	// Before snapshots are published, public queries return empty (never live data).
	if mkPage, err := svc.GetMonsterKillsRanking(ctx, 10, 0); err != nil || mkPage.Total != 0 || len(mkPage.Entries) != 0 || mkPage.IsSnapshot {
		t.Fatalf("expected empty unpublished monster kills page, got %+v, %v", mkPage, err)
	}

	if err := svc.RefreshAllSnapshots(ctx); err != nil {
		t.Fatalf("RefreshAllSnapshots failed: %v", err)
	}

	// Monster Kills (kill_m / 英雄ランキング)
	mkPage, err := svc.GetMonsterKillsRanking(ctx, 10, 0)
	if err != nil {
		t.Fatalf("unexpected error for monster kills: %v", err)
	}
	if !mkPage.IsSnapshot || mkPage.Total != 2 || mkPage.Entries[0].Score != 150 {
		t.Fatalf("unexpected monster kills result: %+v", mkPage)
	}

	// Mao Count (mao_c / 魔王ランキング)
	maoPage, err := svc.GetMaoCountRanking(ctx, 10, 0)
	if err != nil {
		t.Fatalf("unexpected error for mao count: %v", err)
	}
	if !maoPage.IsSnapshot || maoPage.Total != 1 || maoPage.Entries[0].Score != 5 {
		t.Fatalf("unexpected mao count result: %+v", maoPage)
	}

	// Hero Count (hero_c / 勇者ランキング)
	heroPage, err := svc.GetHeroCountRanking(ctx, 10, 0)
	if err != nil {
		t.Fatalf("unexpected error for hero count: %v", err)
	}
	if !heroPage.IsSnapshot || heroPage.Total != 1 || heroPage.Entries[0].Score != 12 {
		t.Fatalf("unexpected hero count result: %+v", heroPage)
	}
}

func TestService_GetJobPopularity(t *testing.T) {
	repo := newMockRepo()
	repo.jobPopularityRankings = []ranking.JobPopularityEntry{
		{Rank: 1, JobID: "warrior", TotalCount: 40, MaleCount: 25, FemaleCount: 15, Percentage: 40.0},
		{Rank: 2, JobID: "mage", TotalCount: 30, MaleCount: 10, FemaleCount: 20, Percentage: 30.0},
	}

	svc, _ := ranking.NewService(repo)
	ctx := context.Background()

	// Unpublished job popularity returns empty (never live reads).
	unpublished, err := svc.GetJobPopularityRanking(ctx)
	if err != nil || unpublished.Total != 0 || len(unpublished.Entries) != 0 || unpublished.IsSnapshot {
		t.Fatalf("expected empty unpublished popularity page, got %+v, %v", unpublished, err)
	}

	if err := svc.RefreshSnapshot(ctx, ranking.RankingTypeJobPopularity); err != nil {
		t.Fatalf("RefreshSnapshot(job_popularity) failed: %v", err)
	}

	popPage, err := svc.GetJobPopularityRanking(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !popPage.IsSnapshot || popPage.Total != 2 || popPage.Entries[0].JobID != "warrior" {
		t.Fatalf("unexpected popularity page: %+v", popPage)
	}
}

func TestService_GetRankingByType(t *testing.T) {
	repo := newMockRepo()
	repo.monsterKillsRankings = []ranking.CharacterRankingEntry{
		{Rank: 1, CharacterID: "c1", CharacterName: "Hero", Score: 99},
	}
	repo.monsterKillsTotal = 1

	svc, _ := ranking.NewService(repo)
	ctx := context.Background()
	if err := svc.RefreshSnapshot(ctx, ranking.RankingTypeMonsterKills); err != nil {
		t.Fatalf("RefreshSnapshot failed: %v", err)
	}

	res, err := svc.GetRankingByType(ctx, ranking.RankingTypeMonsterKills, 10, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	page, ok := res.(ranking.RankingPage[ranking.CharacterRankingEntry])
	if !ok || page.Total != 1 {
		t.Fatalf("unexpected type/content: %+v", res)
	}

	_, err = svc.GetRankingByType(ctx, ranking.RankingType("non_existent"), 10, 0)
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

	if len(repo.snapshots) != 8 {
		t.Fatalf("expected 8 snapshots saved in repository, got %d", len(repo.snapshots))
	}
}

func TestService_GetSnapshot_And_GetAllSnapshots(t *testing.T) {
	repo := newMockRepo()
	fixedTime := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)
	repo.snapshots[ranking.RankingTypeMonsterKills] = ranking.RankingSnapshot{
		RankingType:  ranking.RankingTypeMonsterKills,
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
	s, err := svc.GetSnapshot(ctx, ranking.RankingTypeMonsterKills)
	if err != nil {
		t.Fatalf("unexpected error from GetSnapshot: %v", err)
	}
	if s.TotalCount != 1 || s.RankingType != ranking.RankingTypeMonsterKills {
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
	repo.snapshots[ranking.RankingTypeMonsterKills] = ranking.RankingSnapshot{
		RankingType:  ranking.RankingTypeMonsterKills,
		SnapshotData: `[{"rank":1,"character_id":"c1","character_name":"Hero","score":99},{"rank":2,"character_id":"c2","character_name":"Mage","score":85}]`,
		TotalCount:   2,
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
	repo.monsterKillsRankings = nil
	repo.monsterKillsTotal = 0
	repo.jobPopularityRankings = nil

	svc, err := ranking.NewService(repo, ranking.WithNowFunc(func() time.Time { return fixedTime }))
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	ctx := context.Background()

	// 1. Character ranking snapshot DB fallback
	mkPage, err := svc.GetMonsterKillsRanking(ctx, 10, 0)
	if err != nil {
		t.Fatalf("unexpected error for monster kills ranking: %v", err)
	}
	if !mkPage.IsSnapshot || mkPage.Total != 2 || len(mkPage.Entries) != 2 {
		t.Fatalf("expected snapshot fallback for monster kills ranking, got %+v", mkPage)
	}
	if mkPage.Entries[0].CharacterName != "Hero" || mkPage.Entries[1].CharacterName != "Mage" {
		t.Fatalf("unexpected monster kills entries: %+v", mkPage.Entries)
	}

	// 2. Second read hits in-memory cache (modify repo snapshot to verify memory cache is used)
	delete(repo.snapshots, ranking.RankingTypeMonsterKills)
	cachedMKPage, err := svc.GetMonsterKillsRanking(ctx, 10, 0)
	if err != nil {
		t.Fatalf("unexpected error for cached monster kills ranking: %v", err)
	}
	if !cachedMKPage.IsSnapshot || cachedMKPage.Total != 2 {
		t.Fatalf("expected in-memory cached monster kills page, got %+v", cachedMKPage)
	}

	// 3. Job popularity ranking snapshot DB fallback
	jobPopPage, err := svc.GetJobPopularityRanking(ctx)
	if err != nil {
		t.Fatalf("unexpected error for job popularity: %v", err)
	}
	if !jobPopPage.IsSnapshot || jobPopPage.Total != 1 || jobPopPage.Entries[0].JobID != "hero" {
		t.Fatalf("expected snapshot fallback for job popularity, got %+v", jobPopPage)
	}
}

func TestService_RecordJobPopularity(t *testing.T) {
	repo := newMockRepo()
	svc, err := ranking.NewService(repo)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	ctx := context.Background()

	// Empty job ID should fail
	if err := svc.RecordJobPopularity(ctx, "", "m", 10); err == nil {
		t.Error("expected error for empty job ID")
	}

	// Negative points clamped to 0
	if err := svc.RecordJobPopularity(ctx, "job-01", "m", -5); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Valid male record
	if err := svc.RecordJobPopularity(ctx, "job-01", "m", 10); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Valid female record
	if err := svc.RecordJobPopularity(ctx, "job-01", "f", 15); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify repo
	entries, err := repo.GetJobPopularityRanking(ctx)
	if err != nil {
		t.Fatalf("failed to get rankings: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].JobID != "job-01" {
		t.Errorf("expected job-01, got %s", entries[0].JobID)
	}
	if entries[0].TotalCount != 25 {
		t.Errorf("expected total 25, got %d", entries[0].TotalCount)
	}
	if entries[0].MaleCount != 10 {
		t.Errorf("expected male 10, got %d", entries[0].MaleCount)
	}
	if entries[0].FemaleCount != 15 {
		t.Errorf("expected female 15, got %d", entries[0].FemaleCount)
	}
}

func TestService_SnapshotReadDistinguishesAbsenceFromFailures(t *testing.T) {
	ctx := context.Background()
	fixedTime := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)

	characterGetters := []struct {
		name        string
		rankingType ranking.RankingType
		get         func(*ranking.Service, context.Context, int, int) (ranking.RankingPage[ranking.CharacterRankingEntry], error)
	}{
		{"pvp_victory", ranking.RankingTypePvPVictory, (*ranking.Service).GetPvPVictoryRanking},
		{"monster_kills", ranking.RankingTypeMonsterKills, (*ranking.Service).GetMonsterKillsRanking},
		{"mao_count", ranking.RankingTypeMaoCount, (*ranking.Service).GetMaoCountRanking},
		{"hero_count", ranking.RankingTypeHeroCount, (*ranking.Service).GetHeroCountRanking},
		{"casino_wins", ranking.RankingTypeCasinoWins, (*ranking.Service).GetCasinoWinsRanking},
		{"helper", ranking.RankingTypeHelper, (*ranking.Service).GetHelperRanking},
		{"alchemy", ranking.RankingTypeAlchemy, (*ranking.Service).GetAlchemyRanking},
		{"weekly_job_change", ranking.RankingTypeWeeklyJobChange, (*ranking.Service).GetWeeklyJobChangeRanking},
	}

	t.Run("absent snapshot returns empty non-snapshot page even when live data exists", func(t *testing.T) {
		repo := newMockRepo()
		repo.pvpRankings = []ranking.CharacterRankingEntry{{Rank: 1, CharacterID: "c1", Score: 10}}
		repo.monsterKillsRankings = repo.pvpRankings
		repo.maoCountRankings = repo.pvpRankings
		repo.heroCountRankings = repo.pvpRankings
		repo.casinoWinsRankings = repo.pvpRankings
		repo.helperRankings = repo.pvpRankings
		repo.alchemyRankings = repo.pvpRankings
		repo.weeklyJobChangeRankings = repo.pvpRankings
		repo.jobPopularityRankings = []ranking.JobPopularityEntry{{Rank: 1, JobID: "warrior", TotalCount: 5}}

		svc, err := ranking.NewService(repo, ranking.WithNowFunc(func() time.Time { return fixedTime }))
		if err != nil {
			t.Fatalf("NewService: %v", err)
		}

		for _, cg := range characterGetters {
			page, err := cg.get(svc, ctx, 10, 0)
			if err != nil {
				t.Fatalf("%s: expected nil error on ErrSnapshotNotFound, got %v", cg.name, err)
			}
			if page.IsSnapshot || page.Total != 0 || page.Entries == nil || len(page.Entries) != 0 {
				t.Fatalf("%s: expected empty non-snapshot page, got %+v", cg.name, page)
			}
			raw, err := svc.GetRankingByType(ctx, cg.rankingType, 10, 0)
			if err != nil {
				t.Fatalf("GetRankingByType(%s): expected nil error, got %v", cg.rankingType, err)
			}
			if p, ok := raw.(ranking.RankingPage[ranking.CharacterRankingEntry]); !ok || p.IsSnapshot || len(p.Entries) != 0 {
				t.Fatalf("GetRankingByType(%s): unexpected page %+v", cg.rankingType, raw)
			}
		}

		popPage, err := svc.GetJobPopularityRanking(ctx)
		if err != nil {
			t.Fatalf("job_popularity: expected nil error on ErrSnapshotNotFound, got %v", err)
		}
		if popPage.IsSnapshot || popPage.Total != 0 || popPage.Entries == nil || len(popPage.Entries) != 0 {
			t.Fatalf("job_popularity: expected empty non-snapshot page, got %+v", popPage)
		}
	})

	t.Run("sql error propagates without poisoning caches", func(t *testing.T) {
		sqlErr := errors.New("database connection lost")
		repo := newMockRepo()
		repo.getSnapshotErr = sqlErr
		cache := newMockSnapshotCache()

		svc, err := ranking.NewService(
			repo,
			ranking.WithSnapshotCache(cache),
			ranking.WithNowFunc(func() time.Time { return fixedTime }),
		)
		if err != nil {
			t.Fatalf("NewService: %v", err)
		}

		for _, cg := range characterGetters {
			if _, err := cg.get(svc, ctx, 10, 0); !errors.Is(err, sqlErr) {
				t.Fatalf("%s: expected %v, got %v", cg.name, sqlErr, err)
			}
			if _, err := svc.GetRankingByType(ctx, cg.rankingType, 10, 0); !errors.Is(err, sqlErr) {
				t.Fatalf("GetRankingByType(%s): expected %v, got %v", cg.rankingType, sqlErr, err)
			}
		}
		if _, err := svc.GetJobPopularityRanking(ctx); !errors.Is(err, sqlErr) {
			t.Fatalf("job_popularity: expected %v, got %v", sqlErr, err)
		}
		if _, err := svc.GetRankingByType(ctx, ranking.RankingTypeJobPopularity, 10, 0); !errors.Is(err, sqlErr) {
			t.Fatalf("GetRankingByType(job_popularity): expected %v, got %v", sqlErr, err)
		}

		// Clearing the SQL failure with a valid snapshot succeeds on the next read.
		repo.getSnapshotErr = nil
		repo.snapshots[ranking.RankingTypeMonsterKills] = ranking.RankingSnapshot{
			RankingType:  ranking.RankingTypeMonsterKills,
			SnapshotData: `[{"rank":1,"character_id":"c1","character_name":"Hero","score":5}]`,
			TotalCount:   1,
			CalculatedAt: fixedTime,
			UpdatedAt:    fixedTime,
		}
		page, err := svc.GetMonsterKillsRanking(ctx, 10, 0)
		if err != nil || !page.IsSnapshot || page.Total != 1 {
			t.Fatalf("expected recovery after SQL failure cleared, got %+v, %v", page, err)
		}
	})

	t.Run("invalid sql snapshot json propagates without poisoning caches", func(t *testing.T) {
		for _, badJSON := range []string{"", "{invalid", "[123]"} {
			repo := newMockRepo()
			cache := newMockSnapshotCache()
			for _, cg := range characterGetters {
				repo.snapshots[cg.rankingType] = ranking.RankingSnapshot{
					RankingType:  cg.rankingType,
					SnapshotData: badJSON,
					TotalCount:   1,
					CalculatedAt: fixedTime,
					UpdatedAt:    fixedTime,
				}
			}
			repo.snapshots[ranking.RankingTypeJobPopularity] = ranking.RankingSnapshot{
				RankingType:  ranking.RankingTypeJobPopularity,
				SnapshotData: badJSON,
				TotalCount:   1,
				CalculatedAt: fixedTime,
				UpdatedAt:    fixedTime,
			}

			svc, err := ranking.NewService(
				repo,
				ranking.WithSnapshotCache(cache),
				ranking.WithNowFunc(func() time.Time { return fixedTime }),
			)
			if err != nil {
				t.Fatalf("NewService: %v", err)
			}

			for _, cg := range characterGetters {
				_, err := cg.get(svc, ctx, 10, 0)
				if err == nil || errors.Is(err, ranking.ErrSnapshotNotFound) {
					t.Fatalf("%s (%q): expected decode error, got %v", cg.name, badJSON, err)
				}
			}
			if _, err := svc.GetJobPopularityRanking(ctx); err == nil || errors.Is(err, ranking.ErrSnapshotNotFound) {
				t.Fatalf("job_popularity (%q): expected decode error, got %v", badJSON, err)
			}
			if _, found, _ := cache.Get(ctx, ranking.RankingTypeMonsterKills); found {
				t.Fatalf("invalid snapshot (%q) populated Valkey cache", badJSON)
			}

			// Repairing SQL snapshot allows subsequent read to succeed.
			repo.snapshots[ranking.RankingTypeMonsterKills] = ranking.RankingSnapshot{
				RankingType:  ranking.RankingTypeMonsterKills,
				SnapshotData: `[{"rank":1,"character_id":"c1","character_name":"Hero","score":5}]`,
				TotalCount:   1,
				CalculatedAt: fixedTime,
				UpdatedAt:    fixedTime,
			}
			page, err := svc.GetMonsterKillsRanking(ctx, 10, 0)
			if err != nil || !page.IsSnapshot || page.Total != 1 {
				t.Fatalf("expected valid SQL snapshot after repair (%q), got %+v, %v", badJSON, page, err)
			}
		}
	})
}
