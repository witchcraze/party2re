package database

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/ranking"
)

func TestRankingExtraRepository_Integration(t *testing.T) {
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

	ctx := context.Background()
	now := time.Now().UTC()
	prefix := fmt.Sprintf("rkx_%d_", time.Now().UnixNano()%1000000)

	playerID := prefix + "p1"
	p, err := coreplayer.New(playerID, "pass", now)
	if err != nil {
		t.Fatal(err)
	}
	if err := playerRepo.Save(ctx, p); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = playerRepo.Delete(ctx, playerID)
	})

	c, err := corecharacter.NewWithOptions(prefix+"Char", "warrior", "m", nil)
	if err != nil {
		t.Fatal(err)
	}
	c.PlayerID = p.ID
	c.Level = 25
	c.Experience = 500
	c.CasinoWins = 15
	if err := charRepo.Save(ctx, c); err != nil {
		t.Fatal(err)
	}
	charID := c.ID
	t.Cleanup(func() {
		_ = charRepo.Delete(ctx, charID)
	})

	// 1. Test GetCasinoWinsRanking
	casRankings, total, err := rankingRepo.GetCasinoWinsRanking(ctx, 10, 0)
	if err != nil {
		t.Fatalf("GetCasinoWinsRanking failed: %v", err)
	}
	if total < 1 {
		t.Fatalf("expected total >= 1, got %d", total)
	}
	foundCas := false
	for _, entry := range casRankings {
		if entry.CharacterID == charID {
			foundCas = true
			if entry.Score != 15 {
				t.Fatalf("expected casino wins score 15, got %d", entry.Score)
			}
			break
		}
	}
	if !foundCas {
		t.Fatalf("character %s not found in casino wins ranking", charID)
	}

	// 2. Test GetAlchemyRanking
	// Insert character_alchemy record
	_, err = db.ExecContext(ctx, "INSERT INTO character_alchemy (character_id, total_crafts) VALUES (?, ?) ON DUPLICATE KEY UPDATE total_crafts = ?", charID, 42, 42)
	if err != nil {
		t.Fatalf("insert character_alchemy failed: %v", err)
	}

	alcRankings, alcTotal, err := rankingRepo.GetAlchemyRanking(ctx, 10, 0)
	if err != nil {
		t.Fatalf("GetAlchemyRanking failed: %v", err)
	}
	if alcTotal < 1 {
		t.Fatalf("expected alcTotal >= 1, got %d", alcTotal)
	}
	foundAlc := false
	for _, entry := range alcRankings {
		if entry.CharacterID == charID {
			foundAlc = true
			if entry.Score != 42 {
				t.Fatalf("expected alchemy craft score 42, got %d", entry.Score)
			}
			break
		}
	}
	if !foundAlc {
		t.Fatalf("character %s not found in alchemy ranking", charID)
	}

	// 3. Test Weekly Job Changes
	if err := rankingRepo.IncrementJobChangeCount(ctx, charID); err != nil {
		t.Fatalf("IncrementJobChangeCount failed: %v", err)
	}
	if err := rankingRepo.IncrementJobChangeCount(ctx, charID); err != nil {
		t.Fatalf("second IncrementJobChangeCount failed: %v", err)
	}

	wjcRankings, wjcTotal, err := rankingRepo.GetActiveWeeklyJobChangeRanking(ctx, 10, 0)
	if err != nil {
		t.Fatalf("GetActiveWeeklyJobChangeRanking failed: %v", err)
	}
	if wjcTotal < 1 {
		t.Fatalf("expected wjcTotal >= 1, got %d", wjcTotal)
	}
	foundWJC := false
	for _, entry := range wjcRankings {
		if entry.CharacterID == charID {
			foundWJC = true
			if entry.Score != 2 {
				t.Fatalf("expected weekly job change count 2, got %d", entry.Score)
			}
			break
		}
	}
	if !foundWJC {
		t.Fatalf("character %s not found in active weekly job changes", charID)
	}

	// Test ResetWeeklyJobChanges
	if err := rankingRepo.ResetWeeklyJobChanges(ctx); err != nil {
		t.Fatalf("ResetWeeklyJobChanges failed: %v", err)
	}
	wjcAfter, wjcTotalAfter, err := rankingRepo.GetActiveWeeklyJobChangeRanking(ctx, 10, 0)
	if err != nil {
		t.Fatalf("GetActiveWeeklyJobChangeRanking after reset failed: %v", err)
	}
	if wjcTotalAfter != 0 || len(wjcAfter) != 0 {
		t.Fatalf("expected 0 active weekly job changes after reset, got %d", wjcTotalAfter)
	}

	// 4. Test Hall of Fame (legend_records)
	legendEntry := ranking.LegendEntry{
		Category:      ranking.LegendCategoryJobMastery,
		CharacterID:   charID,
		CharacterName: prefix + "Legend",
		GuildName:     "Champions",
		Color:         "#00ff00",
		Icon:          "champ.png",
		Message:       "Master of all jobs!",
		InductedAt:    now,
	}

	newlyInducted, err := rankingRepo.RecordLegend(ctx, legendEntry)
	if err != nil {
		t.Fatalf("RecordLegend failed: %v", err)
	}
	if !newlyInducted {
		t.Fatal("expected newlyInducted to be true")
	}

	// Duplicate prevention
	dupInducted, err := rankingRepo.RecordLegend(ctx, legendEntry)
	if err != nil {
		t.Fatalf("duplicate RecordLegend failed: %v", err)
	}
	if dupInducted {
		t.Fatal("expected duplicate RecordLegend to return false")
	}

	// Query inductees
	inductees, err := rankingRepo.GetLegendInductees(ctx, ranking.LegendCategoryJobMastery)
	if err != nil {
		t.Fatalf("GetLegendInductees failed: %v", err)
	}
	foundLegend := false
	for _, ind := range inductees {
		if ind.CharacterID == charID {
			foundLegend = true
			if ind.CharacterName != prefix+"Legend" || ind.GuildName != "Champions" {
				t.Fatalf("unexpected legend details: %+v", ind)
			}
			break
		}
	}
	if !foundLegend {
		t.Fatalf("legend record for %s not found in inductees", charID)
	}

	// Query category counts
	counts, err := rankingRepo.GetLegendCategoryCounts(ctx)
	if err != nil {
		t.Fatalf("GetLegendCategoryCounts failed: %v", err)
	}
	if counts[ranking.LegendCategoryJobMastery] < 1 {
		t.Fatalf("expected comp_job count >= 1, got %d", counts[ranking.LegendCategoryJobMastery])
	}
}
