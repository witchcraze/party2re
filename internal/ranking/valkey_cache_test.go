package ranking_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/ranking"
	"github.com/witchcraze/party2re/internal/valkey"
)

type mockSnapshotCache struct {
	mu     sync.Mutex
	data   map[ranking.RankingType]ranking.RankingSnapshot
	getErr error
}

func newMockSnapshotCache() *mockSnapshotCache {
	return &mockSnapshotCache{
		data: make(map[ranking.RankingType]ranking.RankingSnapshot),
	}
}

func (m *mockSnapshotCache) Get(ctx context.Context, rankingType ranking.RankingType) (ranking.RankingSnapshot, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.getErr != nil {
		return ranking.RankingSnapshot{}, false, m.getErr
	}
	snap, ok := m.data[rankingType]
	if !ok {
		return ranking.RankingSnapshot{}, false, nil
	}
	return snap, true, nil
}

func (m *mockSnapshotCache) Set(ctx context.Context, rankingType ranking.RankingType, snapshot ranking.RankingSnapshot, ttl time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[rankingType] = snapshot
	return nil
}

func (m *mockSnapshotCache) Delete(ctx context.Context, rankingType ranking.RankingType) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, rankingType)
	return nil
}

func TestService_ValkeyCache_HitAndInvalidation(t *testing.T) {
	ctx := context.Background()
	repo := newMockRepo()
	repo.monsterKillsRankings = []ranking.CharacterRankingEntry{
		{Rank: 1, CharacterID: "char-1", CharacterName: "Alice", Level: 50, Score: 100},
	}
	repo.monsterKillsTotal = 1

	mockCache := newMockSnapshotCache()
	fixedTime := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)
	svc, err := ranking.NewService(
		repo,
		ranking.WithSnapshotCache(mockCache),
		ranking.WithNowFunc(func() time.Time { return fixedTime }),
	)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	// 1. Refresh snapshot -> should save to repo and mockCache
	if err := svc.RefreshSnapshot(ctx, ranking.RankingTypeMonsterKills); err != nil {
		t.Fatalf("RefreshSnapshot: %v", err)
	}

	snap, found, err := mockCache.Get(ctx, ranking.RankingTypeMonsterKills)
	if err != nil || !found {
		t.Fatalf("expected snapshot in cache, found=%v, err=%v", found, err)
	}
	if snap.TotalCount != 1 {
		t.Errorf("expected total count 1 in cached snapshot, got %d", snap.TotalCount)
	}

	// 2. Query returns published snapshot from cache
	page, err := svc.GetMonsterKillsRanking(ctx, 10, 0)
	if err != nil {
		t.Fatalf("GetMonsterKillsRanking: %v", err)
	}
	if !page.IsSnapshot {
		t.Errorf("expected IsSnapshot = true")
	}
	if len(page.Entries) != 1 || page.Entries[0].CharacterName != "Alice" {
		t.Errorf("unexpected page entries: %+v", page.Entries)
	}

	// 3. Delete from cache
	_ = mockCache.Delete(ctx, ranking.RankingTypeMonsterKills)
	_, found, _ = mockCache.Get(ctx, ranking.RankingTypeMonsterKills)
	if found {
		t.Errorf("expected cache entry to be deleted")
	}
}

type countingRankingRepository struct {
	mockRankingRepository
	getSnapshotCalls int64
}

func (c *countingRankingRepository) GetSnapshot(ctx context.Context, rankingType ranking.RankingType) (ranking.RankingSnapshot, error) {
	atomic.AddInt64(&c.getSnapshotCalls, 1)
	time.Sleep(10 * time.Millisecond) // simulate db query delay
	return c.mockRankingRepository.GetSnapshot(ctx, rankingType)
}

func TestService_Singleflight_PreventsCacheStampede(t *testing.T) {
	ctx := context.Background()
	countingRepo := &countingRankingRepository{
		mockRankingRepository: *newMockRepo(),
	}
	countingRepo.monsterKillsRankings = []ranking.CharacterRankingEntry{
		{Rank: 1, CharacterID: "char-1", CharacterName: "Alice", Level: 50, Score: 100},
	}
	countingRepo.monsterKillsTotal = 1

	mockCache := newMockSnapshotCache()
	fixedTime := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)
	svc, err := ranking.NewService(
		countingRepo,
		ranking.WithSnapshotCache(mockCache),
		ranking.WithNowFunc(func() time.Time { return fixedTime }),
	)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	// Save snapshot directly in DB repo (not in cache)
	if err := countingRepo.SaveSnapshot(ctx, ranking.RankingSnapshot{
		RankingType:  ranking.RankingTypeMonsterKills,
		SnapshotData: `[{"rank":1,"character_id":"char-1","character_name":"Alice","level":50,"score":100}]`,
		TotalCount:   1,
		CalculatedAt: fixedTime,
		UpdatedAt:    fixedTime,
	}); err != nil {
		t.Fatalf("SaveSnapshot: %v", err)
	}

	// Launch 25 concurrent queries on cache miss
	const concurrency = 25
	var wg sync.WaitGroup
	errCh := make(chan error, concurrency)

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			page, err := svc.GetMonsterKillsRanking(ctx, 10, 0)
			if err != nil {
				errCh <- err
				return
			}
			if len(page.Entries) != 1 || page.Entries[0].CharacterName != "Alice" {
				errCh <- fmt.Errorf("unexpected entry: %+v", page.Entries)
			}
		}()
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Errorf("concurrent query error: %v", err)
	}

	calls := atomic.LoadInt64(&countingRepo.getSnapshotCalls)
	if calls > 2 {
		t.Errorf("expected singleflight to coalesce DB calls (<= 2 calls), got %d calls for %d concurrent requests", calls, concurrency)
	}
}

