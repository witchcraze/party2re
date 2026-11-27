package ranking_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/ranking"
)

func rankedEntries(ids ...string) []ranking.CharacterRankingEntry {
	entries := make([]ranking.CharacterRankingEntry, len(ids))
	for i, id := range ids {
		entries[i] = ranking.CharacterRankingEntry{Rank: i + 1, CharacterID: id, Score: int64(100 - i)}
	}
	return entries
}

func seqIDs(prefix string, n int) []string {
	ids := make([]string, n)
	for i := range ids {
		ids[i] = fmt.Sprintf("%s%02d", prefix, i+1)
	}
	return ids
}

func boardIDs(t *testing.T, entries []ranking.CharacterRankingEntry, firstRank int) []string {
	t.Helper()
	ids := make([]string, len(entries))
	for i, e := range entries {
		if e.Rank != firstRank+i {
			t.Fatalf("entry %d (%s) rank = %d, want %d", i, e.CharacterID, e.Rank, firstRank+i)
		}
		ids[i] = e.CharacterID
	}
	return ids
}

// Legacy login.cgi write_ranking: kill_m skips the kill_p top 10 and hero_c skips the mao_c top 10.
func TestBoards_ExcludeLeadersOfPairedBoard(t *testing.T) {
	leaders := seqIDs("L", 12) // L11 and L12 are outside the leaders' top 10.
	// Candidates interleave every leader with ordinary characters N01..N12.
	var candidates []string
	for i := 0; i < 12; i++ {
		candidates = append(candidates, leaders[i], fmt.Sprintf("N%02d", i+1))
	}
	want := []string{"N01", "N02", "N03", "N04", "N05", "N06", "N07", "N08", "N09", "N10"}

	cases := []struct {
		name  string
		setup func(m *mockRankingRepository)
		get   func(*ranking.Service, context.Context, int, int) (ranking.RankingPage[ranking.CharacterRankingEntry], error)
	}{
		{"monster_kills excludes pvp_victory top 10", func(m *mockRankingRepository) {
			m.pvpRankings, m.monsterKillsRankings = rankedEntries(leaders...), rankedEntries(candidates...)
		}, (*ranking.Service).GetMonsterKillsRanking},
		{"hero_count excludes mao_count top 10", func(m *mockRankingRepository) {
			m.maoCountRankings, m.heroCountRankings = rankedEntries(leaders...), rankedEntries(candidates...)
		}, (*ranking.Service).GetHeroCountRanking},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := newMockRepo()
			tc.setup(repo)
			svc, err := ranking.NewService(repo)
			if err != nil {
				t.Fatal(err)
			}
			ctx := context.Background()
			// Before publication, public reads do not fall back to live queries.
			unpublished, err := tc.get(svc, ctx, 20, 0)
			if err != nil || unpublished.Total != 0 || len(unpublished.Entries) != 0 {
				t.Fatalf("unpublished = %+v, %v; want empty", unpublished, err)
			}
			if err := svc.RefreshAllSnapshots(ctx); err != nil {
				t.Fatal(err)
			}
			page, err := tc.get(svc, ctx, 20, 0)
			if err != nil || !page.IsSnapshot {
				t.Fatalf("published = %+v, %v", page, err)
			}
			if got := boardIDs(t, page.Entries, 1); fmt.Sprint(got) != fmt.Sprint(want) {
				t.Fatalf("board = %v, want %v", got, want)
			}
			if page.Total != 10 {
				t.Fatalf("total = %d, want 10", page.Total)
			}
			paged, err := tc.get(svc, ctx, 2, 8)
			if err != nil || !paged.IsSnapshot {
				t.Fatal(err)
			}
			if got := boardIDs(t, paged.Entries, 9); fmt.Sprint(got) != "[N09 N10]" {
				t.Fatalf("page 9-10 = %v", got)
			}
		})
	}
}

// With ten or fewer characters every one is a kill_p leader, so legacy kill_m is empty.
func TestBoards_SmallServerLeavesExcludedBoardEmpty(t *testing.T) {
	repo := newMockRepo()
	ids := seqIDs("C", 4)
	repo.pvpRankings, repo.monsterKillsRankings = rankedEntries(ids...), rankedEntries(ids...)
	svc, err := ranking.NewService(repo)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := svc.RefreshAllSnapshots(ctx); err != nil {
		t.Fatal(err)
	}
	page, err := svc.GetMonsterKillsRanking(ctx, 20, 0)
	if err != nil || page.Total != 0 || page.Entries == nil || len(page.Entries) != 0 {
		t.Fatalf("page = %+v, %v; want empty non-nil board", page, err)
	}
}

