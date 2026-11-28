package ranking_test

import (
	"context"
	"encoding/json"
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
	mu      sync.Mutex
	data    map[ranking.RankingType]ranking.RankingSnapshot
	pairs   map[string]ranking.SnapshotPair
	getErr  error
	setErr  error
	nowFunc func() time.Time
	expiry  map[string]time.Time
}

func newMockSnapshotCache() *mockSnapshotCache {
	return &mockSnapshotCache{
		data:   make(map[ranking.RankingType]ranking.RankingSnapshot),
		pairs:  make(map[string]ranking.SnapshotPair),
		expiry: make(map[string]time.Time),
	}
}

func mockPairKey(leader, dependent ranking.RankingType) string {
	return string(leader) + ":" + string(dependent)
}

func mockPairFor(t ranking.RankingType) (ranking.RankingType, ranking.RankingType, bool) {
	switch t {
	case ranking.RankingTypePvPVictory, ranking.RankingTypeMonsterKills:
		return ranking.RankingTypePvPVictory, ranking.RankingTypeMonsterKills, true
	case ranking.RankingTypeMaoCount, ranking.RankingTypeHeroCount:
		return ranking.RankingTypeMaoCount, ranking.RankingTypeHeroCount, true
	default:
		return "", "", false
	}
}

func isValidSnapshotJSON(raw string) bool {
	if raw == "" {
		return false
	}
	var v any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return false
	}
	_, ok := v.([]any)
	return ok
}

func (m *mockSnapshotCache) isExpiredLocked(key string) bool {
	if m.nowFunc == nil {
		return false
	}
	exp, ok := m.expiry[key]
	if !ok {
		return false
	}
	return !exp.After(m.nowFunc())
}

func (m *mockSnapshotCache) setExpiryLocked(key string, ttl time.Duration) {
	if m.nowFunc != nil && ttl > 0 {
		m.expiry[key] = m.nowFunc().Add(ttl)
	}
}

