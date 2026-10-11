package ranking_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/ranking"
)

func TestService_WeeklyJobChange(t *testing.T) {
	repo := newMockRepo()
	fixedTime := time.Date(2026, 9, 20, 10, 0, 0, 0, time.UTC) // Sunday
	repo.weeklyRotationDue = fixedTime.Add(-7 * 24 * time.Hour)
	svc, err := ranking.NewService(repo, ranking.WithNowFunc(func() time.Time { return fixedTime }))
	if err != nil {
		t.Fatalf("failed to create ranking service: %v", err)
	}

	ctx := context.Background()

	// 1. Record job changes
	if err := svc.RecordJobChange(ctx, "char-1"); err != nil {
		t.Fatalf("RecordJobChange failed: %v", err)
	}
	if err := svc.RecordJobChange(ctx, "char-1"); err != nil {
		t.Fatalf("RecordJobChange failed: %v", err)
	}
	if err := svc.RecordJobChange(ctx, "char-2"); err != nil {
		t.Fatalf("RecordJobChange failed: %v", err)
	}

	if repo.activeWeeklyJobChanges["char-1"] != 2 {
		t.Fatalf("expected 2 active job changes for char-1, got %d", repo.activeWeeklyJobChanges["char-1"])
	}
	if repo.activeWeeklyJobChanges["char-2"] != 1 {
		t.Fatalf("expected 1 active job change for char-2, got %d", repo.activeWeeklyJobChanges["char-2"])
	}

	// Mock ranking entries
	repo.weeklyJobChangeRankings = []ranking.CharacterRankingEntry{
		{Rank: 1, CharacterID: "char-1", CharacterName: "JobMaster", Score: 2},
		{Rank: 2, CharacterID: "char-2", CharacterName: "Apprentice", Score: 1},
	}
	repo.weeklyJobChangeTotal = 2

	// 2. Query live active weekly rankings (snapshot=false)
	page, err := svc.GetWeeklyJobChangeRanking(ctx, 10, 0, false)
	if err != nil {
		t.Fatalf("GetWeeklyJobChangeRanking live failed: %v", err)
	}
	if page.Total != 2 || len(page.Entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", page.Total)
	}
	if page.Entries[0].CharacterID != "char-1" || page.Entries[0].Score != 2 {
		t.Fatalf("unexpected entry: %+v", page.Entries[0])
	}

	// 3. Weekly rotation: freezes active into snapshot and resets active counts
	if _, err := svc.RotateWeeklyJobChangeRanking(ctx); err != nil {
		t.Fatalf("RotateWeeklyJobChangeRanking failed: %v", err)
	}

	if len(repo.activeWeeklyJobChanges) != 0 {
		t.Fatalf("expected active weekly job changes to be reset, got %d", len(repo.activeWeeklyJobChanges))
	}

	snap, err := svc.GetSnapshot(ctx, ranking.RankingTypeWeeklyJobChange)
	if err != nil {
		t.Fatalf("GetSnapshot failed: %v", err)
	}
	if snap.TotalCount != 2 {
		t.Fatalf("expected snapshot total count 2, got %d", snap.TotalCount)
	}

	// 4. Query snapshot weekly rankings (snapshot=true)
	cachedPage, err := svc.GetWeeklyJobChangeRanking(ctx, 10, 0, true)
	if err != nil {
		t.Fatalf("GetWeeklyJobChangeRanking cached failed: %v", err)
	}
	if !cachedPage.IsSnapshot {
		t.Fatal("expected IsSnapshot to be true")
	}
	if len(cachedPage.Entries) != 2 {
		t.Fatalf("expected 2 entries from cached snapshot, got %d", len(cachedPage.Entries))
	}
}

func TestNextSundayMidnightJST(t *testing.T) {
	jst := time.FixedZone("Asia/Tokyo", 9*60*60)

	// Wednesday at 15:00 JST -> next Sunday 00:00 JST
	wed := time.Date(2026, 9, 23, 15, 0, 0, 0, jst)
	nextSun := ranking.NextSundayMidnightJST(wed)
	expectedSun := time.Date(2026, 9, 27, 0, 0, 0, 0, jst).UTC()
	if !nextSun.Equal(expectedSun) {
		t.Fatalf("expected %v, got %v", expectedSun, nextSun)
	}

	// Sunday at 00:00 JST -> strictly after, so should be 7 days later
	sunMidnight := time.Date(2026, 9, 27, 0, 0, 0, 0, jst)
	nextSun2 := ranking.NextSundayMidnightJST(sunMidnight)
	expectedSun2 := time.Date(2026, 10, 4, 0, 0, 0, 0, jst).UTC()
	if !nextSun2.Equal(expectedSun2) {
		t.Fatalf("expected %v, got %v", expectedSun2, nextSun2)
	}

	// Saturday 23:59:59 JST -> 1 second later (tomorrow Sunday 00:00 JST)
	satNight := time.Date(2026, 9, 26, 23, 59, 59, 0, jst)
	nextSun3 := ranking.NextSundayMidnightJST(satNight)
	expectedSun3 := time.Date(2026, 9, 27, 0, 0, 0, 0, jst).UTC()
	if !nextSun3.Equal(expectedSun3) {
		t.Fatalf("expected %v, got %v", expectedSun3, nextSun3)
	}
}

