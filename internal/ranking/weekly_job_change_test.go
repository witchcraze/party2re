package ranking_test

import (
	"context"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/ranking"
)

func TestService_WeeklyJobChange(t *testing.T) {
	repo := newMockRepo()
	fixedTime := time.Date(2026, 9, 20, 10, 0, 0, 0, time.UTC) // Sunday
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
	if err := svc.RotateWeeklyJobChangeRanking(ctx); err != nil {
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