func (m *mockSnapshotCache) Get(ctx context.Context, rankingType ranking.RankingType) (ranking.RankingSnapshot, bool, error) {
	if l, d, ok := mockPairFor(rankingType); ok {
		sp, found, err := m.GetPair(ctx, l, d)
		if err != nil || !found {
			return ranking.RankingSnapshot{}, false, err
		}
		if rankingType == l {
			return sp.Leader, true, nil
		}
		return sp.Dependent, true, nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.getErr != nil {
		return ranking.RankingSnapshot{}, false, m.getErr
	}
	key := string(rankingType)
	if m.isExpiredLocked(key) {
		delete(m.data, rankingType)
		delete(m.expiry, key)
		return ranking.RankingSnapshot{}, false, nil
	}
	snap, ok := m.data[rankingType]
	if !ok {
		return ranking.RankingSnapshot{}, false, nil
	}
	return snap, true, nil
}

func shouldOverwriteMock(mode ranking.CacheWriteMode, existingValid bool, existingTS, newTS time.Time) bool {
	if !existingValid {
		return true
	}
	if mode == ranking.CacheWriteBackfill {
		return newTS.After(existingTS)
	}
	return !newTS.Before(existingTS)
}

func (m *mockSnapshotCache) Set(ctx context.Context, rankingType ranking.RankingType, snapshot ranking.RankingSnapshot, ttl time.Duration, mode ranking.CacheWriteMode) (bool, error) {
	if _, _, ok := mockPairFor(rankingType); ok {
		return false, fmt.Errorf("paired ranking %s must be cached via SetPair", rankingType)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.setErr != nil {
		return false, m.setErr
	}
	if ttl <= 0 {
		return false, nil
	}
	key := string(rankingType)
	if m.isExpiredLocked(key) {
		delete(m.data, rankingType)
		delete(m.expiry, key)
	}
	existing, ok := m.data[rankingType]
	valid := ok && isValidSnapshotJSON(existing.SnapshotData)
	if !shouldOverwriteMock(mode, valid, existing.CalculatedAt, snapshot.CalculatedAt) {
		return false, nil
	}
	m.data[rankingType] = snapshot
	m.setExpiryLocked(key, ttl)
	return true, nil
}

func (m *mockSnapshotCache) GetPair(ctx context.Context, leaderType, dependentType ranking.RankingType) (ranking.SnapshotPair, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.getErr != nil {
		return ranking.SnapshotPair{}, false, m.getErr
	}
	pk := mockPairKey(leaderType, dependentType)
	if m.isExpiredLocked("pair:" + pk) {
		delete(m.pairs, pk)
		delete(m.expiry, "pair:"+pk)
		return ranking.SnapshotPair{}, false, nil
	}
	sp, ok := m.pairs[pk]
	if !ok {
		return ranking.SnapshotPair{}, false, nil
	}
	return sp, true, nil
}

func (m *mockSnapshotCache) SetPair(ctx context.Context, pair ranking.SnapshotPair, ttl time.Duration, mode ranking.CacheWriteMode) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.setErr != nil {
		return false, m.setErr
	}
	if ttl <= 0 {
		return false, nil
	}
	pk := mockPairKey(pair.Leader.RankingType, pair.Dependent.RankingType)
	expKey := "pair:" + pk
	if m.isExpiredLocked(expKey) {
		delete(m.pairs, pk)
		delete(m.expiry, expKey)
	}
	existing, ok := m.pairs[pk]
	valid := ok && isValidSnapshotJSON(existing.Leader.SnapshotData) && isValidSnapshotJSON(existing.Dependent.SnapshotData)
	existingTS := existing.Leader.CalculatedAt
	if existing.Dependent.CalculatedAt.After(existingTS) {
		existingTS = existing.Dependent.CalculatedAt
	}
	newTS := pair.Leader.CalculatedAt
	if pair.Dependent.CalculatedAt.After(newTS) {
		newTS = pair.Dependent.CalculatedAt
	}
	if !shouldOverwriteMock(mode, valid, existingTS, newTS) {
		return false, nil
	}
	m.pairs[pk] = pair
	m.setExpiryLocked(expKey, ttl)
	return true, nil
}

func (m *mockSnapshotCache) Delete(ctx context.Context, rankingType ranking.RankingType) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if l, d, ok := mockPairFor(rankingType); ok {
		pk := mockPairKey(l, d)
		delete(m.pairs, pk)
		delete(m.expiry, "pair:"+pk)
		return nil
	}
	delete(m.data, rankingType)
	delete(m.expiry, string(rankingType))
	return nil
}