func TestBoards_CapAtTen(t *testing.T) {
	repo := newMockRepo()
	repo.casinoWinsRankings = rankedEntries(seqIDs("C", 15)...)
	svc, err := ranking.NewService(repo)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := svc.RefreshAllSnapshots(ctx); err != nil {
		t.Fatal(err)
	}
	page, err := svc.GetCasinoWinsRanking(ctx, 100, 0)
	if err != nil || page.Total != 10 || len(page.Entries) != 10 {
		t.Fatalf("page = %+v, %v; want 10 entries", page, err)
	}
	beyond, err := svc.GetCasinoWinsRanking(ctx, 5, 10)
	if err != nil || len(beyond.Entries) != 0 || beyond.Total != 10 {
		t.Fatalf("beyond board = %+v, %v", beyond, err)
	}
}

func TestRefreshSnapshot_KeepsFinalizedWeeklySnapshot(t *testing.T) {
	repo := newMockRepo()
	finalized := ranking.RankingSnapshot{RankingType: ranking.RankingTypeWeeklyJobChange, SnapshotData: `[{"rank":1,"character_id":"w1","score":3}]`, TotalCount: 1}
	repo.snapshots[ranking.RankingTypeWeeklyJobChange] = finalized
	repo.weeklyJobChangeRankings = nil // the active week was reset by rotation
	svc, err := ranking.NewService(repo)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.RefreshSnapshot(context.Background(), ranking.RankingTypeWeeklyJobChange); err != nil {
		t.Fatal(err)
	}
	if got := repo.snapshots[ranking.RankingTypeWeeklyJobChange]; got != finalized {
		t.Fatalf("weekly snapshot overwritten: %+v", got)
	}
}

type pairedBoardCase struct {
	name          string
	leaderType    ranking.RankingType
	dependentType ranking.RankingType
	setLeaders    func(*mockRankingRepository, []ranking.CharacterRankingEntry)
	setDependent  func(*mockRankingRepository, []ranking.CharacterRankingEntry)
	getLeader     func(*ranking.Service, context.Context, int, int) (ranking.RankingPage[ranking.CharacterRankingEntry], error)
	getDependent  func(*ranking.Service, context.Context, int, int) (ranking.RankingPage[ranking.CharacterRankingEntry], error)
}

var pairedBoardCases = []pairedBoardCase{
	{
		name:          "pvp_victory_and_monster_kills",
		leaderType:    ranking.RankingTypePvPVictory,
		dependentType: ranking.RankingTypeMonsterKills,
		setLeaders: func(m *mockRankingRepository, e []ranking.CharacterRankingEntry) {
			m.pvpRankings, m.pvpTotal = e, len(e)
		},
		setDependent: func(m *mockRankingRepository, e []ranking.CharacterRankingEntry) {
			m.monsterKillsRankings, m.monsterKillsTotal = e, len(e)
		},
		getLeader:    (*ranking.Service).GetPvPVictoryRanking,
		getDependent: (*ranking.Service).GetMonsterKillsRanking,
	},
	{
		name:          "mao_count_and_hero_count",
		leaderType:    ranking.RankingTypeMaoCount,
		dependentType: ranking.RankingTypeHeroCount,
		setLeaders: func(m *mockRankingRepository, e []ranking.CharacterRankingEntry) {
			m.maoCountRankings, m.maoCountTotal = e, len(e)
		},
		setDependent: func(m *mockRankingRepository, e []ranking.CharacterRankingEntry) {
			m.heroCountRankings, m.heroCountTotal = e, len(e)
		},
		getLeader:    (*ranking.Service).GetMaoCountRanking,
		getDependent: (*ranking.Service).GetHeroCountRanking,
	},
}

type interleavedPairRepo struct {
	*mockRankingRepository
	mu              sync.Mutex
	targetLeader    ranking.RankingType
	onLeaderRead    func()
	leaderReadCount int
	failSaveType    ranking.RankingType
	failSaveErr     error
	saveOrder       []ranking.RankingType
}

func (r *interleavedPairRepo) GetPvPVictoryRanking(ctx context.Context, limit, offset int) ([]ranking.CharacterRankingEntry, int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	entries, total, err := r.mockRankingRepository.GetPvPVictoryRanking(ctx, limit, offset)
	if r.targetLeader == ranking.RankingTypePvPVictory {
		r.leaderReadCount++
		if r.leaderReadCount == 1 && r.onLeaderRead != nil {
			r.onLeaderRead()
		}
	}
	return entries, total, err
}

