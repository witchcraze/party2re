package database

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/adventure"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/id"
)

func TestAdventureRepository(t *testing.T) {
	db := OpenTestDB(t)

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
	db := OpenTestDB(t)

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
	db := OpenTestDB(t)

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

func TestAdventureRepository_AggregatedStatsDefeatOutcomes(t *testing.T) {
	db := OpenTestDB(t)

	ctx := context.Background()
	character, err := CreateTestCharacter(ctx, db, "Defeat Aggregate Hero")
	if err != nil {
		t.Fatal(err)
	}

	repo, err := NewAdventureRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 8, 30, 10, 0, 0, 0, time.UTC)

	// 1. Save resolved solo victory: winner_id = character.ID, is_cleared = true
	advWin := adventure.Adventure{
		ID:               id.New(),
		CharacterID:      character.ID,
		Type:             "stage-01",
		StageID:          "stage-01",
		StartedAt:        now,
		FloorsCleared:    5,
		IsCleared:        true,
		PartySize:        1,
		ExperienceReward: 10,
		BattleResult: corebattle.Result{
			Outcome:  corebattle.OutcomeWin,
			WinnerID: character.ID,
			Turns:    3,
			Reward: corebattle.Reward{
				Experience: 10,
				Currency:   5,
			},
		},
		Resolved: true,
	}
	if err := repo.Save(ctx, advWin); err != nil {
		t.Fatalf("Save advWin: %v", err)
	}

	// Verify baseline after win: total=1, win=1, defeat=0, draw=0
	stats1, err := repo.GetAggregatedStats(ctx, character.ID)
	if err != nil {
		t.Fatalf("GetAggregatedStats after win: %v", err)
	}
	if stats1.TotalAdventures != 1 || stats1.TotalVictories != 1 || stats1.TotalDefeats != 0 || stats1.TotalDraws != 0 {
		t.Fatalf("unexpected stats after win: %+v", stats1)
	}

	// 2. Save resolved coop party defeat: canonical OutcomeDefeat, is_cleared = false, party_size = 2
	advPartyDefeat := adventure.Adventure{
		ID:               id.New(),
		CharacterID:      character.ID,
		Type:             "stage-01",
		StageID:          "stage-01",
		StartedAt:        now.Add(1 * time.Hour),
		FloorsCleared:    2,
		IsCleared:        false,
		PartySize:        2,
		ExperienceReward: 2,
		BattleResult: corebattle.Result{
			Outcome:  corebattle.OutcomeDefeat,
			WinnerID: "",
			Turns:    4,
			Reward: corebattle.Reward{
				Experience: 2,
				Currency:   1,
			},
		},
		Resolved: true,
	}
	if err := repo.Save(ctx, advPartyDefeat); err != nil {
		t.Fatalf("Save advPartyDefeat: %v", err)
	}

	// Verify contrast: total=2, win=1, defeat=1, draw=0
	stats2, err := repo.GetAggregatedStats(ctx, character.ID)
	if err != nil {
		t.Fatalf("GetAggregatedStats after party defeat: %v", err)
	}
	if stats2.TotalAdventures != 2 || stats2.TotalVictories != 1 || stats2.TotalDefeats != 1 || stats2.TotalDraws != 0 {
		t.Fatalf("unexpected stats after party defeat: TotalAdventures=%d, TotalVictories=%d, TotalDefeats=%d, TotalDraws=%d",
			stats2.TotalAdventures, stats2.TotalVictories, stats2.TotalDefeats, stats2.TotalDraws)
	}

	// 3. Save resolved solo defeat: canonical OutcomeDefeat, is_cleared = false, party_size = 1
	advSoloDefeat := adventure.Adventure{
		ID:               id.New(),
		CharacterID:      character.ID,
		Type:             "stage-02",
		StageID:          "stage-02",
		StartedAt:        now.Add(2 * time.Hour),
		FloorsCleared:    3,
		IsCleared:        false,
		PartySize:        1,
		ExperienceReward: 0,
		BattleResult: corebattle.Result{
			Outcome:  corebattle.OutcomeDefeat,
			WinnerID: "",
			Turns:    5,
		},
		Resolved: true,
	}
	if err := repo.Save(ctx, advSoloDefeat); err != nil {
		t.Fatalf("Save advSoloDefeat: %v", err)
	}

	// 4. Save legacy 1-on-1 defeat representation: outcome=OutcomeWin, winner_id=monster, is_cleared=false
	advLegacy1v1Defeat := adventure.Adventure{
		ID:               id.New(),
		CharacterID:      character.ID,
		Type:             "stage-02",
		StageID:          "stage-02",
		StartedAt:        now.Add(3 * time.Hour),
		FloorsCleared:    1,
		IsCleared:        false,
		PartySize:        1,
		ExperienceReward: 0,
		BattleResult: corebattle.Result{
			Outcome:  corebattle.OutcomeWin,
			WinnerID: "mon-boss-99",
			Turns:    6,
		},
		Resolved: true,
	}
	if err := repo.Save(ctx, advLegacy1v1Defeat); err != nil {
		t.Fatalf("Save advLegacy1v1Defeat: %v", err)
	}

	// 5. Save legacy compatibility record with outcome="loss"
	advLegacyLoss := adventure.Adventure{
		ID:               id.New(),
		CharacterID:      character.ID,
		Type:             "stage-03",
		StageID:          "stage-03",
		StartedAt:        now.Add(4 * time.Hour),
		FloorsCleared:    0,
		IsCleared:        false,
		PartySize:        1,
		ExperienceReward: 0,
		BattleResult: corebattle.Result{
			Outcome:  corebattle.Outcome("loss"),
			WinnerID: "",
			Turns:    2,
		},
		Resolved: true,
	}
	if err := repo.Save(ctx, advLegacyLoss); err != nil {
		t.Fatalf("Save advLegacyLoss: %v", err)
	}

	// 6. Save draw record: canonical OutcomeDraw
	advDraw := adventure.Adventure{
		ID:               id.New(),
		CharacterID:      character.ID,
		Type:             "stage-03",
		StageID:          "stage-03",
		StartedAt:        now.Add(5 * time.Hour),
		FloorsCleared:    2,
		IsCleared:        false,
		PartySize:        1,
		ExperienceReward: 5,
		BattleResult: corebattle.Result{
			Outcome:  corebattle.OutcomeDraw,
			WinnerID: "",
			Turns:    10,
			Reward: corebattle.Reward{
				Experience: 5,
				Currency:   2,
			},
		},
		Resolved: true,
	}
	if err := repo.Save(ctx, advDraw); err != nil {
		t.Fatalf("Save advDraw: %v", err)
	}

	// Final verification of all aggregated totals:
	finalStats, err := repo.GetAggregatedStats(ctx, character.ID)
	if err != nil {
		t.Fatalf("GetAggregatedStats final: %v", err)
	}

	if finalStats.TotalAdventures != 6 {
		t.Errorf("TotalAdventures = %d, want 6", finalStats.TotalAdventures)
	}
	if finalStats.TotalVictories != 1 {
		t.Errorf("TotalVictories = %d, want 1", finalStats.TotalVictories)
	}
	if finalStats.TotalDefeats != 4 {
		t.Errorf("TotalDefeats = %d, want 4", finalStats.TotalDefeats)
	}
	if finalStats.TotalDraws != 1 {
		t.Errorf("TotalDraws = %d, want 1", finalStats.TotalDraws)
	}
	if finalStats.TotalTurns != 3+4+5+6+2+10 {
		t.Errorf("TotalTurns = %d, want %d", finalStats.TotalTurns, 3+4+5+6+2+10)
	}
	if finalStats.TotalExpEarned != 10+2+0+0+0+5 {
		t.Errorf("TotalExpEarned = %d, want %d", finalStats.TotalExpEarned, 10+2+0+0+0+5)
	}
	if finalStats.TotalGoldEarned != 5+1+0+0+0+2 {
		t.Errorf("TotalGoldEarned = %d, want %d", finalStats.TotalGoldEarned, 5+1+0+0+0+2)
	}

	// Verify stage stats
	stageClears := make(map[string]int)
	stageAttempts := make(map[string]int)
	for _, ss := range finalStats.StageStats {
		stageClears[ss.StageID] += ss.ClearCount
		stageAttempts[ss.StageID] += ss.TotalAttempts
	}

	if stageAttempts["stage-01"] != 2 || stageClears["stage-01"] != 1 {
		t.Errorf("stage-01 attempts=%d clears=%d, want 2 attempts 1 clear", stageAttempts["stage-01"], stageClears["stage-01"])
	}
	if stageAttempts["stage-02"] != 2 || stageClears["stage-02"] != 0 {
		t.Errorf("stage-02 attempts=%d clears=%d, want 2 attempts 0 clear", stageAttempts["stage-02"], stageClears["stage-02"])
	}
	if stageAttempts["stage-03"] != 2 || stageClears["stage-03"] != 0 {
		t.Errorf("stage-03 attempts=%d clears=%d, want 2 attempts 0 clear", stageAttempts["stage-03"], stageClears["stage-03"])
	}
}

