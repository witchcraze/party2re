package ranking_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/ranking"
)

func TestRankingServiceIntegration(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()

	rankingRepo, err := database.NewRankingRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	svc, err := ranking.NewService(rankingRepo)
	if err != nil {
		t.Fatal(err)
	}

	// 1. Live queries
	lvlPage, err := svc.GetLevelRanking(ctx, 10, 0, false)
	if err != nil {
		t.Fatalf("GetLevelRanking failed: %v", err)
	}
	if lvlPage.RankingType != ranking.RankingTypeLevel {
		t.Errorf("expected level ranking type, got %s", lvlPage.RankingType)
	}

	wealthPage, err := svc.GetPlayerWealthRanking(ctx, 10, 0, false)
	if err != nil {
		t.Fatalf("GetPlayerWealthRanking failed: %v", err)
	}
	if wealthPage.RankingType != ranking.RankingTypePlayerWealth {
		t.Errorf("expected player_wealth ranking type, got %s", wealthPage.RankingType)
	}

	mkPage, err := svc.GetMonsterKillsRanking(ctx, 10, 0, false)
	if err != nil {
		t.Fatalf("GetMonsterKillsRanking failed: %v", err)
	}
	if mkPage.RankingType != ranking.RankingTypeMonsterKills {
		t.Errorf("expected monster_kills ranking type, got %s", mkPage.RankingType)
	}

	maoPage, err := svc.GetMaoCountRanking(ctx, 10, 0, false)
	if err != nil {
		t.Fatalf("GetMaoCountRanking failed: %v", err)
	}
	if maoPage.RankingType != ranking.RankingTypeMaoCount {
		t.Errorf("expected mao_count ranking type, got %s", maoPage.RankingType)
	}

	heroPage, err := svc.GetHeroCountRanking(ctx, 10, 0, false)
	if err != nil {
		t.Fatalf("GetHeroCountRanking failed: %v", err)
	}
	if heroPage.RankingType != ranking.RankingTypeHeroCount {
		t.Errorf("expected hero_count ranking type, got %s", heroPage.RankingType)
	}

	jobPopPage, err := svc.GetJobPopularityRanking(ctx, false)
	if err != nil {
		t.Fatalf("GetJobPopularityRanking failed: %v", err)
	}
	if jobPopPage.RankingType != ranking.RankingTypeJobPopularity {
		t.Errorf("expected job_popularity ranking type, got %s", jobPopPage.RankingType)
	}

	// 2. Snapshot refresh and cached queries
	if err := svc.RefreshAllSnapshots(ctx); err != nil {
		t.Fatalf("RefreshAllSnapshots failed: %v", err)
	}

	cachedLvlPage, err := svc.GetLevelRanking(ctx, 10, 0, true)
	if err != nil {
		t.Fatalf("GetLevelRanking (cached) failed: %v", err)
	}
	if !cachedLvlPage.IsSnapshot {
		t.Errorf("expected IsSnapshot=true for cached page")
	}

	// 3. Snapshot retrieval
	snap, err := svc.GetSnapshot(ctx, ranking.RankingTypeLevel)
	if err != nil {
		t.Fatalf("GetSnapshot failed: %v", err)
	}
	if snap.RankingType != ranking.RankingTypeLevel {
		t.Errorf("expected ranking type level, got %s", snap.RankingType)
	}

	allSnaps, err := svc.GetAllSnapshots(ctx)
	if err != nil {
		t.Fatalf("GetAllSnapshots failed: %v", err)
	}
	if len(allSnaps) < 13 {
		t.Errorf("expected at least 13 snapshots, got %d", len(allSnaps))
	}

	// 3b. Casino wins and Alchemy rankings
	casPage, err := svc.GetCasinoWinsRanking(ctx, 10, 0, false)
	if err != nil {
		t.Fatalf("GetCasinoWinsRanking failed: %v", err)
	}
	if casPage.RankingType != ranking.RankingTypeCasinoWins {
		t.Errorf("expected casino_wins ranking type, got %s", casPage.RankingType)
	}

	alcPage, err := svc.GetAlchemyRanking(ctx, 10, 0, false)
	if err != nil {
		t.Fatalf("GetAlchemyRanking failed: %v", err)
	}
	if alcPage.RankingType != ranking.RankingTypeAlchemy {
		t.Errorf("expected alchemy ranking type, got %s", alcPage.RankingType)
	}

	// 3c. Weekly job changes and rotation
	if err := svc.RotateWeeklyJobChangeRanking(ctx); err != nil {
		t.Fatalf("RotateWeeklyJobChangeRanking failed: %v", err)
	}
	weeklyPage, err := svc.GetWeeklyJobChangeRanking(ctx, 10, 0, true)
	if err != nil {
		t.Fatalf("GetWeeklyJobChangeRanking failed: %v", err)
	}
	if weeklyPage.RankingType != ranking.RankingTypeWeeklyJobChange {
		t.Errorf("expected weekly_job_change ranking type, got %s", weeklyPage.RankingType)
	}

	// 3d. Hall of Fame
	legends, err := svc.GetLegends(ctx)
	if err != nil {
		t.Fatalf("GetLegends failed: %v", err)
	}
	if len(legends) != 6 {
		t.Fatalf("expected 6 legend categories, got %d", len(legends))
	}
	legPage, err := svc.GetLegendCategory(ctx, ranking.LegendCategoryJobMastery)
	if err != nil {
		t.Fatalf("GetLegendCategory failed: %v", err)
	}
	if legPage.Category != ranking.LegendCategoryJobMastery {
		t.Errorf("expected comp_job category, got %s", legPage.Category)
	}

	// 4. Cold-start Service instance (clean in-memory cache) falls back to database snapshot
	freshSvc, err := ranking.NewService(rankingRepo)
	if err != nil {
		t.Fatal(err)
	}

	coldLvlPage, err := freshSvc.GetLevelRanking(ctx, 10, 0, true)
	if err != nil {
		t.Fatalf("GetLevelRanking (cold snapshot fallback) failed: %v", err)
	}
	if !coldLvlPage.IsSnapshot {
		t.Errorf("expected IsSnapshot=true for cold start snapshot query")
	}

	// 5. Warmup cache on fresh instance
	warmupSvc, err := ranking.NewService(rankingRepo)
	if err != nil {
		t.Fatal(err)
	}
	if err := warmupSvc.WarmupCache(ctx); err != nil {
		t.Fatalf("WarmupCache failed: %v", err)
	}
	warmLvlPage, err := warmupSvc.GetLevelRanking(ctx, 10, 0, true)
	if err != nil || !warmLvlPage.IsSnapshot {
		t.Fatalf("expected cached ranking after warmup: %v, %+v", err, warmLvlPage)
	}
}