func TestValkeySnapshotCache_RealOrNop(t *testing.T) {
	// This probe also runs without an address and skips unreachable servers.
	client, err := valkey.NewClient()
	if err != nil {
		t.Skip("Valkey is not reachable")
	}
	defer client.Close()

	ctx := context.Background()
	cache := ranking.NewValkeySnapshotCache(client)

	fixedTime := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)
	testSnap := ranking.RankingSnapshot{
		RankingType:  ranking.RankingTypeMonsterKills,
		SnapshotData: `[{"rank":1,"character_id":"char-1","character_name":"Alice","level":50}]`,
		TotalCount:   1,
		CalculatedAt: fixedTime,
		UpdatedAt:    fixedTime,
	}

	// 1. Set snapshot
	if err := cache.Set(ctx, ranking.RankingTypeMonsterKills, testSnap, 2*time.Second); err != nil {
		t.Fatalf("Set: %v", err)
	}

	// 2. Get snapshot
	got, found, err := cache.Get(ctx, ranking.RankingTypeMonsterKills)
	if err != nil || !found {
		t.Fatalf("Get failed: found=%v, err=%v", found, err)
	}
	if got.TotalCount != 1 || got.SnapshotData != testSnap.SnapshotData {
		t.Errorf("unexpected snapshot: %+v", got)
	}

	// 3. Delete snapshot
	if err := cache.Delete(ctx, ranking.RankingTypeMonsterKills); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, found, err = cache.Get(ctx, ranking.RankingTypeMonsterKills)
	if err != nil || found {
		t.Errorf("expected not found after delete, got found=%v, err=%v", found, err)
	}
}

func TestService_SnapshotFallbackOnMissingOrUnusableValkey(t *testing.T) {
	ctx := context.Background()
	fixedTime := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)

	sqlBoard := ranking.RankingSnapshot{
		RankingType:  ranking.RankingTypeMonsterKills,
		SnapshotData: `[{"rank":1,"character_id":"c1","character_name":"Alice","score":100}]`,
		TotalCount:   1,
		CalculatedAt: fixedTime,
		UpdatedAt:    fixedTime,
	}
	sqlPop := ranking.RankingSnapshot{
		RankingType:  ranking.RankingTypeJobPopularity,
		SnapshotData: `[{"rank":1,"job_id":"warrior","total_count":10,"percentage":100}]`,
		TotalCount:   1,
		CalculatedAt: fixedTime,
		UpdatedAt:    fixedTime,
	}

	cases := []struct {
		name  string
		setup func(c *mockSnapshotCache)
	}{
		{
			name:  "cache miss",
			setup: func(c *mockSnapshotCache) {},
		},
		{
			name: "valkey read error",
			setup: func(c *mockSnapshotCache) {
				c.getErr = errors.New("valkey unavailable")
			},
		},
		{
			name: "empty snapshot_data in valkey",
			setup: func(c *mockSnapshotCache) {
				c.data[ranking.RankingTypeMonsterKills] = ranking.RankingSnapshot{RankingType: ranking.RankingTypeMonsterKills, SnapshotData: ""}
				c.data[ranking.RankingTypeJobPopularity] = ranking.RankingSnapshot{RankingType: ranking.RankingTypeJobPopularity, SnapshotData: ""}
			},
		},
		{
			name: "corrupt json in valkey",
			setup: func(c *mockSnapshotCache) {
				c.data[ranking.RankingTypeMonsterKills] = ranking.RankingSnapshot{RankingType: ranking.RankingTypeMonsterKills, SnapshotData: "{corrupt"}
				c.data[ranking.RankingTypeJobPopularity] = ranking.RankingSnapshot{RankingType: ranking.RankingTypeJobPopularity, SnapshotData: "{corrupt"}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := newMockRepo()
			repo.snapshots[ranking.RankingTypeMonsterKills] = sqlBoard
			repo.snapshots[ranking.RankingTypeJobPopularity] = sqlPop
			cache := newMockSnapshotCache()
			tc.setup(cache)

			svc, err := ranking.NewService(
				repo,
				ranking.WithSnapshotCache(cache),
				ranking.WithNowFunc(func() time.Time { return fixedTime }),
			)
			if err != nil {
				t.Fatalf("NewService: %v", err)
			}

			boardPage, err := svc.GetMonsterKillsRanking(ctx, 10, 0)
			if err != nil {
				t.Fatalf("GetMonsterKillsRanking failed: %v", err)
			}
			if !boardPage.IsSnapshot || boardPage.Total != 1 || len(boardPage.Entries) != 1 || boardPage.Entries[0].CharacterName != "Alice" {
				t.Fatalf("unexpected board fallback page: %+v", boardPage)
			}

			popPage, err := svc.GetJobPopularityRanking(ctx)
			if err != nil {
				t.Fatalf("GetJobPopularityRanking failed: %v", err)
			}
			if !popPage.IsSnapshot || popPage.Total != 1 || len(popPage.Entries) != 1 || popPage.Entries[0].JobID != "warrior" {
				t.Fatalf("unexpected popularity fallback page: %+v", popPage)
			}

			// Repaired cache entries serve subsequent reads after error is cleared.
			cache.getErr = nil
			repaired, found, err := cache.Get(ctx, ranking.RankingTypeMonsterKills)
			if err != nil || !found || repaired.SnapshotData != sqlBoard.SnapshotData {
				t.Fatalf("expected Valkey cache to be repaired from SQL, got found=%v err=%v snap=%+v", found, err, repaired)
			}
		})
	}
}
