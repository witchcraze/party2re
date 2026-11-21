package database

import (
	"context"
	"fmt"
	"testing"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/ranking"
)

func TestRankingRepository_Integration(t *testing.T) {
	db := OpenTestDB(t)

	rankingRepo, err := NewRankingRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	charRepo, err := NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	playerRepo, err := NewPlayerRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	now := time.Now().UTC()
	prefix := fmt.Sprintf("rank_%d_", time.Now().UnixNano())

	// 1. Create players
	p1, err := coreplayer.New(prefix+"p1", "password123", now)
	if err != nil {
		t.Fatal(err)
	}
	if err := playerRepo.Save(ctx, p1); err != nil {
		t.Fatal(err)
	}

	p2, err := coreplayer.New(prefix+"p2", "password123", now)
	if err != nil {
		t.Fatal(err)
	}
	if err := playerRepo.Save(ctx, p2); err != nil {
		t.Fatal(err)
	}

	// 2. Create characters with funds and deposits
	c1Name := fmt.Sprintf("r1_%05d", time.Now().UnixNano()%100000)
	c1, err := corecharacter.NewWithOptions(c1Name, "job-01", "m", nil)
	if err != nil {
		t.Fatal(err)
	}
	c1.PlayerID = p1.ID
	c1.Level = 80
	c1.Experience = 64000
	c1.Money = 50000
	c1.Deposit = 500000
	c1.SP = 3
	c1.SmallMedals = 15
	c1.HelpCount = 10
	if err := charRepo.Save(ctx, c1); err != nil {
		t.Fatal(err)
	}

	c2Name := fmt.Sprintf("r2_%05d", time.Now().UnixNano()%100000)
	c2, err := corecharacter.NewWithOptions(c2Name, "job-02", "f", nil)
	if err != nil {
		t.Fatal(err)
	}
	c2.PlayerID = p2.ID
	c2.Level = 90
	c2.Experience = 81000
	c2.Money = 300000
	c2.Deposit = 200000
	c2.SP = 1
	c2.SmallMedals = 50
	c2.HelpCount = 2
	if err := charRepo.Save(ctx, c2); err != nil {
		t.Fatal(err)
	}

	// 3. PvP Wins & Authentic Combat Counters
	_, err = db.ExecContext(ctx, `
		UPDATE characters SET pvp_wins = 25, monster_kills = 88, mao_count = 5, hero_count = 14 WHERE id = ?
	`, c1.ID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.ExecContext(ctx, `
		UPDATE characters SET pvp_wins = 5, monster_kills = 30, mao_count = 1, hero_count = 2 WHERE id = ?
	`, c2.ID)
	if err != nil {
		t.Fatal(err)
	}

	// === Test PvP Victory Ranking (kill_p / 王者ランキング) ===
	pvpRankings, total, err := rankingRepo.GetPvPVictoryRanking(ctx, 10000, 0)
	if err != nil {
		t.Fatalf("GetPvPVictoryRanking failed: %v", err)
	}
	if total < 2 {
		t.Fatalf("expected total >= 2, got %d", total)
	}
	var c1Rank, c2Rank int
	for _, entry := range pvpRankings {
		if entry.CharacterID == c1.ID {
			c1Rank = entry.Rank
			if entry.Score != 25 {
				t.Fatalf("expected c1 pvp_wins 25, got %d", entry.Score)
			}
		}
		if entry.CharacterID == c2.ID {
			c2Rank = entry.Rank
			if entry.Score != 5 {
				t.Fatalf("expected c2 pvp_wins 5, got %d", entry.Score)
			}
		}
	}
	if c1Rank == 0 || c2Rank == 0 || c1Rank >= c2Rank {
		t.Fatalf("expected c1 rank (%d) < c2 rank (%d)", c1Rank, c2Rank)
	}

	// === Test Monster Kills Ranking (kill_m / 英雄ランキング) ===
	mkRankings, _, err := rankingRepo.GetMonsterKillsRanking(ctx, 10000, 0)
	if err != nil {
		t.Fatalf("GetMonsterKillsRanking failed: %v", err)
	}
	for _, entry := range mkRankings {
		if entry.CharacterID == c1.ID && entry.Score != 88 {
			t.Fatalf("expected c1 monster kills 88, got %d", entry.Score)
		}
		if entry.CharacterID == c2.ID && entry.Score != 30 {
			t.Fatalf("expected c2 monster kills 30, got %d", entry.Score)
		}
	}

	// === Test Mao Count Ranking (mao_c / 魔王ランキング) ===
	maoRankings, _, err := rankingRepo.GetMaoCountRanking(ctx, 10000, 0)
	if err != nil {
		t.Fatalf("GetMaoCountRanking failed: %v", err)
	}
	for _, entry := range maoRankings {
		if entry.CharacterID == c1.ID && entry.Score != 5 {
			t.Fatalf("expected c1 mao count 5, got %d", entry.Score)
		}
		if entry.CharacterID == c2.ID && entry.Score != 1 {
			t.Fatalf("expected c2 mao count 1, got %d", entry.Score)
		}
	}

	// === Test Hero Count Ranking (hero_c / 勇者ランキング) ===
	heroRankings, _, err := rankingRepo.GetHeroCountRanking(ctx, 10000, 0)
	if err != nil {
		t.Fatalf("GetHeroCountRanking failed: %v", err)
	}
	for _, entry := range heroRankings {
		if entry.CharacterID == c1.ID && entry.Score != 14 {
			t.Fatalf("expected c1 hero count 14, got %d", entry.Score)
		}
		if entry.CharacterID == c2.ID && entry.Score != 2 {
			t.Fatalf("expected c2 hero count 2, got %d", entry.Score)
		}
	}

	// === Test Job Popularity Ranking ===
	popJobA := fmt.Sprintf("%s_job_a", prefix)
	popJobB := fmt.Sprintf("%s_job_b", prefix)
	if err := rankingRepo.RecordJobPopularity(ctx, popJobA, "m", 10); err != nil {
		t.Fatalf("RecordJobPopularity male failed: %v", err)
	}
	if err := rankingRepo.RecordJobPopularity(ctx, popJobA, "f", 10); err != nil {
		t.Fatalf("RecordJobPopularity female failed: %v", err)
	}
	if err := rankingRepo.RecordJobPopularity(ctx, popJobB, "f", 30); err != nil {
		t.Fatalf("RecordJobPopularity popJobB failed: %v", err)
	}

	popRankings, err := rankingRepo.GetJobPopularityRanking(ctx)
	if err != nil {
		t.Fatalf("GetJobPopularityRanking failed: %v", err)
	}
	if len(popRankings) < 2 {
		t.Fatalf("expected at least 2 job popularity entries, got %d", len(popRankings))
	}
	var foundA, foundB *ranking.JobPopularityEntry
	for i := range popRankings {
		if popRankings[i].JobID == popJobA {
			foundA = &popRankings[i]
		}
		if popRankings[i].JobID == popJobB {
			foundB = &popRankings[i]
		}
	}
	if foundA == nil || foundB == nil {
		t.Fatalf("expected to find both %s and %s in rankings", popJobA, popJobB)
	}
	if foundA.TotalCount != 20 || foundA.MaleCount != 10 || foundA.FemaleCount != 10 {
		t.Errorf("jobA: expected total 20, male 10, female 10; got total %d, male %d, female %d", foundA.TotalCount, foundA.MaleCount, foundA.FemaleCount)
	}
	if foundB.TotalCount != 30 || foundB.MaleCount != 0 || foundB.FemaleCount != 30 {
		t.Errorf("jobB: expected total 30, male 0, female 30; got total %d, male %d, female %d", foundB.TotalCount, foundB.MaleCount, foundB.FemaleCount)
	}
	if foundB.Rank >= foundA.Rank {
		t.Errorf("expected jobB (30 pts) to outrank jobA (20 pts): rankB=%d, rankA=%d", foundB.Rank, foundA.Rank)
	}

	// === Test Helper Ranking ===
	helpRankings, _, err := rankingRepo.GetHelperRanking(ctx, 10000, 0)
	if err != nil {
		t.Fatalf("GetHelperRanking failed: %v", err)
	}
	for _, entry := range helpRankings {
		if entry.CharacterID == c1.ID && entry.Score != 10 {
			t.Fatalf("expected c1 help count 10, got %d", entry.Score)
		}
	}

	// === Test Snapshots ===
	snap := ranking.RankingSnapshot{
		RankingType:  ranking.RankingTypeMonsterKills,
		SnapshotData: `[{"rank":1,"character_id":"test"}]`,
		TotalCount:   1,
		CalculatedAt: now,
		UpdatedAt:    now,
	}
	if err := rankingRepo.SaveSnapshot(ctx, snap); err != nil {
		t.Fatalf("SaveSnapshot failed: %v", err)
	}

	gotSnap, err := rankingRepo.GetSnapshot(ctx, ranking.RankingTypeMonsterKills)
	if err != nil {
		t.Fatalf("GetSnapshot failed: %v", err)
	}
	if gotSnap.SnapshotData != snap.SnapshotData || gotSnap.TotalCount != 1 {
		t.Fatalf("snapshot mismatch: %+v", gotSnap)
	}

	allSnaps, err := rankingRepo.GetAllSnapshots(ctx)
	if err != nil {
		t.Fatalf("GetAllSnapshots failed: %v", err)
	}
	if _, ok := allSnaps[ranking.RankingTypeMonsterKills]; !ok {
		t.Fatalf("expected monster_kills snapshot in all snapshots")
	}

	_, err = rankingRepo.GetSnapshot(ctx, ranking.RankingType("non_existent"))
	if err != ranking.ErrSnapshotNotFound {
		t.Fatalf("expected ErrSnapshotNotFound, got %v", err)
	}
}