func TestService_ValkeyCache_HitAndInvalidation(t *testing.T) {
	ctx := context.Background()
	repo := newMockRepo()
	repo.pvpRankings = []ranking.CharacterRankingEntry{
		{Rank: 1, CharacterID: "char-0", CharacterName: "Leader", Level: 60, Score: 200},
	}
	repo.pvpTotal = 1
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

	// 3. Delete from cache removes the pair entry
	_ = mockCache.Delete(ctx, ranking.RankingTypeMonsterKills)
	_, found, _ = mockCache.Get(ctx, ranking.RankingTypeMonsterKills)
	if found {
		t.Errorf("expected cache entry to be deleted")
	}
	_, foundLeader, _ := mockCache.Get(ctx, ranking.RankingTypePvPVictory)
	if foundLeader {
		t.Errorf("expected paired leader cache entry to be deleted together")
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

func (c *countingRankingRepository) GetAllSnapshots(ctx context.Context) (map[ranking.RankingType]ranking.RankingSnapshot, error) {
	atomic.AddInt64(&c.getSnapshotCalls, 1)
	time.Sleep(10 * time.Millisecond) // simulate db query delay
	return c.mockRankingRepository.GetAllSnapshots(ctx)
}

func TestService_Singleflight_PreventsCacheStampede(t *testing.T) {
	ctx := context.Background()
	countingRepo := &countingRankingRepository{
		mockRankingRepository: *newMockRepo(),
	}
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

	// Save snapshots directly in DB repo (not in cache)
	if err := countingRepo.SaveSnapshot(ctx, ranking.RankingSnapshot{
		RankingType:  ranking.RankingTypePvPVictory,
		SnapshotData: `[{"rank":1,"character_id":"char-0","character_name":"Leader","level":60,"score":200}]`,
		TotalCount:   1,
		CalculatedAt: fixedTime,
		UpdatedAt:    fixedTime,
	}); err != nil {
		t.Fatalf("SaveSnapshot leader: %v", err)
	}
	if err := countingRepo.SaveSnapshot(ctx, ranking.RankingSnapshot{
		RankingType:  ranking.RankingTypeMonsterKills,
		SnapshotData: `[{"rank":1,"character_id":"char-1","character_name":"Alice","level":50,"score":100}]`,
		TotalCount:   1,
		CalculatedAt: fixedTime,
		UpdatedAt:    fixedTime,
	}); err != nil {
		t.Fatalf("SaveSnapshot dependent: %v", err)
	}

	// Launch 25 concurrent queries across both boards of the pair on cache miss
	const concurrency = 25
	var wg sync.WaitGroup
	errCh := make(chan error, concurrency)

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			if idx%2 == 0 {
				page, err := svc.GetMonsterKillsRanking(ctx, 10, 0)
				if err != nil {
					errCh <- err
					return
				}
				if len(page.Entries) != 1 || page.Entries[0].CharacterName != "Alice" {
					errCh <- fmt.Errorf("unexpected monster_kills entry: %+v", page.Entries)
				}
				return
			}
			page, err := svc.GetPvPVictoryRanking(ctx, 10, 0)
			if err != nil {
				errCh <- err
				return
			}
			if len(page.Entries) != 1 || page.Entries[0].CharacterName != "Leader" {
				errCh <- fmt.Errorf("unexpected pvp_victory entry: %+v", page.Entries)
			}
		}(i)
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
	_ = cache.Delete(ctx, ranking.RankingTypeHelper)
	_ = cache.Delete(ctx, ranking.RankingTypePvPVictory)
	defer func() {
		_ = cache.Delete(ctx, ranking.RankingTypeHelper)
		_ = cache.Delete(ctx, ranking.RankingTypePvPVictory)
	}()

	t1 := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)
	t2 := t1.Add(time.Minute)

	// 1. Standalone Set / Get / Backfill / Delete
	singleV1 := ranking.RankingSnapshot{
		RankingType:  ranking.RankingTypeHelper,
		SnapshotData: `[{"rank":1,"character_id":"char-1","character_name":"Alice","level":50}]`,
		TotalCount:   1,
		CalculatedAt: t1,
		UpdatedAt:    t1,
	}
	singleV2 := ranking.RankingSnapshot{
		RankingType:  ranking.RankingTypeHelper,
		SnapshotData: `[{"rank":1,"character_id":"char-2","character_name":"Bob","level":60}]`,
		TotalCount:   1,
		CalculatedAt: t2,
		UpdatedAt:    t2,
	}

	if written, err := cache.Set(ctx, ranking.RankingTypeHelper, singleV2, 5*time.Second, ranking.CacheWriteRefresh); err != nil || !written {
		t.Fatalf("Set singleV2: written=%v err=%v", written, err)
	}
	// Stale backfill must not overwrite newer entry.
	if written, err := cache.Set(ctx, ranking.RankingTypeHelper, singleV1, 5*time.Second, ranking.CacheWriteBackfill); err != nil || written {
		t.Fatalf("expected stale single backfill to be rejected, written=%v err=%v", written, err)
	}
	// Equal-timestamp backfill must not overwrite existing valid entry.
	if written, err := cache.Set(ctx, ranking.RankingTypeHelper, singleV2, 5*time.Second, ranking.CacheWriteBackfill); err != nil || written {
		t.Fatalf("expected equal-ts single backfill to be rejected, written=%v err=%v", written, err)
	}

	got, found, err := cache.Get(ctx, ranking.RankingTypeHelper)
	if err != nil || !found || got.SnapshotData != singleV2.SnapshotData {
		t.Fatalf("Get single failed: found=%v err=%v got=%+v", found, err, got)
	}
	if err := cache.Delete(ctx, ranking.RankingTypeHelper); err != nil {
		t.Fatalf("Delete single: %v", err)
	}

	// 2. Paired SetPair / GetPair / Backfill / Corrupt repair / Delete
	pairV1 := ranking.SnapshotPair{
		Leader: ranking.RankingSnapshot{
			RankingType:  ranking.RankingTypePvPVictory,
			SnapshotData: `[{"rank":1,"character_id":"a","score":10}]`,
			TotalCount:   1,
			CalculatedAt: t1,
			UpdatedAt:    t1,
		},
		Dependent: ranking.RankingSnapshot{
			RankingType:  ranking.RankingTypeMonsterKills,
			SnapshotData: `[{"rank":1,"character_id":"b","score":5}]`,
			TotalCount:   1,
			CalculatedAt: t1,
			UpdatedAt:    t1,
		},
	}
	pairV2 := ranking.SnapshotPair{
		Leader: ranking.RankingSnapshot{
			RankingType:  ranking.RankingTypePvPVictory,
			SnapshotData: `[{"rank":1,"character_id":"b","score":20}]`,
			TotalCount:   1,
			CalculatedAt: t2,
			UpdatedAt:    t2,
		},
		Dependent: ranking.RankingSnapshot{
			RankingType:  ranking.RankingTypeMonsterKills,
			SnapshotData: `[{"rank":1,"character_id":"a","score":15}]`,
			TotalCount:   1,
			CalculatedAt: t2,
			UpdatedAt:    t2,
		},
	}

	if written, err := cache.SetPair(ctx, pairV2, 5*time.Second, ranking.CacheWriteRefresh); err != nil || !written {
		t.Fatalf("SetPair V2: written=%v err=%v", written, err)
	}
	if written, err := cache.SetPair(ctx, pairV1, 5*time.Second, ranking.CacheWriteBackfill); err != nil || written {
		t.Fatalf("expected stale pair backfill to be rejected, written=%v err=%v", written, err)
	}
	if written, err := cache.SetPair(ctx, pairV2, 5*time.Second, ranking.CacheWriteBackfill); err != nil || written {
		t.Fatalf("expected equal-ts pair backfill to be rejected, written=%v err=%v", written, err)
	}
	// Stale refresh must not overwrite newer refresh.
	if written, err := cache.SetPair(ctx, pairV1, 5*time.Second, ranking.CacheWriteRefresh); err != nil || written {
		t.Fatalf("expected stale pair refresh to be rejected, written=%v err=%v", written, err)
	}
	// Same-timestamp refresh must overwrite (supports fixed clocks under refreshMu).
	if written, err := cache.SetPair(ctx, pairV2, 5*time.Second, ranking.CacheWriteRefresh); err != nil || !written {
		t.Fatalf("expected same-ts pair refresh to succeed, written=%v err=%v", written, err)
	}

	gotPair, found, err := cache.GetPair(ctx, ranking.RankingTypePvPVictory, ranking.RankingTypeMonsterKills)
	if err != nil || !found || gotPair.Leader.SnapshotData != pairV2.Leader.SnapshotData || gotPair.Dependent.SnapshotData != pairV2.Dependent.SnapshotData {
		t.Fatalf("GetPair failed: found=%v err=%v pair=%+v", found, err, gotPair)
	}

	// Corrupt JSON stored under pair key must be repaired by backfill.
	if err := client.Do(ctx, client.B().Set().Key("party2:ranking:snapshot:pair:pvp_victory:monster_kills").Value("{corrupt").Ex(5*time.Second).Build()).Error(); err != nil {
		t.Fatalf("seed corrupt valkey value: %v", err)
	}
	if written, err := cache.SetPair(ctx, pairV2, 5*time.Second, ranking.CacheWriteBackfill); err != nil || !written {
		t.Fatalf("expected backfill to repair corrupt pair value, written=%v err=%v", written, err)
	}

	if err := cache.Delete(ctx, ranking.RankingTypePvPVictory); err != nil {
		t.Fatalf("Delete pair: %v", err)
	}
	_, found, err = cache.GetPair(ctx, ranking.RankingTypePvPVictory, ranking.RankingTypeMonsterKills)
	if err != nil || found {
		t.Fatalf("expected pair not found after delete, found=%v err=%v", found, err)
	}
}

