package database_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/dungeon"
	"github.com/witchcraze/party2re/internal/id"
)

func TestDungeonRepository_FinalizeExpedition_MultiMemberVitalityAndRewards(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()
	dungeonRepo, err := database.NewDungeonRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	charRepo, err := database.NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	// Create leader and ally
	c1, err := database.CreateTestCharacter(ctx, db, "Party Leader")
	if err != nil {
		t.Fatal(err)
	}
	c2, err := database.CreateTestCharacter(ctx, db, "Party Follower")
	if err != nil {
		t.Fatal(err)
	}

	// Set initial baseline
	c1.Stats.HP = 150
	c1.Stats.MaxHP = 150
	c1.Stats.MP = 50
	c1.Stats.MaxMP = 50
	c1.Money = 1000
	c1.SmallMedals = 0
	if err := charRepo.Update(ctx, c1); err != nil {
		t.Fatal(err)
	}

	c2.Stats.HP = 120
	c2.Stats.MaxHP = 120
	c2.Stats.MP = 40
	c2.Stats.MaxMP = 40
	c2.Money = 1000
	c2.SmallMedals = 0
	if err := charRepo.Update(ctx, c2); err != nil {
		t.Fatal(err)
	}

	rec, err := dungeonRepo.GetRecord(ctx, c1.ID)
	if err != nil {
		t.Fatal(err)
	}
	rec.HighestDungeonCleared = 1
	rec.TotalExpeditions = 1

	now := time.Now().UTC()
	expID := id.New()
	history := dungeon.DungeonExpeditionHistory{
		ID:            expID,
		CharacterID:   c1.ID,
		DungeonID:     "dungeon-01",
		Outcome:       dungeon.StatusCleared,
		FloorsReached: 2,
		ExpReward:     250,
		GoldReward:    400,
		MedalsReward:  3,
		CreatedAt:     now,
	}

	// Expedition members:
	// Leader fell in combat after earning 100 EXP and 150 Gold: CurrentHP = 0, CurrentMP = 15
	// Ally survived earning 250 EXP and 400 Gold: CurrentHP = 75, CurrentMP = 22
	members := []dungeon.ExpeditionMember{
		{
			CharacterID:     c1.ID,
			Name:            c1.Name,
			Level:           c1.Level,
			CurrentHP:       0,
			MaxHP:           150,
			CurrentMP:       15,
			MaxMP:           50,
			AccumulatedExp:  100,
			AccumulatedGold: 150,
		},
		{
			CharacterID:     c2.ID,
			Name:            c2.Name,
			Level:           c2.Level,
			CurrentHP:       75,
			MaxHP:           120,
			CurrentMP:       22,
			MaxMP:           40,
			AccumulatedExp:  250,
			AccumulatedGold: 400,
		},
	}

	// Save active expedition
	active := dungeon.ActiveExpedition{
		ID:             expID,
		CharacterID:    c1.ID,
		DungeonID:      "dungeon-01",
		CurrentFloor:   2,
		CurrentHP:      0,
		TurnsRemaining: 10,
		Status:         dungeon.StatusExploring,
		Members:        members,
		StartedAt:      now.Add(-10 * time.Minute),
		UpdatedAt:      now,
	}
	if err := dungeonRepo.SaveActiveExpedition(ctx, active); err != nil {
		t.Fatalf("SaveActiveExpedition failed: %v", err)
	}

	// FinalizeExpedition
	err = dungeonRepo.FinalizeExpedition(ctx, history, rec, &c1, nil, members...)
	if err != nil {
		t.Fatalf("FinalizeExpedition failed: %v", err)
	}

	// Reload both characters from DB
	reloadedLead, err := charRepo.FindByID(ctx, c1.ID)
	if err != nil {
		t.Fatalf("FindByID for leader failed: %v", err)
	}
	reloadedAlly, err := charRepo.FindByID(ctx, c2.ID)
	if err != nil {
		t.Fatalf("FindByID for ally failed: %v", err)
	}

	// Leader check:
	// Fallen character MUST have 1 HP floor applied by shared combat settlement!
	if reloadedLead.Stats.HP != 1 {
		t.Errorf("expected fallen leader HP floor to be 1, got %d", reloadedLead.Stats.HP)
	}
	if reloadedLead.Stats.MP != 15 {
		t.Errorf("expected leader MP 15, got %d", reloadedLead.Stats.MP)
	}
	if reloadedLead.Money != 1150 {
		t.Errorf("expected leader wallet 1150 (1000 + 150), got %d", reloadedLead.Money)
	}
	if reloadedLead.SmallMedals != 3 {
		t.Errorf("expected leader small medals 3, got %d", reloadedLead.SmallMedals)
	}
	if reloadedLead.Experience < 100 {
		t.Errorf("expected leader EXP to increase by at least 100, got %d", reloadedLead.Experience)
	}

	// Ally check:
	// Surviving character retains HP 75 and MP 22!
	if reloadedAlly.Stats.HP != 75 {
		t.Errorf("expected ally HP 75, got %d", reloadedAlly.Stats.HP)
	}
	if reloadedAlly.Stats.MP != 22 {
		t.Errorf("expected ally MP 22, got %d", reloadedAlly.Stats.MP)
	}
	if reloadedAlly.Money != 1400 {
		t.Errorf("expected ally wallet 1400 (1000 + 400), got %d", reloadedAlly.Money)
	}
	if reloadedAlly.SmallMedals != 0 {
		t.Errorf("expected ally small medals to remain 0 (leader only), got %d", reloadedAlly.SmallMedals)
	}
	if reloadedAlly.Experience < 250 {
		t.Errorf("expected ally EXP to increase by at least 250, got %d", reloadedAlly.Experience)
	}

	// Active expedition must be cleared
	activeCheck, err := dungeonRepo.GetActiveExpedition(ctx, c1.ID)
	if err != nil {
		t.Fatalf("GetActiveExpedition error: %v", err)
	}
	if activeCheck != nil {
		t.Errorf("expected active expedition to be deleted, got %+v", activeCheck)
	}
}

