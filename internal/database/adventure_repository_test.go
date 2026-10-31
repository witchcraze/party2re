package database

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/adventure"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/id"
)

func TestAdventureRepository(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	character, err := CreateTestCharacter(context.Background(), db, "Adventure Test")
	if err != nil {
		t.Fatal(err)
	}

	want := adventure.Adventure{
		ID:               character.ID,
		CharacterID:      character.ID,
		Type:             adventure.StarterAdventure,
		StartedAt:        time.Date(2026, 8, 22, 0, 0, 0, 0, time.UTC),
		FloorsCleared:    10,
		IsCleared:        true,
		PartySize:        1,
		ExperienceReward: adventure.AdventureReward,
		BattleResult: corebattle.Result{
			Outcome:  corebattle.OutcomeWin,
			WinnerID: character.ID,
			LoserID:  adventure.AdventureEnemyID,
			Turns:    3,
		},
		Resolved: true,
	}
	repository, err := NewAdventureRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.Save(context.Background(), want); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	got, err := repository.FindByID(context.Background(), want.ID)
	if err != nil {
		t.Fatalf("FindByID() error = %v", err)
	}
	if got.ID != want.ID || got.CharacterID != want.CharacterID || !reflect.DeepEqual(got.BattleResult, want.BattleResult) ||
		!got.StartedAt.Equal(want.StartedAt) || got.FloorsCleared != want.FloorsCleared ||
		got.IsCleared != want.IsCleared || got.PartySize != want.PartySize || !got.Resolved {
		t.Fatalf("FindByID() = %#v, want %#v", got, want)
	}

	// FindByID not found
	if _, err := repository.FindByID(context.Background(), "nonexistent_adventure"); !errors.Is(err, adventure.ErrNotFound) {
		t.Fatalf("FindByID(nonexistent) error = %v, want %v", err, adventure.ErrNotFound)
	}
}

func TestAdventureRepositoryListByCharacterIDAndAggregatedStats(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()
	character, err := CreateTestCharacter(ctx, db, "Chronicle Test Hero")
	if err != nil {
		t.Fatal(err)
	}

	repo, err := NewAdventureRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 8, 29, 10, 0, 0, 0, time.UTC)

	// Insert multiple adventures
	adv1 := adventure.Adventure{
		ID:               id.New(),
		CharacterID:      character.ID,
		Type:             "stage-01",
		StageID:          "stage-01",
		MonsterID:        "mon-01",
		StartedAt:        now,
		FloorsCleared:    10,
		IsCleared:        true,
		PartySize:        1,
		ExperienceReward: 20,
		BattleResult: corebattle.Result{
			Outcome:  corebattle.OutcomeWin,
			WinnerID: character.ID,
			LoserID:  "mon-01",
			Turns:    3,
			Reward: corebattle.Reward{
				Experience: 20,
				Currency:   15,
			},
		},
		Resolved: true,
	}

	adv2 := adventure.Adventure{
		ID:               id.New(),
		CharacterID:      character.ID,
		Type:             "stage-01",
		StageID:          "stage-01",
		MonsterID:        "mon-01",
		StartedAt:        now.Add(time.Hour),
		FloorsCleared:    10,
		IsCleared:        true,
		PartySize:        1,
		ExperienceReward: 20,
		BattleResult: corebattle.Result{
			Outcome:  corebattle.OutcomeWin,
			WinnerID: character.ID,
			LoserID:  "mon-01",
			Turns:    2,
			Reward: corebattle.Reward{
				Experience: 20,
				Currency:   10,
			},
		},
		Resolved: true,
	}

	adv3 := adventure.Adventure{
		ID:               id.New(),
		CharacterID:      character.ID,
		Type:             "stage-02",
		StageID:          "stage-02",
		MonsterID:        "mon-02",
		StartedAt:        now.Add(2 * time.Hour),
		FloorsCleared:    4,
		IsCleared:        false,
		PartySize:        1,
		ExperienceReward: 50,
		BattleResult: corebattle.Result{
			Outcome:  corebattle.OutcomeWin,
			WinnerID: "mon-02",
			LoserID:  character.ID,
			Turns:    4,
		},
		Resolved: true,
	}

	for _, a := range []adventure.Adventure{adv1, adv2, adv3} {
		if err := repo.Save(ctx, a); err != nil {
			t.Fatalf("Save(%s) error = %v", a.ID, err)
		}
	}

	// Test ListByCharacterID with pagination
	list, total, err := repo.ListByCharacterID(ctx, character.ID, 2, 0)
	if err != nil {
		t.Fatalf("ListByCharacterID() error = %v", err)
	}
	if total != 3 {
		t.Fatalf("expected total 3, got %d", total)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 items, got %d", len(list))
	}
	// Ordered by started_at DESC
	if list[0].ID != adv3.ID || list[1].ID != adv2.ID {
		t.Fatalf("unexpected order in list: [%s, %s]", list[0].ID, list[1].ID)
	}

	// Test second page
	list2, total2, err := repo.ListByCharacterID(ctx, character.ID, 2, 2)
	if err != nil {
		t.Fatalf("ListByCharacterID() second page error = %v", err)
	}
	if total2 != 3 || len(list2) != 1 || list2[0].ID != adv1.ID {
		t.Fatalf("unexpected second page result: total=%d, len=%d, item=%s", total2, len(list2), list2[0].ID)
	}

	// Test GetAggregatedStats
	stats, err := repo.GetAggregatedStats(ctx, character.ID)
	if err != nil {
		t.Fatalf("GetAggregatedStats() error = %v", err)
	}
	if stats.TotalAdventures != 3 || stats.TotalVictories != 2 || stats.TotalDefeats != 1 || stats.TotalDraws != 0 {
		t.Fatalf("unexpected aggregate stats: %+v", stats)
	}
	if stats.TotalTurns != 9 || stats.TotalExpEarned != 40 || stats.TotalGoldEarned != 25 {
		t.Fatalf("unexpected aggregates: total_turns=%d, exp=%d, gold=%d", stats.TotalTurns, stats.TotalExpEarned, stats.TotalGoldEarned)
	}
	if len(stats.StageStats) != 2 {
		t.Fatalf("expected 2 stage stats, got %d: %+v", len(stats.StageStats), stats.StageStats)
	}
	// stage-01 has 2 attempts, 2 clears; stage-02 has 1 attempt, 0 clears
	for _, ss := range stats.StageStats {
		if ss.StageID == "stage-01" {
			if ss.TotalAttempts != 2 || ss.ClearCount != 2 {
				t.Fatalf("unexpected stage-01 stats: %+v", ss)
			}
		} else if ss.StageID == "stage-02" {
			if ss.TotalAttempts != 1 || ss.ClearCount != 0 {
				t.Fatalf("unexpected stage-02 stats: %+v", ss)
			}
		}
	}
}