func (r *interleavedPairRepo) GetMaoCountRanking(ctx context.Context, limit, offset int) ([]ranking.CharacterRankingEntry, int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	entries, total, err := r.mockRankingRepository.GetMaoCountRanking(ctx, limit, offset)
	if r.targetLeader == ranking.RankingTypeMaoCount {
		r.leaderReadCount++
		if r.leaderReadCount == 1 && r.onLeaderRead != nil {
			r.onLeaderRead()
		}
	}
	return entries, total, err
}

func (r *interleavedPairRepo) GetMonsterKillsRanking(ctx context.Context, limit, offset int) ([]ranking.CharacterRankingEntry, int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.mockRankingRepository.GetMonsterKillsRanking(ctx, limit, offset)
}

func (r *interleavedPairRepo) GetHeroCountRanking(ctx context.Context, limit, offset int) ([]ranking.CharacterRankingEntry, int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.mockRankingRepository.GetHeroCountRanking(ctx, limit, offset)
}

func (r *interleavedPairRepo) SaveSnapshot(ctx context.Context, snapshot ranking.RankingSnapshot) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.saveOrder = append(r.saveOrder, snapshot.RankingType)
	if r.failSaveErr != nil && snapshot.RankingType == r.failSaveType {
		return r.failSaveErr
	}
	return r.mockRankingRepository.SaveSnapshot(ctx, snapshot)
}

func assertDisjointPair(t *testing.T, leaderPage, dependentPage ranking.RankingPage[ranking.CharacterRankingEntry]) {
	t.Helper()
	if !leaderPage.IsSnapshot || !dependentPage.IsSnapshot {
		t.Fatalf("expected snapshot pages, got leader=%v dependent=%v", leaderPage.IsSnapshot, dependentPage.IsSnapshot)
	}
	if len(leaderPage.Entries) > 10 || len(dependentPage.Entries) > 10 {
		t.Fatalf("board exceeded 10 entries: leader=%d dependent=%d", len(leaderPage.Entries), len(dependentPage.Entries))
	}
	seen := make(map[string]bool, len(leaderPage.Entries))
	for i, e := range leaderPage.Entries {
		if e.Rank != i+1 {
			t.Fatalf("leader rank[%d] = %d, want %d", i, e.Rank, i+1)
		}
		seen[e.CharacterID] = true
	}
	for i, e := range dependentPage.Entries {
		if e.Rank != i+1 {
			t.Fatalf("dependent rank[%d] = %d, want %d", i, e.Rank, i+1)
		}
		if seen[e.CharacterID] {
			t.Fatalf("character %s appeared in both published leader and dependent boards", e.CharacterID)
		}
	}
}

func TestPairedBoards_DeriveExclusionFromPublishedLeadersOnInterleaving(t *testing.T) {
	initialLeaders := []string{"Alice", "L02", "L03", "L04", "L05", "L06", "L07", "L08", "L09", "L10"}
	mutatedLeaders := []string{"Bob", "L02", "L03", "L04", "L05", "L06", "L07", "L08", "L09", "L10"}
	candidates := append([]string{"Alice", "Bob"}, seqIDs("M", 10)...)

	for _, tc := range pairedBoardCases {
		for _, mode := range []struct {
			name    string
			refresh func(*ranking.Service, context.Context) error
		}{
			{"RefreshAllSnapshots", (*ranking.Service).RefreshAllSnapshots},
			{"RefreshSnapshot(leader)", func(s *ranking.Service, ctx context.Context) error { return s.RefreshSnapshot(ctx, tc.leaderType) }},
			{"RefreshSnapshot(dependent)", func(s *ranking.Service, ctx context.Context) error { return s.RefreshSnapshot(ctx, tc.dependentType) }},
		} {
			t.Run(tc.name+"/"+mode.name, func(t *testing.T) {
				base := newMockRepo()
				tc.setLeaders(base, rankedEntries(initialLeaders...))
				tc.setDependent(base, rankedEntries(candidates...))
				repo := &interleavedPairRepo{
					mockRankingRepository: base,
					targetLeader:          tc.leaderType,
					onLeaderRead: func() {
						// Mutate leaders right after the first leader query so a second query would return Bob instead of Alice.
						tc.setLeaders(base, rankedEntries(mutatedLeaders...))
					},
				}
				svc, err := ranking.NewService(repo)
				if err != nil {
					t.Fatal(err)
				}
				ctx := context.Background()
				if err := mode.refresh(svc, ctx); err != nil {
					t.Fatalf("refresh failed: %v", err)
				}
				if repo.leaderReadCount != 1 {
					t.Fatalf("expected exactly 1 leader query per pair publication, got %d", repo.leaderReadCount)
				}

				leaderPage, err := tc.getLeader(svc, ctx, 20, 0)
				if err != nil {
					t.Fatal(err)
				}
				depPage, err := tc.getDependent(svc, ctx, 20, 0)
				if err != nil {
					t.Fatal(err)
				}
				assertDisjointPair(t, leaderPage, depPage)

				if got := boardIDs(t, leaderPage.Entries, 1); fmt.Sprint(got) != fmt.Sprint(initialLeaders) {
					t.Fatalf("leader board = %v, want %v", got, initialLeaders)
				}
				wantDep := []string{"Bob", "M01", "M02", "M03", "M04", "M05", "M06", "M07", "M08", "M09"}
				if got := boardIDs(t, depPage.Entries, 1); fmt.Sprint(got) != fmt.Sprint(wantDep) {
					t.Fatalf("dependent board = %v, want %v", got, wantDep)
				}
				if depPage.Total != 10 {
					t.Fatalf("dependent total = %d, want 10", depPage.Total)
				}
			})
		}
	}
}