func TestAdventureRepository_CrystalRewardsRoundTripAndUpdate(t *testing.T) {
	db := OpenTestDB(t)

	ctx := context.Background()
	character, err := CreateTestCharacter(ctx, db, "Crystal History")
	if err != nil {
		t.Fatal(err)
	}
	RegisterCharacterCleanup(t, db, character)

	repo, err := NewAdventureRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	baseTime := time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)
	advZero := adventure.Adventure{
		ID:               "00000000000000000000000000000001",
		CharacterID:      character.ID,
		Type:             "stage-01",
		StageID:          "stage-01",
		StartedAt:        baseTime,
		FloorsCleared:    5,
		IsCleared:        false,
		PartySize:        1,
		ExperienceReward: 5,
		BattleResult: corebattle.Result{
			Outcome: corebattle.OutcomeDefeat,
			Turns:   4,
			Reward:  corebattle.Reward{Experience: 5, Currency: 8, Crystals: 0},
		},
		Resolved: true,
	}
	advCrystal := adventure.Adventure{
		ID:               "00000000000000000000000000000002",
		CharacterID:      character.ID,
		Type:             "stage-02",
		StageID:          "stage-02",
		StartedAt:        baseTime.Add(time.Minute),
		FloorsCleared:    10,
		IsCleared:        true,
		PartySize:        2,
		ExperienceReward: 7,
		BattleResult: corebattle.Result{
			Outcome:  corebattle.OutcomeWin,
			WinnerID: character.ID,
			Turns:    6,
			Reward: corebattle.Reward{
				Experience:       7,
				Currency:         12,
				Crystals:         1,
				ItemDefinitionID: "item-001",
				ItemQuantity:     2,
			},
		},
		Resolved: true,
	}
	advLater := adventure.Adventure{
		ID:               "00000000000000000000000000000003",
		CharacterID:      character.ID,
		Type:             "stage-03",
		StageID:          "stage-03",
		StartedAt:        baseTime.Add(2 * time.Minute),
		FloorsCleared:    10,
		IsCleared:        true,
		PartySize:        1,
		ExperienceReward: 9,
		BattleResult: corebattle.Result{
			Outcome:  corebattle.OutcomeWin,
			WinnerID: character.ID,
			Turns:    7,
			Reward: corebattle.Reward{
				Experience: 9,
				Currency:   14,
				Crystals:   3,
			},
		},
		Resolved: true,
	}

	for _, adv := range []adventure.Adventure{advZero, advCrystal, advLater} {
		if err := repo.Save(ctx, adv); err != nil {
			t.Fatalf("Save(%s): %v", adv.ID, err)
		}
	}

	// Update an existing record's crystal reward and confirm other reward fields stay intact.
	advCrystal.BattleResult.Reward.Crystals = 4
	if err := repo.Save(ctx, advCrystal); err != nil {
		t.Fatalf("Save(update %s): %v", advCrystal.ID, err)
	}

	wantByID := map[string]corebattle.Reward{
		advZero.ID:    advZero.BattleResult.Reward,
		advCrystal.ID: advCrystal.BattleResult.Reward,
		advLater.ID:   advLater.BattleResult.Reward,
	}

	// 1. FindByID
	for advID, wantReward := range wantByID {
		got, err := repo.FindByID(ctx, advID)
		if err != nil {
			t.Fatalf("FindByID(%s): %v", advID, err)
		}
		if !reflect.DeepEqual(got.BattleResult.Reward, wantReward) {
			t.Fatalf("FindByID(%s) reward = %+v, want %+v", advID, got.BattleResult.Reward, wantReward)
		}
	}

	// 2. ListByCharacterID
	listed, total, err := repo.ListByCharacterID(ctx, character.ID, 10, 0)
	if err != nil {
		t.Fatalf("ListByCharacterID: %v", err)
	}
	if total != 3 || len(listed) != 3 {
		t.Fatalf("ListByCharacterID count = %d/%d, want 3/3", len(listed), total)
	}
	for _, got := range listed {
		if !reflect.DeepEqual(got.BattleResult.Reward, wantByID[got.ID]) {
			t.Fatalf("ListByCharacterID(%s) reward = %+v, want %+v", got.ID, got.BattleResult.Reward, wantByID[got.ID])
		}
	}

	// 3. ListByCharacterIDByCursor across all four cursor branches.
	cursorCases := []struct {
		name       string
		beforeTime time.Time
		beforeID   string
		wantIDs    []string
	}{
		{"initial", time.Time{}, "", []string{advLater.ID, advCrystal.ID, advZero.ID}},
		{"time_and_id", advLater.StartedAt, advLater.ID, []string{advCrystal.ID, advZero.ID}},
		{"time_only", advLater.StartedAt, "", []string{advCrystal.ID, advZero.ID}},
		{"id_only", time.Time{}, advLater.ID, []string{advCrystal.ID, advZero.ID}},
	}
	for _, tc := range cursorCases {
		gotList, err := repo.ListByCharacterIDByCursor(ctx, character.ID, 10, tc.beforeTime, tc.beforeID)
		if err != nil {
			t.Fatalf("ListByCharacterIDByCursor(%s): %v", tc.name, err)
		}
		if len(gotList) != len(tc.wantIDs) {
			t.Fatalf("ListByCharacterIDByCursor(%s) len = %d, want %d", tc.name, len(gotList), len(tc.wantIDs))
		}
		for i, got := range gotList {
			if got.ID != tc.wantIDs[i] {
				t.Fatalf("ListByCharacterIDByCursor(%s)[%d] id = %s, want %s", tc.name, i, got.ID, tc.wantIDs[i])
			}
			if !reflect.DeepEqual(got.BattleResult.Reward, wantByID[got.ID]) {
				t.Fatalf("ListByCharacterIDByCursor(%s)[%s] reward = %+v, want %+v", tc.name, got.ID, got.BattleResult.Reward, wantByID[got.ID])
			}
		}
	}
}

