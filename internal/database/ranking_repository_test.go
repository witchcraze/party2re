package database

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/boss"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	corejob "github.com/witchcraze/party2re/internal/core/job"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/pvp"
	"github.com/witchcraze/party2re/internal/ranking"
)

func TestRankingRepository_Integration(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

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
	jobRepo, err := NewCharacterJobRepository(db)
	if err != nil {
		t.Fatal(err)
	}
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

	// 2. Set bank balances
	_, err = db.ExecContext(ctx, `
		INSERT INTO bank_accounts (player_id, balance)
		VALUES (?, 500000), (?, 200000)
		ON DUPLICATE KEY UPDATE balance = VALUES(balance)
	`, p1.ID, p2.ID)
	if err != nil {
		t.Fatal(err)
	}

	// 3. Create characters
	c1, err := corecharacter.NewWithOptions(prefix+"Hero", "warrior", "m", nil)
	if err != nil {
		t.Fatal(err)
	}
	c1.PlayerID = p1.ID
	c1.Level = 80
	c1.Experience = 64000
	c1.Money = 50000
	c1.RebirthCount = 3
	c1.SmallMedals = 15
	c1.HelpCount = 10
	if err := charRepo.Save(ctx, c1); err != nil {
		t.Fatal(err)
	}

	c2, err := corecharacter.NewWithOptions(prefix+"Mage", "mage", "f", nil)
	if err != nil {
		t.Fatal(err)
	}
	c2.PlayerID = p2.ID
	c2.Level = 90
	c2.Experience = 81000
	c2.Money = 300000
	c2.RebirthCount = 1
	c2.SmallMedals = 50
	c2.HelpCount = 2
	if err := charRepo.Save(ctx, c2); err != nil {
		t.Fatal(err)
	}

	// 4. Job Masteries
	if err := jobRepo.Save(ctx, corejob.CharacterJob{
		CharacterID:  c1.ID,
		CurrentJobID: "warrior",
		MasteredJobs: []string{"warrior", "knight", "paladin"},
	}); err != nil {
		t.Fatal(err)
	}

	if err := jobRepo.Save(ctx, corejob.CharacterJob{
		CharacterID:  c2.ID,
		CurrentJobID: "mage",
		MasteredJobs: []string{"mage"},
	}); err != nil {
		t.Fatal(err)
	}

	// 5. PvP Wins
	_, err = db.ExecContext(ctx, `
		INSERT INTO arena_ratings (character_id, rating, wins, losses, draws)
		VALUES (?, 1400, 25, 2, 0), (?, 1100, 5, 10, 0)
		ON DUPLICATE KEY UPDATE wins = VALUES(wins), rating = VALUES(rating)
	`, c1.ID, c2.ID)
	if err != nil {
		t.Fatal(err)
	}

	// 6. Boss Defeats
	_, err = db.ExecContext(ctx, `
		INSERT INTO character_boss_records (character_id, highest_tier_cleared, total_boss_defeats, daily_attempts_reset_at)
		VALUES (?, 5, 12, ?), (?, 2, 3, ?)
		ON DUPLICATE KEY UPDATE total_boss_defeats = VALUES(total_boss_defeats)
	`, c1.ID, now, c2.ID, now)
	if err != nil {
		t.Fatal(err)
	}

	// 7. Adventure Wins
	_, err = db.ExecContext(ctx, `
		INSERT INTO adventures (id, character_id, adventure_type, started_at, available_at, experience_reward, outcome, resolved, claimed)
		VALUES (?, ?, 'stage_1', ?, ?, 100, 'WIN', TRUE, TRUE),
		       (?, ?, 'stage_2', ?, ?, 200, 'WIN', TRUE, TRUE)
	`, prefix+"adv1", c1.ID, now, now, prefix+"adv2", c1.ID, now, now)
	if err != nil {
		t.Fatal(err)
	}

	// === Test Level Ranking ===
	levelRankings, total, err := rankingRepo.GetLevelRanking(ctx, 100, 0)
	if err != nil {
		t.Fatalf("GetLevelRanking failed: %v", err)
	}
	if total < 2 {
		t.Fatalf("expected total >= 2, got %d", total)
	}
	// Verify c2 (Lv90) ranks above c1 (Lv80)
	var c1Rank, c2Rank int
	for _, entry := range levelRankings {
		if entry.CharacterID == c1.ID {
			c1Rank = entry.Rank
		}
		if entry.CharacterID == c2.ID {
			c2Rank = entry.Rank
		}
	}
	if c2Rank == 0 || c1Rank == 0 || c2Rank >= c1Rank {
		t.Fatalf("expected c2 (Lv 90) rank (%d) < c1 (Lv 80) rank (%d)", c2Rank, c1Rank)
	}

	// === Test Player Wealth Ranking ===
	wealthRankings, pTotal, err := rankingRepo.GetPlayerWealthRanking(ctx, 100, 0)
	if err != nil {
		t.Fatalf("GetPlayerWealthRanking failed: %v", err)
	}
	if pTotal < 2 {
		t.Fatalf("expected pTotal >= 2, got %d", pTotal)
	}
	var p1Wealth, p2Wealth int64
	for _, entry := range wealthRankings {
		if entry.PlayerID == p1.ID {
			p1Wealth = entry.TotalWealth
		}
		if entry.PlayerID == p2.ID {
			p2Wealth = entry.TotalWealth
		}
	}
	// p1 = 500,000 bank + 50,000 char = 550,000
	// p2 = 200,000 bank + 300,000 char = 500,000
	if p1Wealth != 550000 || p2Wealth != 500000 {
		t.Fatalf("expected p1Wealth=550000, p2Wealth=500000, got p1=%d, p2=%d", p1Wealth, p2Wealth)
	}

	// === Test Character Wealth Ranking ===
	charWealthRankings, _, err := rankingRepo.GetCharacterWealthRanking(ctx, 100, 0)
	if err != nil {
		t.Fatalf("GetCharacterWealthRanking failed: %v", err)
	}
	for _, entry := range charWealthRankings {
		if entry.CharacterID == c2.ID && entry.Score != 300000 {
			t.Fatalf("expected c2 held gold 300000, got %d", entry.Score)
		}
	}

	// === Test Battle Victory Ranking ===
	battleRankings, _, err := rankingRepo.GetBattleVictoryRanking(ctx, 100, 0)
	if err != nil {
		t.Fatalf("GetBattleVictoryRanking failed: %v", err)
	}
	for _, entry := range battleRankings {
		if entry.CharacterID == c1.ID {
			// c1: 25 pvp + 12 boss + 2 adv = 39
			if entry.Score != 39 {
				t.Fatalf("expected c1 total victories 39, got %d", entry.Score)
			}
		}
	}

	// === Test Job Mastery Ranking ===
	masteryRankings, _, err := rankingRepo.GetJobMasteryRanking(ctx, 100, 0)
	if err != nil {
		t.Fatalf("GetJobMasteryRanking failed: %v", err)
	}
	for _, entry := range masteryRankings {
		if entry.CharacterID == c1.ID && entry.Score != 3 {
			t.Fatalf("expected c1 mastered count 3, got %d", entry.Score)
		}
		if entry.CharacterID == c2.ID && entry.Score != 1 {
			t.Fatalf("expected c2 mastered count 1, got %d", entry.Score)
		}
	}

	// === Test Job Popularity Ranking ===
	popRankings, err := rankingRepo.GetJobPopularityRanking(ctx)
	if err != nil {
		t.Fatalf("GetJobPopularityRanking failed: %v", err)
	}
	if len(popRankings) == 0 {
		t.Fatalf("expected job popularity entries")
	}

	// === Test Helper Ranking ===
	helpRankings, _, err := rankingRepo.GetHelperRanking(ctx, 100, 0)
	if err != nil {
		t.Fatalf("GetHelperRanking failed: %v", err)
	}
	for _, entry := range helpRankings {
		if entry.CharacterID == c1.ID && entry.Score != 10 {
			t.Fatalf("expected c1 help count 10, got %d", entry.Score)
		}
	}

	// === Test Rebirth Ranking ===
	rebirthRankings, _, err := rankingRepo.GetRebirthRanking(ctx, 100, 0)
	if err != nil {
		t.Fatalf("GetRebirthRanking failed: %v", err)
	}
	for _, entry := range rebirthRankings {
		if entry.CharacterID == c1.ID && entry.Score != 3 {
			t.Fatalf("expected c1 rebirth count 3, got %d", entry.Score)
		}
	}

	// === Test Small Medal Ranking ===
	medalRankings, _, err := rankingRepo.GetSmallMedalRanking(ctx, 100, 0)
	if err != nil {
		t.Fatalf("GetSmallMedalRanking failed: %v", err)
	}
	for _, entry := range medalRankings {
		if entry.CharacterID == c2.ID && entry.Score != 50 {
			t.Fatalf("expected c2 medal count 50, got %d", entry.Score)
		}
	}

	// === Test Snapshots ===
	snap := ranking.RankingSnapshot{
		RankingType:  ranking.RankingTypeLevel,
		SnapshotData: `[{"rank":1,"character_id":"test"}]`,
		TotalCount:   1,
		CalculatedAt: now,
		UpdatedAt:    now,
	}
	if err := rankingRepo.SaveSnapshot(ctx, snap); err != nil {
		t.Fatalf("SaveSnapshot failed: %v", err)
	}

	gotSnap, err := rankingRepo.GetSnapshot(ctx, ranking.RankingTypeLevel)
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
	if _, ok := allSnaps[ranking.RankingTypeLevel]; !ok {
		t.Fatalf("expected level snapshot in all snapshots")
	}

	_, err = rankingRepo.GetSnapshot(ctx, ranking.RankingType("non_existent"))
	if err != ranking.ErrSnapshotNotFound {
		t.Fatalf("expected ErrSnapshotNotFound, got %v", err)
	}
}

// Unused imports check suppression
var (
	_ = pvp.DefaultRating
	_ = boss.ErrBossNotFound
)
