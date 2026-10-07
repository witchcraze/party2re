package database

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/ranking"
)

func findRankingEntry(
	ctx context.Context,
	charID string,
	fetch func(ctx context.Context, limit, offset int) ([]ranking.CharacterRankingEntry, int, error),
) (*ranking.CharacterRankingEntry, int, error) {
	const pageSize = 10
	totalCount := 0
	for offset := 0; ; offset += pageSize {
		entries, total, err := fetch(ctx, pageSize, offset)
		if err != nil {
			return nil, 0, err
		}
		totalCount = total
		for i := range entries {
			if entries[i].CharacterID == charID {
				entry := entries[i]
				return &entry, totalCount, nil
			}
		}
		if len(entries) == 0 || offset+len(entries) >= total {
			break
		}
	}
	return nil, totalCount, nil
}

func TestRankingExtraRepository_Integration(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close db failed: %v", err)
		}
	})

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

	// Base test player & fixture character
	p, err := CreateTestPlayer(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := playerRepo.Delete(ctx, p.ID); err != nil {
			t.Errorf("cleanup player %s failed: %v", p.ID, err)
		}
	})

	c, err := corecharacter.NewWithOptions(prefix+"Char", "job-01", "m", nil)
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
		if err := charRepo.Delete(ctx, charID); err != nil {
			t.Errorf("cleanup char %s failed: %v", charID, err)
		}
	})

	// Seed interfering characters with higher/tied scores ahead of the test fixture to verify
	// that ranking integration assertions succeed even when the fixture is pushed beyond the first page.
	interferePlayer, err := CreateTestPlayer(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := playerRepo.Delete(ctx, interferePlayer.ID); err != nil {
			t.Errorf("cleanup interfere player %s failed: %v", interferePlayer.ID, err)
		}
	})

	const numInterfering = 11
	for i := 0; i < numInterfering; i++ {
		intChar, err := corecharacter.NewWithOptions(fmt.Sprintf("%si%d", prefix, i), "job-01", "m", nil)
		if err != nil {
			t.Fatal(err)
		}
		intChar.PlayerID = interferePlayer.ID
		// Mix of higher scores (> 15) and tied scores (15 with higher level 30 vs fixture's 25)
		if i%2 == 0 {
			intChar.CasinoWins = 20
			intChar.Level = 25
		} else {
			intChar.CasinoWins = 15
			intChar.Level = 30
		}
		if err := charRepo.Save(ctx, intChar); err != nil {
			t.Fatal(err)
		}
		intCharID := intChar.ID
		t.Cleanup(func() {
			if err := charRepo.Delete(ctx, intCharID); err != nil {
				t.Errorf("cleanup interfere char %s failed: %v", intCharID, err)
			}
		})

		// Seed interfering alchemy crafts ahead of fixture's 42 (crafts = 50)
		_, err = db.ExecContext(ctx, "INSERT INTO character_alchemy (character_id, total_crafts) VALUES (?, ?)", intCharID, 50)
		if err != nil {
			t.Fatalf("insert interfering alchemy failed: %v", err)
		}

		// Seed interfering weekly job changes ahead of fixture's 2 (change count = 5)
		_, err = db.ExecContext(ctx, "INSERT INTO weekly_job_changes (character_id, change_count) VALUES (?, ?)", intCharID, 5)
		if err != nil {
			t.Fatalf("insert interfering weekly job changes failed: %v", err)
		}
	}

	// 1. Test GetCasinoWinsRanking
	// Verify that page 0 does not contain our fixture because of the interfering rows.
	firstPageCas, casTotal, err := rankingRepo.GetCasinoWinsRanking(ctx, 10, 0)
	if err != nil {
		t.Fatalf("GetCasinoWinsRanking page 0 failed: %v", err)
	}
	if casTotal < numInterfering+1 {
		t.Fatalf("expected casino total >= %d, got %d", numInterfering+1, casTotal)
	}
	for _, entry := range firstPageCas {
		if entry.CharacterID == charID {
			t.Fatalf("expected fixture %s to be pushed past first page by interfering rows", charID)
		}
	}

	// Verify that paginated search locates our fixture and validates its score.
	casEntry, _, err := findRankingEntry(ctx, charID, rankingRepo.GetCasinoWinsRanking)
	if err != nil {
		t.Fatalf("findRankingEntry for casino wins failed: %v", err)
	}
	if casEntry == nil {
		t.Fatalf("character %s not found in casino wins ranking across pages", charID)
	}
	if casEntry.Score != 15 {
		t.Fatalf("expected casino wins score 15, got %d", casEntry.Score)
	}

	// 2. Test GetAlchemyRanking
	// Insert character_alchemy record for the test fixture
	_, err = db.ExecContext(ctx, "INSERT INTO character_alchemy (character_id, total_crafts) VALUES (?, ?) ON DUPLICATE KEY UPDATE total_crafts = ?", charID, 42, 42)
	if err != nil {
		t.Fatalf("insert character_alchemy failed: %v", err)
	}

	firstPageAlc, alcTotal, err := rankingRepo.GetAlchemyRanking(ctx, 10, 0)
	if err != nil {
		t.Fatalf("GetAlchemyRanking page 0 failed: %v", err)
	}
	if alcTotal < numInterfering+1 {
		t.Fatalf("expected alchemy total >= %d, got %d", numInterfering+1, alcTotal)
	}
	for _, entry := range firstPageAlc {
		if entry.CharacterID == charID {
			t.Fatalf("expected fixture %s to be pushed past first page in alchemy ranking", charID)
		}
	}

	alcEntry, _, err := findRankingEntry(ctx, charID, rankingRepo.GetAlchemyRanking)
	if err != nil {
		t.Fatalf("findRankingEntry for alchemy ranking failed: %v", err)
	}
	if alcEntry == nil {
		t.Fatalf("character %s not found in alchemy ranking across pages", charID)
	}
	if alcEntry.Score != 42 {
		t.Fatalf("expected alchemy craft score 42, got %d", alcEntry.Score)
	}

	// 3. Test Weekly Job Changes
	if err := rankingRepo.IncrementJobChangeCount(ctx, charID); err != nil {
		t.Fatalf("IncrementJobChangeCount failed: %v", err)
	}
	if err := rankingRepo.IncrementJobChangeCount(ctx, charID); err != nil {
		t.Fatalf("second IncrementJobChangeCount failed: %v", err)
	}

	firstPageWJC, wjcTotal, err := rankingRepo.GetActiveWeeklyJobChangeRanking(ctx, 10, 0)
	if err != nil {
		t.Fatalf("GetActiveWeeklyJobChangeRanking page 0 failed: %v", err)
	}
	if wjcTotal < numInterfering+1 {
		t.Fatalf("expected weekly job change total >= %d, got %d", numInterfering+1, wjcTotal)
	}
	for _, entry := range firstPageWJC {
		if entry.CharacterID == charID {
			t.Fatalf("expected fixture %s to be pushed past first page in weekly job changes", charID)
		}
	}

	wjcEntry, _, err := findRankingEntry(ctx, charID, rankingRepo.GetActiveWeeklyJobChangeRanking)
	if err != nil {
		t.Fatalf("findRankingEntry for weekly job changes failed: %v", err)
	}
	if wjcEntry == nil {
		t.Fatalf("character %s not found in active weekly job changes across pages", charID)
	}
	if wjcEntry.Score != 2 {
		t.Fatalf("expected weekly job change count 2, got %d", wjcEntry.Score)
	}

	// Test ResetWeeklyJobChanges within an isolated rollback boundary so
	// global Sunday-midnight rotation is verified against all active records
	// without permanently deleting unrelated shared development/test data.
	errRollback := errors.New("rollback weekly reset")
	err = RunInTx(ctx, db, func(txCtx context.Context) error {
		if err := rankingRepo.ResetWeeklyJobChanges(txCtx); err != nil {
			return fmt.Errorf("ResetWeeklyJobChanges failed: %w", err)
		}
		wjcAfter, wjcTotalAfter, err := rankingRepo.GetActiveWeeklyJobChangeRanking(txCtx, 10, 0)
		if err != nil {
			return fmt.Errorf("GetActiveWeeklyJobChangeRanking after reset failed: %w", err)
		}
		if wjcTotalAfter != 0 || len(wjcAfter) != 0 {
			return fmt.Errorf("expected 0 active weekly job changes after reset in transaction, got %d", wjcTotalAfter)
		}
		return errRollback
	})
	if err != nil && !errors.Is(err, errRollback) {
		t.Fatalf("isolated ResetWeeklyJobChanges assertion failed: %v", err)
	}

	// Outside the isolated rollback transaction, verify that active records survived and were restored.
	wjcRestored, _, err := findRankingEntry(ctx, charID, rankingRepo.GetActiveWeeklyJobChangeRanking)
	if err != nil {
		t.Fatalf("findRankingEntry after rollback failed: %v", err)
	}
	if wjcRestored == nil {
		t.Fatalf("expected fixture %s to survive rollback of weekly reset", charID)
	}
	if wjcRestored.Score != 2 {
		t.Fatalf("expected weekly job change count 2 after rollback, got %d", wjcRestored.Score)
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

	// 5. Test auto-enrichment when profile fields are omitted in LegendEntry
	minimalEntry := ranking.LegendEntry{
		Category:    ranking.LegendCategoryMonsterMastery,
		CharacterID: charID,
	}
	newlyInductedMin, err := rankingRepo.RecordLegend(ctx, minimalEntry)
	if err != nil {
		t.Fatalf("RecordLegend with minimalEntry failed: %v", err)
	}
	if !newlyInductedMin {
		t.Fatal("expected newlyInductedMin to be true")
	}
	monInductees, err := rankingRepo.GetLegendInductees(ctx, ranking.LegendCategoryMonsterMastery)
	if err != nil {
		t.Fatalf("GetLegendInductees for comp_mon failed: %v", err)
	}
	foundMon := false
	for _, ind := range monInductees {
		if ind.CharacterID == charID {
			foundMon = true
			if ind.CharacterName != c.Name {
				t.Fatalf("expected auto-enriched CharacterName %s, got %s", c.Name, ind.CharacterName)
			}
			break
		}
	}
	if !foundMon {
		t.Fatalf("legend record for minimal entry %s not found", charID)
	}
}