func TestService_ValkeyPairConsistency_RealOrNop(t *testing.T) {
	client, err := valkey.NewClient()
	if err != nil {
		t.Skip("Valkey is not reachable")
	}
	defer client.Close()

	ctx := context.Background()
	cache := ranking.NewValkeySnapshotCache(client)
	for _, rt := range []ranking.RankingType{ranking.RankingTypePvPVictory, ranking.RankingTypeMaoCount} {
		_ = cache.Delete(ctx, rt)
		defer func(tType ranking.RankingType) {
			_ = cache.Delete(ctx, tType)
		}(rt)
	}

	repo := newMockRepo()
	repo.pvpRankings = []ranking.CharacterRankingEntry{{Rank: 1, CharacterID: "a", Score: 10}}
	repo.pvpTotal = 1
	repo.monsterKillsRankings = []ranking.CharacterRankingEntry{
		{Rank: 1, CharacterID: "a", Score: 20},
		{Rank: 2, CharacterID: "b", Score: 15},
	}
	repo.monsterKillsTotal = 2
	repo.maoCountRankings = []ranking.CharacterRankingEntry{{Rank: 1, CharacterID: "a", Score: 10}}
	repo.maoCountTotal = 1
	repo.heroCountRankings = []ranking.CharacterRankingEntry{
		{Rank: 1, CharacterID: "a", Score: 20},
		{Rank: 2, CharacterID: "b", Score: 15},
	}
	repo.heroCountTotal = 2

	now := time.Date(2026, 11, 28, 4, 0, 0, 0, time.UTC)
	nowFn := func() time.Time { return now }

	writerSvc, err := ranking.NewService(repo, ranking.WithSnapshotCache(cache), ranking.WithNowFunc(nowFn))
	if err != nil {
		t.Fatal(err)
	}
	if err := writerSvc.RefreshAllSnapshots(ctx); err != nil {
		t.Fatalf("initial RefreshAllSnapshots: %v", err)
	}

	// Capture Gen 1 snapshots before Gen 2 refresh.
	gen1All, err := repo.GetAllSnapshots(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Publish Gen 2 ("b" moves to leader, "a" moves to dependent).
	now = now.Add(time.Minute)
	repo.pvpRankings = []ranking.CharacterRankingEntry{{Rank: 1, CharacterID: "b", Score: 30}}
	repo.maoCountRankings = []ranking.CharacterRankingEntry{{Rank: 1, CharacterID: "b", Score: 30}}
	if err := writerSvc.RefreshAllSnapshots(ctx); err != nil {
		t.Fatalf("Gen 2 RefreshAllSnapshots: %v", err)
	}

	// Simulate a delayed Gen 1 backfill hitting real Valkey Lua set_snapshot.lua.
	for _, pair := range []struct {
		leader, dependent ranking.RankingType
	}{
		{ranking.RankingTypePvPVictory, ranking.RankingTypeMonsterKills},
		{ranking.RankingTypeMaoCount, ranking.RankingTypeHeroCount},
	} {
		written, err := cache.SetPair(ctx, ranking.SnapshotPair{
			Leader:    gen1All[pair.leader],
			Dependent: gen1All[pair.dependent],
		}, 5*time.Minute, ranking.CacheWriteBackfill)
		if err != nil || written {
			t.Fatalf("expected real Valkey to reject delayed Gen 1 backfill for (%s, %s), written=%v err=%v", pair.leader, pair.dependent, written, err)
		}
	}

	// Fresh service must still read Gen 2 consistently from real Valkey.
	readerSvc, err := ranking.NewService(repo, ranking.WithSnapshotCache(cache), ranking.WithNowFunc(nowFn))
	if err != nil {
		t.Fatal(err)
	}
	pvpPage, err := readerSvc.GetPvPVictoryRanking(ctx, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	mkPage, err := readerSvc.GetMonsterKillsRanking(ctx, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(pvpPage.Entries) != 1 || pvpPage.Entries[0].CharacterID != "b" || len(mkPage.Entries) != 1 || mkPage.Entries[0].CharacterID != "a" {
		t.Fatalf("unexpected pvp/monster pair after delayed backfill: pvp=%+v mk=%+v", pvpPage.Entries, mkPage.Entries)
	}
	maoPage, err := readerSvc.GetMaoCountRanking(ctx, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	heroPage, err := readerSvc.GetHeroCountRanking(ctx, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(maoPage.Entries) != 1 || maoPage.Entries[0].CharacterID != "b" || len(heroPage.Entries) != 1 || heroPage.Entries[0].CharacterID != "a" {
		t.Fatalf("unexpected mao/hero pair after delayed backfill: mao=%+v hero=%+v", maoPage.Entries, heroPage.Entries)
	}
}

func TestService_SnapshotFallbackOnMissingOrUnusableValkey(t *testing.T) {
	ctx := context.Background()
	fixedTime := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)

	sqlLeader := ranking.RankingSnapshot{
		RankingType:  ranking.RankingTypePvPVictory,
		SnapshotData: `[{"rank":1,"character_id":"p1","character_name":"Leader","score":200}]`,
		TotalCount:   1,
		CalculatedAt: fixedTime,
		UpdatedAt:    fixedTime,
	}
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
				c.pairs[mockPairKey(ranking.RankingTypePvPVictory, ranking.RankingTypeMonsterKills)] = ranking.SnapshotPair{
					Leader:    ranking.RankingSnapshot{RankingType: ranking.RankingTypePvPVictory, SnapshotData: ""},
					Dependent: ranking.RankingSnapshot{RankingType: ranking.RankingTypeMonsterKills, SnapshotData: ""},
				}
				c.data[ranking.RankingTypeJobPopularity] = ranking.RankingSnapshot{RankingType: ranking.RankingTypeJobPopularity, SnapshotData: ""}
			},
		},
		{
			name: "corrupt json in valkey",
			setup: func(c *mockSnapshotCache) {
				c.pairs[mockPairKey(ranking.RankingTypePvPVictory, ranking.RankingTypeMonsterKills)] = ranking.SnapshotPair{
					Leader:    ranking.RankingSnapshot{RankingType: ranking.RankingTypePvPVictory, SnapshotData: "{corrupt"},
					Dependent: ranking.RankingSnapshot{RankingType: ranking.RankingTypeMonsterKills, SnapshotData: "{corrupt"},
				}
				c.data[ranking.RankingTypeJobPopularity] = ranking.RankingSnapshot{RankingType: ranking.RankingTypeJobPopularity, SnapshotData: "{corrupt"}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := newMockRepo()
			repo.snapshots[ranking.RankingTypePvPVictory] = sqlLeader
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
			repairedPair, found, err := cache.GetPair(ctx, ranking.RankingTypePvPVictory, ranking.RankingTypeMonsterKills)
			if err != nil || !found || repairedPair.Dependent.SnapshotData != sqlBoard.SnapshotData || repairedPair.Leader.SnapshotData != sqlLeader.SnapshotData {
				t.Fatalf("expected Valkey pair cache to be repaired from SQL, got found=%v err=%v pair=%+v", found, err, repairedPair)
			}
		})
	}
}