func TestService_WeeklyJobChange_ReRotationDoesNotOverwriteFinalizedSnapshot(t *testing.T) {
	repo := newMockRepo()
	fixedTime := time.Date(2026, 9, 20, 10, 0, 0, 0, time.UTC)
	repo.weeklyRotationDue = fixedTime.Add(-7 * 24 * time.Hour)
	svc, err := ranking.NewService(repo, ranking.WithNowFunc(func() time.Time { return fixedTime }))
	if err != nil {
		t.Fatalf("failed to create ranking service: %v", err)
	}

	ctx := context.Background()

	// 1. Initial rotation with 2 active entries
	if err := svc.RecordJobChange(ctx, "char-1"); err != nil {
		t.Fatalf("RecordJobChange failed: %v", err)
	}
	repo.weeklyJobChangeRankings = []ranking.CharacterRankingEntry{
		{Rank: 1, CharacterID: "char-1", CharacterName: "JobMaster", Score: 1},
	}
	repo.weeklyJobChangeTotal = 1

	if _, err := svc.RotateWeeklyJobChangeRanking(ctx); err != nil {
		t.Fatalf("RotateWeeklyJobChangeRanking failed: %v", err)
	}

	snapBefore, err := svc.GetSnapshot(ctx, ranking.RankingTypeWeeklyJobChange)
	if err != nil {
		t.Fatalf("GetSnapshot failed: %v", err)
	}
	if snapBefore.TotalCount != 1 {
		t.Fatalf("expected snapshot total count 1, got %d", snapBefore.TotalCount)
	}

	// 2. Simulate re-running rotation when active weekly changes are now empty (0 entries)
	repo.weeklyJobChangeRankings = nil
	repo.weeklyJobChangeTotal = 0

	// Advance time by 10 seconds (immediate retry/re-execution)
	svc, _ = ranking.NewService(repo, ranking.WithNowFunc(func() time.Time { return fixedTime.Add(10 * time.Second) }))
	if _, err := svc.RotateWeeklyJobChangeRanking(ctx); err != nil {
		t.Fatalf("re-running RotateWeeklyJobChangeRanking failed: %v", err)
	}

	// Acceptance criteria: rotation再実行で確定済み前週snapshotを空内容へ上書きしない
	snapAfter, err := svc.GetSnapshot(ctx, ranking.RankingTypeWeeklyJobChange)
	if err != nil {
		t.Fatalf("GetSnapshot failed: %v", err)
	}
	if snapAfter.TotalCount != 1 {
		t.Fatalf("expected finalized snapshot to be preserved with total count 1, got %d", snapAfter.TotalCount)
	}
	if snapAfter.CalculatedAt != snapBefore.CalculatedAt {
		t.Fatalf("expected finalized snapshot CalculatedAt %v, got %v", snapBefore.CalculatedAt, snapAfter.CalculatedAt)
	}
}