func TestRankingExtraRepository_Integration_ResetIsolation(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close db failed: %v", err)
		}
	})

	ctx := context.Background()
	playerRepo, err := NewPlayerRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	charRepo, err := NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	// 1. Seed sentinel player and character representing unrelated persisted data.
	sentinelPlayer, err := CreateTestPlayer(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := playerRepo.Delete(ctx, sentinelPlayer.ID); err != nil {
			t.Errorf("cleanup sentinel player %s failed: %v", sentinelPlayer.ID, err)
		}
	})

	sentinelPrefix := fmt.Sprintf("snt_%d_", time.Now().UnixNano()%1000000)
	sentinelChar, err := corecharacter.NewWithOptions(sentinelPrefix+"Char", "job-01", "m", nil)
	if err != nil {
		t.Fatal(err)
	}
	sentinelChar.PlayerID = sentinelPlayer.ID
	if err := charRepo.Save(ctx, sentinelChar); err != nil {
		t.Fatal(err)
	}
	sentinelCharID := sentinelChar.ID
	t.Cleanup(func() {
		if err := charRepo.Delete(ctx, sentinelCharID); err != nil {
			t.Errorf("cleanup sentinel char %s failed: %v", sentinelCharID, err)
		}
	})

	const sentinelCount = 7
	_, err = db.ExecContext(ctx, "INSERT INTO weekly_job_changes (character_id, change_count) VALUES (?, ?)", sentinelCharID, sentinelCount)
	if err != nil {
		t.Fatalf("insert sentinel weekly job change failed: %v", err)
	}

	// 2. Run the complete integration test across repeated executions to verify
	// that unrelated persisted records are preserved across test runs and teardown paths.
	for run := 1; run <= 2; run++ {
		t.Run(fmt.Sprintf("CompleteIntegration_Run%d", run), func(subT *testing.T) {
			TestRankingExtraRepository_Integration(subT)
		})

		// 3. Verify sentinel unrelated weekly count survives after the complete integration test and its cleanup.
		var currentCount int
		err = db.QueryRowContext(ctx, "SELECT change_count FROM weekly_job_changes WHERE character_id = ?", sentinelCharID).Scan(&currentCount)
		if err != nil {
			t.Fatalf("sentinel weekly job change record disappeared after run %d: %v", run, err)
		}
		if currentCount != sentinelCount {
			t.Fatalf("expected sentinel count %d to survive after run %d, got %d", sentinelCount, run, currentCount)
		}
	}
}