func TestPairedBoards_DependentSaveFailurePreservesConsistentSQLAndCache(t *testing.T) {
	saveErr := errors.New("disk full saving dependent snapshot")
	v1Leaders := seqIDs("A", 10)
	v1Candidates := append([]string{"A01"}, seqIDs("B", 10)...)
	v2Leaders := seqIDs("C", 10)
	v2Candidates := append([]string{"C01"}, seqIDs("D", 10)...)

	for _, tc := range pairedBoardCases {
		for _, mode := range []struct {
			name    string
			refresh func(*ranking.Service, context.Context) error
		}{
			{"RefreshAllSnapshots", (*ranking.Service).RefreshAllSnapshots},
			{"RefreshSnapshot(leader)", func(s *ranking.Service, ctx context.Context) error { return s.RefreshSnapshot(ctx, tc.leaderType) }},
			{"RefreshSnapshot(dependent)", func(s *ranking.Service, ctx context.Context) error { return s.RefreshSnapshot(ctx, tc.dependentType) }},
		} {
			t.Run(tc.name+"/"+mode.name, func(t *testing.T) {
				base := newMockRepo()
				tc.setLeaders(base, rankedEntries(v1Leaders...))
				tc.setDependent(base, rankedEntries(v1Candidates...))
				repo := &interleavedPairRepo{mockRankingRepository: base}
				cache := newMockSnapshotCache()
				now := time.Date(2026, 11, 27, 1, 0, 0, 0, time.UTC)

				svc, err := ranking.NewService(
					repo,
					ranking.WithSnapshotCache(cache),
					ranking.WithNowFunc(func() time.Time { return now }),
				)
				if err != nil {
					t.Fatal(err)
				}
				ctx := context.Background()
				if err := svc.RefreshAllSnapshots(ctx); err != nil {
					t.Fatalf("initial RefreshAllSnapshots: %v", err)
				}

				// Update underlying rankings to V2 and fail saving the dependent board.
				now = now.Add(time.Minute)
				tc.setLeaders(base, rankedEntries(v2Leaders...))
				tc.setDependent(base, rankedEntries(v2Candidates...))
				repo.failSaveType = tc.dependentType
				repo.failSaveErr = saveErr

				if err := mode.refresh(svc, ctx); !errors.Is(err, saveErr) {
					t.Fatalf("expected %v, got %v", saveErr, err)
				}

				// SQL snapshots must both still reflect V1.
				sqlLeaderSnap, err := base.GetSnapshot(ctx, tc.leaderType)
				if err != nil {
					t.Fatal(err)
				}
				sqlDepSnap, err := base.GetSnapshot(ctx, tc.dependentType)
				if err != nil {
					t.Fatal(err)
				}
				var sqlLeaderEntries, sqlDepEntries []ranking.CharacterRankingEntry
				if err := json.Unmarshal([]byte(sqlLeaderSnap.SnapshotData), &sqlLeaderEntries); err != nil {
					t.Fatal(err)
				}
				if err := json.Unmarshal([]byte(sqlDepSnap.SnapshotData), &sqlDepEntries); err != nil {
					t.Fatal(err)
				}
				if got := boardIDs(t, sqlLeaderEntries, 1); fmt.Sprint(got) != fmt.Sprint(v1Leaders) {
					t.Fatalf("SQL leader rolled back check: got %v, want %v", got, v1Leaders)
				}
				wantV1Dep := seqIDs("B", 10)
				if got := boardIDs(t, sqlDepEntries, 1); fmt.Sprint(got) != fmt.Sprint(wantV1Dep) {
					t.Fatalf("SQL dependent check: got %v, want %v", got, wantV1Dep)
				}

				// Valkey cache must also still reflect V1.
				cacheLeader, foundL, _ := cache.Get(ctx, tc.leaderType)
				cacheDep, foundD, _ := cache.Get(ctx, tc.dependentType)
				if !foundL || !foundD || cacheLeader.SnapshotData != sqlLeaderSnap.SnapshotData || cacheDep.SnapshotData != sqlDepSnap.SnapshotData {
					t.Fatalf("Valkey cache diverged from SQL after rollback: leader=(%v,%q) dep=(%v,%q)", foundL, cacheLeader.SnapshotData, foundD, cacheDep.SnapshotData)
				}

				// Service reads must still return V1 for both boards.
				lPage, err := tc.getLeader(svc, ctx, 20, 0)
				if err != nil {
					t.Fatal(err)
				}
				dPage, err := tc.getDependent(svc, ctx, 20, 0)
				if err != nil {
					t.Fatal(err)
				}
				if got := boardIDs(t, lPage.Entries, 1); fmt.Sprint(got) != fmt.Sprint(v1Leaders) {
					t.Fatalf("service leader = %v, want %v", got, v1Leaders)
				}
				if got := boardIDs(t, dPage.Entries, 1); fmt.Sprint(got) != fmt.Sprint(wantV1Dep) {
					t.Fatalf("service dependent = %v, want %v", got, wantV1Dep)
				}
			})
		}
	}
}