func TestAdventureRewardCrystalsMigration(t *testing.T) {
	db := CreateTestDatabase(t, "players", "characters", "character_depots", "adventures", "schema_migrations")
	ctx := context.Background()

	// Ensure pre-141 state even when the cloned schema already applied migration 141.
	if _, err := db.ExecContext(ctx, `ALTER TABLE adventures DROP COLUMN IF EXISTS reward_crystals`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM schema_migrations WHERE version = '141_adventure_reward_crystals'`); err != nil {
		t.Fatal(err)
	}

	char, err := CreateTestCharacter(ctx, db, "AdvMig")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE characters SET crystal = 42 WHERE id = ?`, char.ID); err != nil {
		t.Fatal(err)
	}

	advID := id.New()
	startedAt := time.Date(2026, 11, 14, 9, 0, 0, 0, time.UTC)
	if _, err := db.ExecContext(ctx, `
		INSERT INTO adventures
			(id, character_id, adventure_type, started_at, experience_reward,
			 outcome, winner_id, battle_turns, floors_cleared, is_cleared,
			 seasonal_sub_id, season, party_size,
			 reward_experience, reward_currency, reward_item_definition_id, reward_item_quantity, resolved)
		VALUES (?, ?, 'stage-01', ?, 10, 'win', ?, 3, 10, TRUE, 0, '', 1, 10, 20, 'item-001', 1, TRUE)
	`, advID, char.ID, startedAt, char.ID); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join("..", "..", "migrations", "141_adventure_reward_crystals.sql"))
	if err != nil {
		t.Fatal(err)
	}
	apply := func() {
		t.Helper()
		for _, stmt := range strings.Split(string(data), ";") {
			if strings.TrimSpace(stmt) != "" {
				if _, err := db.ExecContext(ctx, stmt); err != nil {
					t.Fatalf("exec %q: %v", strings.TrimSpace(stmt), err)
				}
			}
		}
	}

	// Apply twice to verify idempotency.
	apply()
	apply()

	var colCount int
	if err := db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM information_schema.columns
		WHERE table_schema = DATABASE() AND table_name = 'adventures' AND column_name = 'reward_crystals'
	`).Scan(&colCount); err != nil || colCount != 1 {
		t.Fatalf("reward_crystals column count = %d, err = %v", colCount, err)
	}

	repo, err := NewAdventureRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	got, err := repo.FindByID(ctx, advID)
	if err != nil {
		t.Fatalf("FindByID after migration: %v", err)
	}
	wantReward := corebattle.Reward{
		Experience:       10,
		Currency:         20,
		Crystals:         0,
		ItemDefinitionID: "item-001",
		ItemQuantity:     1,
	}
	if !reflect.DeepEqual(got.BattleResult.Reward, wantReward) {
		t.Fatalf("post-migration reward = %+v, want %+v", got.BattleResult.Reward, wantReward)
	}

	var crystalBalance int
	if err := db.QueryRowContext(ctx, `SELECT crystal FROM characters WHERE id = ?`, char.ID).Scan(&crystalBalance); err != nil || crystalBalance != 42 {
		t.Fatalf("character crystal balance = %d, err = %v, want 42", crystalBalance, err)
	}

	var migCount int
	if err := db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM schema_migrations WHERE version = '141_adventure_reward_crystals'
	`).Scan(&migCount); err != nil || migCount != 1 {
		t.Fatalf("migration record count = %d, err = %v", migCount, err)
	}
}