func TestService_WeeklyJobChange_RollbackOnResetFailure(t *testing.T) {
	repo := newMockRepo()
	fixedTime := time.Date(2026, 9, 20, 10, 0, 0, 0, time.UTC)
	repo.weeklyRotationDue = fixedTime.Add(-7 * 24 * time.Hour)
	svc, err := ranking.NewService(repo, ranking.WithNowFunc(func() time.Time { return fixedTime }))
	if err != nil {
		t.Fatalf("failed to create ranking service: %v", err)
	}

	ctx := context.Background()

	if err := svc.RecordJobChange(ctx, "char-1"); err != nil {
		t.Fatalf("RecordJobChange failed: %v", err)
	}
	repo.weeklyJobChangeRankings = []ranking.CharacterRankingEntry{
		{Rank: 1, CharacterID: "char-1", CharacterName: "JobMaster", Score: 1},
	}
	repo.weeklyJobChangeTotal = 1

	// Inject reset failure
	repo.resetWeeklyJobChangesErr = errors.New("simulated DB reset failure")

	_, err = svc.RotateWeeklyJobChangeRanking(ctx)
	if err == nil {
		t.Fatal("expected RotateWeeklyJobChangeRanking to fail on reset error, got nil")
	}

	// Acceptance criteria: snapshot保存・reset失敗で一貫したrollbackになる
	// 1. Active weekly job changes should NOT be reset (rolled back)
	if repo.activeWeeklyJobChanges["char-1"] != 1 {
		t.Fatalf("expected active weekly job changes to be rolled back to 1, got %d", repo.activeWeeklyJobChanges["char-1"])
	}

	// 2. Snapshot should NOT be committed in repo
	if _, ok := repo.snapshots[ranking.RankingTypeWeeklyJobChange]; ok {
		t.Fatal("expected snapshot to not be persisted after reset failure")
	}

	// 3. Cache should NOT be published
	cachedPage, err := svc.GetWeeklyJobChangeRanking(ctx, 10, 0, true)
	if err == nil && cachedPage.IsSnapshot && cachedPage.Total > 0 {
		t.Fatal("expected cache to not be populated after failed rotation")
	}
}

func TestService_WeeklyJobChange_RollbackOnSaveSnapshotFailure(t *testing.T) {
	repo := newMockRepo()
	fixedTime := time.Date(2026, 9, 20, 10, 0, 0, 0, time.UTC)
	repo.weeklyRotationDue = fixedTime.Add(-7 * 24 * time.Hour)
	svc, err := ranking.NewService(repo, ranking.WithNowFunc(func() time.Time { return fixedTime }))
	if err != nil {
		t.Fatalf("failed to create ranking service: %v", err)
	}

	ctx := context.Background()

	if err := svc.RecordJobChange(ctx, "char-1"); err != nil {
		t.Fatalf("RecordJobChange failed: %v", err)
	}
	repo.weeklyJobChangeRankings = []ranking.CharacterRankingEntry{
		{Rank: 1, CharacterID: "char-1", CharacterName: "JobMaster", Score: 1},
	}
	repo.weeklyJobChangeTotal = 1

	// Inject save snapshot failure
	repo.saveSnapshotErr = errors.New("simulated save snapshot failure")

	_, err = svc.RotateWeeklyJobChangeRanking(ctx)
	if err == nil {
		t.Fatal("expected RotateWeeklyJobChangeRanking to fail on save snapshot error, got nil")
	}

	// Acceptance criteria: snapshot保存・reset失敗で一貫したrollbackになる
	if repo.activeWeeklyJobChanges["char-1"] != 1 {
		t.Fatalf("expected active weekly job changes to be preserved, got %d", repo.activeWeeklyJobChanges["char-1"])
	}
}

func TestService_WeeklyJobChange_PrerequisiteReadFailurePreservesSnapshot(t *testing.T) {
	repo := newMockRepo()
	fixedTime := time.Date(2026, 9, 20, 10, 0, 0, 0, time.UTC)
	repo.weeklyRotationDue = fixedTime.Add(-7 * 24 * time.Hour)
	svc, err := ranking.NewService(repo, ranking.WithNowFunc(func() time.Time { return fixedTime }))
	if err != nil {
		t.Fatalf("failed to create ranking service: %v", err)
	}

	ctx := context.Background()

	// Initial rotation with 1 entry
	if err := svc.RecordJobChange(ctx, "char-1"); err != nil {
		t.Fatalf("RecordJobChange failed: %v", err)
	}
	repo.weeklyJobChangeRankings = []ranking.CharacterRankingEntry{
		{Rank: 1, CharacterID: "char-1", CharacterName: "JobMaster", Score: 1},
	}
	repo.weeklyJobChangeTotal = 1

	if _, err := svc.RotateWeeklyJobChangeRanking(ctx); err != nil {
		t.Fatalf("RotateWeeklyJobChangeRanking failed: %v", err)
	}

	snapBefore, err := svc.GetSnapshot(ctx, ranking.RankingTypeWeeklyJobChange)
	if err != nil {
		t.Fatalf("GetSnapshot failed: %v", err)
	}
	if snapBefore.TotalCount != 1 {
		t.Fatalf("expected snapshot total count 1, got %d", snapBefore.TotalCount)
	}

	// Active counters are now 0
	repo.weeklyJobChangeRankings = nil
	repo.weeklyJobChangeTotal = 0

	fixedTime = fixedTime.Add(7 * 24 * time.Hour)

	// Inject storage read error on GetSnapshot
	simulatedErr := errors.New("simulated database read failure")
	repo.getSnapshotErr = simulatedErr

	_, err = svc.RotateWeeklyJobChangeRanking(ctx)
	if err == nil {
		t.Fatal("expected RotateWeeklyJobChangeRanking to fail on GetSnapshot storage error, got nil")
	}
	if !errors.Is(err, simulatedErr) {
		t.Fatalf("expected error wrapping %v, got %v", simulatedErr, err)
	}

	// Clear injected read error to verify persistent repo state
	repo.getSnapshotErr = nil

	// Acceptance criteria: storage/cancellation等の読取失敗は伝播し、既存snapshot/counters/cacheを変更しない
	snapAfter, err := svc.GetSnapshot(ctx, ranking.RankingTypeWeeklyJobChange)
	if err != nil {
		t.Fatalf("GetSnapshot after failed rotation failed: %v", err)
	}
	if snapAfter.TotalCount != 1 || snapAfter.SnapshotData != snapBefore.SnapshotData {
		t.Fatalf("expected finalized snapshot to be preserved, got total=%d, data=%s", snapAfter.TotalCount, snapAfter.SnapshotData)
	}
}