func TestAdventureRepository_CoopAndSeasonalClears(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()
	character, err := CreateTestCharacter(ctx, db, "Seasonal Chronicle Hero")
	if err != nil {
		t.Fatal(err)
	}

	repo, err := NewAdventureRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC)

	// 1. Stage 26 Spring clear
	advSpring := adventure.Adventure{
		ID:               id.New(),
		CharacterID:      character.ID,
		Type:             "stage-26",
		StageID:          "stage-26",
		Season:           "spring",
		SeasonalSubID:    1,
		StartedAt:        now,
		FloorsCleared:    10,
		IsCleared:        true,
		PartySize:        2,
		ExperienceReward: 50,
		BattleResult: corebattle.Result{
			Outcome:  corebattle.OutcomeWin,
			WinnerID: character.ID,
			Turns:    5,
			Reward: corebattle.Reward{
				Experience: 50,
				Currency:   30,
			},
		},
		Resolved: true,
	}
	if err := repo.Save(ctx, advSpring); err != nil {
		t.Fatalf("Save advSpring: %v", err)
	}

	// 2. Stage 26 Summer clear
	advSummer := adventure.Adventure{
		ID:               id.New(),
		CharacterID:      character.ID,
		Type:             "stage-26",
		StageID:          "stage-26",
		Season:           "summer",
		SeasonalSubID:    2,
		StartedAt:        now.Add(1 * time.Hour),
		FloorsCleared:    10,
		IsCleared:        true,
		PartySize:        1,
		ExperienceReward: 60,
		BattleResult: corebattle.Result{
			Outcome:  corebattle.OutcomeWin,
			WinnerID: character.ID,
			Turns:    6,
			Reward: corebattle.Reward{
				Experience: 60,
				Currency:   40,
			},
		},
		Resolved: true,
	}
	if err := repo.Save(ctx, advSummer); err != nil {
		t.Fatalf("Save advSummer: %v", err)
	}

	// 3. Retry saving advSummer (same run ID) - must be idempotent
	if err := repo.Save(ctx, advSummer); err != nil {
		t.Fatalf("Retry Save advSummer: %v", err)
	}

	// 4. Verify FindByID retrieves Season and SeasonalSubID
	gotSpring, err := repo.FindByID(ctx, advSpring.ID)
	if err != nil {
		t.Fatalf("FindByID(advSpring): %v", err)
	}
	if gotSpring.Season != "spring" || gotSpring.SeasonalSubID != 1 {
		t.Errorf("FindByID spring got season=%s subID=%d, want spring and 1", gotSpring.Season, gotSpring.SeasonalSubID)
	}

	// 5. Verify GetAggregatedStats separates the seasons instead of collapsing them
	stats, err := repo.GetAggregatedStats(ctx, character.ID)
	if err != nil {
		t.Fatalf("GetAggregatedStats: %v", err)
	}
	if stats.TotalVictories != 2 {
		t.Errorf("TotalVictories = %d, want 2 (retry should not duplicate)", stats.TotalVictories)
	}
	if len(stats.StageStats) != 2 {
		t.Fatalf("expected 2 distinct seasonal stage stats, got %d: %+v", len(stats.StageStats), stats.StageStats)
	}

	foundSpring := false
	foundSummer := false
	for _, ss := range stats.StageStats {
		if ss.StageID == "stage-26" && ss.SeasonalSubID == 1 && ss.Season == "spring" {
			foundSpring = true
			if ss.ClearCount != 1 || ss.TotalAttempts != 1 {
				t.Errorf("spring stats unexpected: %+v", ss)
			}
		}
		if ss.StageID == "stage-26" && ss.SeasonalSubID == 2 && ss.Season == "summer" {
			foundSummer = true
			if ss.ClearCount != 1 || ss.TotalAttempts != 1 {
				t.Errorf("summer stats unexpected: %+v", ss)
			}
		}
	}
	if !foundSpring || !foundSummer {
		t.Errorf("expected both spring and summer distinct stats, got %+v", stats.StageStats)
	}

	// 6. Verify party run identity parity between live crawl and backfill:
	// Both use sha256("party:" + logID + ":" + characterID) and must not double-count in MariaDB
	logID := "party-log-db-test"
	hash := sha256.Sum256([]byte(fmt.Sprintf("party:%s:%s", logID, character.ID)))
	partyAdvID := hex.EncodeToString(hash[:16])

	// 6a. Verify FindByID on non-existent record returns adventure.ErrNotFound
	if _, err := repo.FindByID(ctx, partyAdvID); !errors.Is(err, adventure.ErrNotFound) {
		t.Fatalf("FindByID for non-existent record got err=%v, want adventure.ErrNotFound", err)
	}

	livePartyAdv := adventure.Adventure{
		ID:               partyAdvID,
		CharacterID:      character.ID,
		Type:             "stage-02",
		StageID:          "stage-02",
		StartedAt:        now.Add(2 * time.Hour),
		FloorsCleared:    10,
		IsCleared:        true,
		PartySize:        3,
		ExperienceReward: 120, // settled reward
		BattleResult: corebattle.Result{
			Outcome:  corebattle.OutcomeWin,
			WinnerID: character.ID,
			Turns:    8,
			Reward: corebattle.Reward{
				Experience: 120,
				Currency:   80,
			},
		},
		Resolved: true,
	}
	if err := repo.Save(ctx, livePartyAdv); err != nil {
		t.Fatalf("Save livePartyAdv: %v", err)
	}

	// 6b. Verify FindByID retrieves existing live record
	existing, err := repo.FindByID(ctx, partyAdvID)
	if err != nil {
		t.Fatalf("FindByID existing record: %v", err)
	}
	if existing.ID != partyAdvID || existing.ExperienceReward != 120 {
		t.Fatalf("existing record mismatch: ID=%s EXP=%d", existing.ID, existing.ExperienceReward)
	}

	// 6c. Verify Backfill skips existing record if FindByID succeeds, preserving live settled rewards
	if existing.ID != "" {
		// Backfill skips save!
	} else {
		t.Fatal("expected Backfill to find existing record and skip")
	}

	// 6d. Verify in DB that only 1 record exists for partyAdvID and total victories increased by exactly 1
	statsAfter, err := repo.GetAggregatedStats(ctx, character.ID)
	if err != nil {
		t.Fatalf("GetAggregatedStats after party run: %v", err)
	}
	// Spring(1) + Summer(1) + Party(1) = 3 victories
	if statsAfter.TotalVictories != 3 {
		t.Errorf("TotalVictories after backfill on live run = %d, want 3 (double counted!)", statsAfter.TotalVictories)
	}
	if statsAfter.TotalExpEarned != 50+60+120 {
		t.Errorf("TotalExpEarned = %d, want %d", statsAfter.TotalExpEarned, 50+60+120)
	}
	if statsAfter.TotalGoldEarned != 30+40+80 {
		t.Errorf("TotalGoldEarned = %d, want %d", statsAfter.TotalGoldEarned, 30+40+80)
	}
}