func TestDungeonRepository_FinalizeExpedition_PreservesConcurrentWalletUpdates(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()
	dungeonRepo, err := database.NewDungeonRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	charRepo, err := database.NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	c1, err := database.CreateTestCharacter(ctx, db, "Concurrent Leader")
	if err != nil {
		t.Fatal(err)
	}
	c2, err := database.CreateTestCharacter(ctx, db, "Concurrent Ally")
	if err != nil {
		t.Fatal(err)
	}

	c1.Money = 2000
	_ = charRepo.Update(ctx, c1)
	c2.Money = 2000
	_ = charRepo.Update(ctx, c2)

	// In the background, ally receives a bank deposit or trade reward (+600 money)
	if _, err := db.ExecContext(ctx, "UPDATE characters SET money = money + 600 WHERE id = ?", c2.ID); err != nil {
		t.Fatalf("Update money failed: %v", err)
	}

	rec, _ := dungeonRepo.GetRecord(ctx, c1.ID)
	expID := id.New()
	history := dungeon.DungeonExpeditionHistory{
		ID:          expID,
		CharacterID: c1.ID,
		DungeonID:   "dungeon-01",
		Outcome:     dungeon.StatusCleared,
		GoldReward:  500,
		CreatedAt:   time.Now().UTC(),
	}

	members := []dungeon.ExpeditionMember{
		{CharacterID: c1.ID, CurrentHP: 100, MaxHP: 100, AccumulatedGold: 500},
		{CharacterID: c2.ID, CurrentHP: 80, MaxHP: 80, AccumulatedGold: 500},
	}

	// FinalizeExpedition with 500 GoldReward
	err = dungeonRepo.FinalizeExpedition(ctx, history, rec, &c1, nil, members...)
	if err != nil {
		t.Fatalf("FinalizeExpedition failed: %v", err)
	}

	// Ally must have 2000 (initial) + 600 (concurrent update) + 500 (expedition reward) = 3100!
	reloadedAlly, err := charRepo.FindByID(ctx, c2.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reloadedAlly.Money != 3100 {
		t.Errorf("expected ally wallet 3100, got %d (lost concurrent update)", reloadedAlly.Money)
	}
}

func TestDungeonRepository_FinalizeExpedition_RollbackOnMemberFailure(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()
	dungeonRepo, err := database.NewDungeonRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	charRepo, err := database.NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	c1, err := database.CreateTestCharacter(ctx, db, "Rollback Leader")
	if err != nil {
		t.Fatal(err)
	}
	c1.Stats.HP = 100
	c1.Money = 5000
	_ = charRepo.Update(ctx, c1)

	rec, _ := dungeonRepo.GetRecord(ctx, c1.ID)
	expID := id.New()
	history := dungeon.DungeonExpeditionHistory{
		ID:          expID,
		CharacterID: c1.ID,
		DungeonID:   "dungeon-01",
		Outcome:     dungeon.StatusCleared,
		GoldReward:  1000,
		CreatedAt:   time.Now().UTC(),
	}

	// Non-existent member ID
	invalidMemberID := fmt.Sprintf("non-existent-%s", id.New())
	members := []dungeon.ExpeditionMember{
		{CharacterID: c1.ID, CurrentHP: 50, MaxHP: 100},
		{CharacterID: invalidMemberID, CurrentHP: 50, MaxHP: 100},
	}

	err = dungeonRepo.FinalizeExpedition(ctx, history, rec, &c1, nil, members...)
	if err == nil {
		t.Fatal("expected FinalizeExpedition to fail with invalid member")
	}

	// Leader must be completely unchanged
	reloadedLead, err := charRepo.FindByID(ctx, c1.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reloadedLead.Stats.HP != 100 {
		t.Errorf("expected leader HP unchanged (100), got %d", reloadedLead.Stats.HP)
	}
	if reloadedLead.Money != 5000 {
		t.Errorf("expected leader Money unchanged (5000), got %d", reloadedLead.Money)
	}

	// History must NOT exist
	histories, err := dungeonRepo.GetHistory(ctx, c1.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range histories {
		if h.ID == expID {
			t.Errorf("expected history record %s to be rolled back, but found it", expID)
		}
	}
}

// Suppress unused import warning for corecharacter
var _ = corecharacter.Character{}