type interceptingRankingRepo struct {
	ranking.Repository
	onAfterGetActiveWeekly func()
}

func (r *interceptingRankingRepo) RunInTx(ctx context.Context, fn func(ctx context.Context) error) error {
	if tp, ok := r.Repository.(ranking.TransactionProvider); ok {
		return tp.RunInTx(ctx, fn)
	}
	return fn(ctx)
}

func (r *interceptingRankingRepo) GetActiveWeeklyJobChangeRanking(ctx context.Context, limit, offset int) ([]ranking.CharacterRankingEntry, int, error) {
	entries, total, err := r.Repository.GetActiveWeeklyJobChangeRanking(ctx, limit, offset)
	if err == nil && r.onAfterGetActiveWeekly != nil {
		r.onAfterGetActiveWeekly()
	}
	return entries, total, err
}

func TestWeeklyJobChange_DeterministicInterleaving_RealDB(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()

	// Clean up weekly job changes before test
	if _, err := db.ExecContext(ctx, "DELETE FROM weekly_job_changes"); err != nil {
		t.Fatal(err)
	}

	rankingRepo, err := database.NewRankingRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	// Create two test characters
	charA, err := database.CreateTestCharacter(ctx, db, fmt.Sprintf("WJCA_%d", time.Now().UnixNano()%100000))
	if err != nil {
		t.Fatal(err)
	}
	charB, err := database.CreateTestCharacter(ctx, db, fmt.Sprintf("WJCB_%d", time.Now().UnixNano()%100000))
	if err != nil {
		t.Fatal(err)
	}

	// Record 2 initial job changes for charA before rotation starts
	baseSvc, err := ranking.NewService(rankingRepo)
	if err != nil {
		t.Fatal(err)
	}
	if err := baseSvc.RecordJobChange(ctx, charA.ID); err != nil {
		t.Fatalf("RecordJobChange charA 1 failed: %v", err)
	}
	if err := baseSvc.RecordJobChange(ctx, charA.ID); err != nil {
		t.Fatalf("RecordJobChange charA 2 failed: %v", err)
	}

	readDone := make(chan struct{})
	resumeReset := make(chan struct{})
	var intercepted atomic.Bool

	interceptedRepo := &interceptingRankingRepo{
		Repository: rankingRepo,
		onAfterGetActiveWeekly: func() {
			if intercepted.CompareAndSwap(false, true) {
				close(readDone)
				<-resumeReset
			}
		},
	}

	rotSvc, err := ranking.NewService(interceptedRepo)
	if err != nil {
		t.Fatal(err)
	}

	// 1. Run RotateWeeklyJobChangeRanking in background goroutine.
	// It acquires LockWeeklyJobChangesForUpdate, reads active changes, and pauses on readDone.
	rotErr := make(chan error, 1)
	go func() {
		rotErr <- rotSvc.RotateWeeklyJobChangeRanking(ctx)
	}()

	select {
	case <-readDone:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for active ranking read")
	}

	// 2. Concurrently record a job change for charB while rotation is paused between read and reset.
	// Because rotation holds LockWeeklyJobChangesForUpdate under its transaction,
	// this call MUST wait on the row lock and not finish until rotation commits!
	writerDone := make(chan error, 1)
	go func() {
		writerDone <- rotSvc.RecordJobChange(ctx, charB.ID)
	}()

	// Verify that writer is blocked while rotation transaction holds the lock
	select {
	case err := <-writerDone:
		t.Fatalf("RecordJobChange completed prematurely while rotation held lock: %v", err)
	case <-time.After(150 * time.Millisecond):
		// Expected: writer is waiting for rotation lock to be released
	}

	// 3. Resume rotation: saves snapshot and executes ResetWeeklyJobChanges, then commits.
	close(resumeReset)

	select {
	case err := <-rotErr:
		if err != nil {
			t.Fatalf("RotateWeeklyJobChangeRanking failed: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for rotation to complete")
	}

	// 4. Now writer must unblock, write its increment into the new week, and complete successfully.
	select {
	case err := <-writerDone:
		if err != nil {
			t.Fatalf("RecordJobChange failed: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for RecordJobChange to complete after rotation")
	}

	// 5. Verify Invariants:
	// - Snapshot contains charA with count 2, and charB is NOT in snapshot.
	snap, err := rotSvc.GetSnapshot(ctx, ranking.RankingTypeWeeklyJobChange)
	if err != nil {
		t.Fatalf("GetSnapshot failed: %v", err)
	}
	var snapEntries []ranking.CharacterRankingEntry
	if err := json.Unmarshal([]byte(snap.SnapshotData), &snapEntries); err != nil {
		t.Fatalf("unmarshal snapshot failed: %v", err)
	}

	var foundCharA, foundCharB bool
	for _, entry := range snapEntries {
		if entry.CharacterID == charA.ID {
			foundCharA = true
			if entry.Score != 2 {
				t.Fatalf("expected charA score 2 in snapshot, got %d", entry.Score)
			}
		}
		if entry.CharacterID == charB.ID {
			foundCharB = true
		}
	}
	if !foundCharA {
		t.Fatal("expected charA in frozen previous-week snapshot")
	}
	if foundCharB {
		t.Fatal("charB should NOT be in previous-week snapshot")
	}

	// - Active weekly job changes contains charB with count 1, and charA has 0 rows (reset).
	activeEntries, total, err := rankingRepo.GetActiveWeeklyJobChangeRanking(ctx, 10, 0)
	if err != nil {
		t.Fatalf("GetActiveWeeklyJobChangeRanking failed: %v", err)
	}
	if total != 1 || len(activeEntries) != 1 {
		t.Fatalf("expected exactly 1 active weekly entry for charB in new week, got total=%d, entries=%d", total, len(activeEntries))
	}
	if activeEntries[0].CharacterID != charB.ID || activeEntries[0].Score != 1 {
		t.Fatalf("unexpected active entry in new week: %+v", activeEntries[0])
	}
}

func TestWeeklyJobChange_ConcurrentRace_RealDB(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()

	rankingRepo, err := database.NewRankingRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	svc, err := ranking.NewService(rankingRepo)
	if err != nil {
		t.Fatal(err)
	}

	for iter := 0; iter < 5; iter++ {
		char, err := database.CreateTestCharacter(ctx, db, fmt.Sprintf("RaceWJC_%d_%d", iter, time.Now().UnixNano()%10000))
		if err != nil {
			t.Fatal(err)
		}

		err1, err2 := database.RunRace2(
			func() error {
				return svc.RotateWeeklyJobChangeRanking(ctx)
			},
			func() error {
				return svc.RecordJobChange(ctx, char.ID)
			},
		)

		if database.IsDeadlockError(err1) || database.IsDeadlockError(err2) {
			t.Fatalf("iter %d: DEADLOCK detected! err1=%v, err2=%v", iter, err1, err2)
		}
		if err1 != nil {
			t.Fatalf("iter %d: RotateWeeklyJobChangeRanking error: %v", iter, err1)
		}
		if err2 != nil {
			t.Fatalf("iter %d: RecordJobChange error: %v", iter, err2)
		}
	}
}

func TestWeeklyJobChange_ReRotationDoesNotOverwriteFinalizedSnapshot_RealDB(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()

	if _, err := db.ExecContext(ctx, "DELETE FROM weekly_job_changes"); err != nil {
		t.Fatal(err)
	}

	rankingRepo, err := database.NewRankingRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	char, err := database.CreateTestCharacter(ctx, db, fmt.Sprintf("ReRotWJC_%d", time.Now().UnixNano()%100000))
	if err != nil {
		t.Fatal(err)
	}

	svc, err := ranking.NewService(rankingRepo)
	if err != nil {
		t.Fatal(err)
	}

	if err := svc.RecordJobChange(ctx, char.ID); err != nil {
		t.Fatal(err)
	}

	// 1. Initial rotation freezes 1 entry
	if err := svc.RotateWeeklyJobChangeRanking(ctx); err != nil {
		t.Fatalf("first RotateWeeklyJobChangeRanking failed: %v", err)
	}

	snap1, err := svc.GetSnapshot(ctx, ranking.RankingTypeWeeklyJobChange)
	if err != nil {
		t.Fatalf("GetSnapshot failed: %v", err)
	}
	if snap1.TotalCount < 1 {
		t.Fatalf("expected snapshot total count >= 1, got %d", snap1.TotalCount)
	}

	// 2. Immediate re-rotation when active table is now empty
	if err := svc.RotateWeeklyJobChangeRanking(ctx); err != nil {
		t.Fatalf("second RotateWeeklyJobChangeRanking failed: %v", err)
	}

	// Acceptance criteria: rotation再実行で確定済み前週snapshotを空内容へ上書きしない
	snap2, err := svc.GetSnapshot(ctx, ranking.RankingTypeWeeklyJobChange)
	if err != nil {
		t.Fatalf("GetSnapshot after re-rotation failed: %v", err)
	}
	if snap2.TotalCount != snap1.TotalCount {
		t.Fatalf("expected snapshot count preserved (%d), got %d", snap1.TotalCount, snap2.TotalCount)
	}
	if snap2.CalculatedAt != snap1.CalculatedAt {
		t.Fatalf("expected snapshot CalculatedAt preserved (%v), got %v", snap1.CalculatedAt, snap2.CalculatedAt)
	}
}