func TestService_WeeklyJobChange_EmptyCounters_ErrSnapshotNotFound(t *testing.T) {
	repo := newMockRepo()
	fixedTime := time.Date(2026, 9, 20, 10, 0, 0, 0, time.UTC)
	repo.weeklyRotationDue = fixedTime.Add(-7 * 24 * time.Hour)
	svc, err := ranking.NewService(repo, ranking.WithNowFunc(func() time.Time { return fixedTime }))
	if err != nil {
		t.Fatalf("failed to create ranking service: %v", err)
	}

	ctx := context.Background()

	// Active counters are empty from the start (no job changes)
	repo.weeklyJobChangeRankings = nil
	repo.weeklyJobChangeTotal = 0

	// Repo has no snapshot yet -> GetSnapshot returns ErrSnapshotNotFound
	if _, err := svc.RotateWeeklyJobChangeRanking(ctx); err != nil {
		t.Fatalf("expected RotateWeeklyJobChangeRanking to succeed when no snapshot exists, got %v", err)
	}

	snap, err := svc.GetSnapshot(ctx, ranking.RankingTypeWeeklyJobChange)
	if err != nil {
		t.Fatalf("GetSnapshot failed: %v", err)
	}
	if snap.TotalCount != 0 || snap.SnapshotData != "[]" {
		t.Fatalf("expected empty snapshot (total=0, data='[]'), got total=%d, data=%s", snap.TotalCount, snap.SnapshotData)
	}
}

func TestService_WeeklyJobChange_EmptyCounters_ExistingEmptySnapshot(t *testing.T) {
	repo := newMockRepo()
	fixedTime := time.Date(2026, 9, 20, 10, 0, 0, 0, time.UTC)
	repo.weeklyRotationDue = fixedTime.Add(-7 * 24 * time.Hour)
	svc, err := ranking.NewService(repo, ranking.WithNowFunc(func() time.Time { return fixedTime }))
	if err != nil {
		t.Fatalf("failed to create ranking service: %v", err)
	}

	ctx := context.Background()

	// Pre-populate an existing empty snapshot
	repo.snapshots[ranking.RankingTypeWeeklyJobChange] = ranking.RankingSnapshot{
		RankingType:  ranking.RankingTypeWeeklyJobChange,
		SnapshotData: "[]",
		TotalCount:   0,
		CalculatedAt: fixedTime.Add(-7 * 24 * time.Hour),
		UpdatedAt:    fixedTime.Add(-7 * 24 * time.Hour),
	}

	// Active counters are empty
	repo.weeklyJobChangeRankings = nil
	repo.weeklyJobChangeTotal = 0

	// Rotation with existing empty snapshot and empty counters should succeed
	if _, err := svc.RotateWeeklyJobChangeRanking(ctx); err != nil {
		t.Fatalf("expected RotateWeeklyJobChangeRanking to succeed with existing empty snapshot, got %v", err)
	}

	snap, err := svc.GetSnapshot(ctx, ranking.RankingTypeWeeklyJobChange)
	if err != nil {
		t.Fatalf("GetSnapshot failed: %v", err)
	}
	if snap.TotalCount != 0 {
		t.Fatalf("expected snapshot total count 0, got %d", snap.TotalCount)
	}
}
