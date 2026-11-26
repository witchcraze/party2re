package ranking_test

import (
	"context"
	"fmt"
	"testing"

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
		get   func(*ranking.Service, context.Context, int, int, bool) (ranking.RankingPage[ranking.CharacterRankingEntry], error)
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
			page, err := tc.get(svc, ctx, 20, 0, false)
			if err != nil {
				t.Fatal(err)
			}
			if got := boardIDs(t, page.Entries, 1); fmt.Sprint(got) != fmt.Sprint(want) {
				t.Fatalf("board = %v, want %v", got, want)
			}
			if page.Total != 10 {
				t.Fatalf("total = %d, want 10", page.Total)
			}
			// Pagination and snapshots page the same excluded board.
			paged, err := tc.get(svc, ctx, 2, 8, false)
			if err != nil {
				t.Fatal(err)
			}
			if got := boardIDs(t, paged.Entries, 9); fmt.Sprint(got) != "[N09 N10]" {
				t.Fatalf("page 9-10 = %v", got)
			}
			if err := svc.RefreshAllSnapshots(ctx); err != nil {
				t.Fatal(err)
			}
			cached, err := tc.get(svc, ctx, 20, 0, true)
			if err != nil || !cached.IsSnapshot || cached.Total != 10 {
				t.Fatalf("snapshot page = %+v, %v", cached, err)
			}
			if got := boardIDs(t, cached.Entries, 1); fmt.Sprint(got) != fmt.Sprint(want) {
				t.Fatalf("snapshot board = %v, want %v", got, want)
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
	page, err := svc.GetMonsterKillsRanking(context.Background(), 20, 0, false)
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
	page, err := svc.GetCasinoWinsRanking(context.Background(), 100, 0, false)
	if err != nil || page.Total != 10 || len(page.Entries) != 10 {
		t.Fatalf("page = %+v, %v; want 10 entries", page, err)
	}
	beyond, err := svc.GetCasinoWinsRanking(context.Background(), 5, 10, false)
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