func TestPairedBoards_ConcurrentScheduledAndManualRefreshConsistency(t *testing.T) {
	ctx := context.Background()
	base := newMockRepo()
	repo := &interleavedPairRepo{mockRankingRepository: base}
	cache := newMockSnapshotCache()
	now := time.Date(2026, 11, 27, 0, 0, 1, 0, time.UTC)

	setPop := func(gen int) {
		repo.mu.Lock()
		defer repo.mu.Unlock()
		prefix := fmt.Sprintf("G%02d_", gen)
		leaders := seqIDs(prefix+"L", 10)
		candidates := append([]string{leaders[0], leaders[1]}, seqIDs(prefix+"M", 10)...)
		base.pvpRankings, base.pvpTotal = rankedEntries(leaders...), 10
		base.monsterKillsRankings, base.monsterKillsTotal = rankedEntries(candidates...), len(candidates)
		base.maoCountRankings, base.maoCountTotal = rankedEntries(leaders...), 10
		base.heroCountRankings, base.heroCountTotal = rankedEntries(candidates...), len(candidates)
	}
	setPop(0)

	svc, err := ranking.NewService(
		repo,
		ranking.WithSnapshotCache(cache),
		ranking.WithNowFunc(func() time.Time { return now }),
	)
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	errs := make(chan error, 12)
	for g := 0; g < 6; g++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < 8; i++ {
				setPop(id*10 + i)
				var err error
				switch i % 3 {
				case 0:
					err = svc.RefreshAllSnapshots(ctx)
				case 1:
					err = svc.RefreshSnapshot(ctx, ranking.RankingTypePvPVictory)
				case 2:
					err = svc.RefreshSnapshot(ctx, ranking.RankingTypeHeroCount)
				}
				if err != nil {
					errs <- err
					return
				}
			}
		}(g)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("concurrent refresh error: %v", err)
	}

	for _, tc := range pairedBoardCases {
		lPage, err := tc.getLeader(svc, ctx, 20, 0)
		if err != nil {
			t.Fatal(err)
		}
		dPage, err := tc.getDependent(svc, ctx, 20, 0)
		if err != nil {
			t.Fatal(err)
		}
		assertDisjointPair(t, lPage, dPage)

		sqlL, err := base.GetSnapshot(ctx, tc.leaderType)
		if err != nil {
			t.Fatal(err)
		}
		sqlD, err := base.GetSnapshot(ctx, tc.dependentType)
		if err != nil {
			t.Fatal(err)
		}
		cacheL, okL, _ := cache.Get(ctx, tc.leaderType)
		cacheD, okD, _ := cache.Get(ctx, tc.dependentType)
		if !okL || !okD || cacheL.SnapshotData != sqlL.SnapshotData || cacheD.SnapshotData != sqlD.SnapshotData {
			t.Fatalf("%s: cache and SQL diverged after concurrent refreshes", tc.name)
		}
	}
}
