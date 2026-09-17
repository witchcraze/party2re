package database_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/challenge"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/depot"
)

func TestChallengeRepository(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()
	char, err := database.CreateTestCharacter(ctx, db, "Challenge Repo Hero")
	if err != nil {
		t.Fatal(err)
	}

	repo, err := database.NewChallengeRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	sessionID := fmt.Sprintf("chal_sess_%016x", now.UnixNano())
	s := challenge.ChallengeSession{
		ID:                 sessionID,
		CharacterID:        char.ID,
		TierID:             "novice",
		CurrentRound:       1,
		CharacterCurrentHP: 150,
		AccumulatedExp:     50,
		AccumulatedGold:    30,
		AccumulatedItems:   []string{"potion_minor"},
		Status:             challenge.StatusActive,
		CreatedAt:          now,
		UpdatedAt:          now,
	}

	// 1. SaveSession
	if err := repo.SaveSession(ctx, s); err != nil {
		t.Fatalf("SaveSession failed: %v", err)
	}

	// 2. FindSessionByID
	fetched, err := repo.FindSessionByID(ctx, sessionID)
	if err != nil {
		t.Fatalf("FindSessionByID failed: %v", err)
	}
	if fetched.ID != sessionID || fetched.CurrentRound != 1 || len(fetched.AccumulatedItems) != 1 {
		t.Errorf("unexpected fetched session: %#v", fetched)
	}

	// 3. FindActiveSessionByCharacter
	active, err := repo.FindActiveSessionByCharacter(ctx, char.ID)
	if err != nil || active == nil {
		t.Fatalf("FindActiveSessionByCharacter failed: %v", err)
	}
	if active.ID != sessionID {
		t.Errorf("expected active session ID %s, got %s", sessionID, active.ID)
	}

	// 4. UpdateSession
	s.CurrentRound = 3
	s.AccumulatedExp = 120
	if err := repo.UpdateSession(ctx, s); err != nil {
		t.Fatalf("UpdateSession failed: %v", err)
	}

	// 5. FinalizeSession
	s.Status = challenge.StatusClaimed
	if err := repo.FinalizeSession(ctx, s, 120, 80, []string{"potion_minor"}, 2); err != nil {
		t.Fatalf("FinalizeSession failed: %v", err)
	}

	charRepo, err := database.NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	updatedChar, err := charRepo.FindByID(ctx, char.ID)
	if err != nil {
		t.Fatalf("FindByID character failed: %v", err)
	}
	if updatedChar.Level <= 1 {
		t.Errorf("expected level up from 120 EXP, got level %d", updatedChar.Level)
	}
	if updatedChar.Money != char.Money+80 {
		t.Errorf("expected money %d, got %d", char.Money+80, updatedChar.Money)
	}

	invRepo, err := database.NewInventoryRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	inv, err := invRepo.FindByCharacterID(ctx, char.ID)
	if err != nil {
		t.Fatalf("FindByCharacterID failed: %v", err)
	}
	if len(inv.Items) != 1 || inv.Items[0].DefinitionID != "potion_minor" {
		t.Fatalf("expected 1 potion_minor in inventory, got %#v", inv.Items)
	}
	if len(inv.Items[0].ID) != 32 {
		t.Errorf("expected 32-char hex ID from id.New(), got length %d (%s)", len(inv.Items[0].ID), inv.Items[0].ID)
	}

	// 6. FindRecord
	rec, err := repo.FindRecord(ctx, char.ID, "novice")
	if err != nil || rec == nil {
		t.Fatalf("FindRecord failed: %v", err)
	}
	if rec.HighestRound != 2 || rec.TotalVictories != 2 || rec.TotalAttempts != 1 {
		t.Errorf("unexpected record: %#v", rec)
	}

	// 7. GetLeaderboard
	leaderboard, err := repo.GetLeaderboard(ctx, "novice", 100)
	if err != nil || len(leaderboard) == 0 {
		t.Fatalf("GetLeaderboard failed: %v", err)
	}
	found := false
	for _, entry := range leaderboard {
		if entry.CharacterID == char.ID {
			found = true
			if entry.HighestRound != 2 {
				t.Errorf("unexpected leaderboard streak: %d", entry.HighestRound)
			}
			break
		}
	}
	if !found {
		t.Errorf("character not found in challenge leaderboard")
	}

	// 8. SaveRecord
	customRec := challenge.CharacterChallengeRecord{
		CharacterID:    char.ID,
		TierID:         "expert",
		HighestRound:   5,
		TotalAttempts:  2,
		TotalVictories: 5,
		BestClearedAt:  now,
	}
	if err := repo.SaveRecord(ctx, customRec); err != nil {
		t.Fatalf("SaveRecord failed: %v", err)
	}
	foundCustomRec, err := repo.FindRecord(ctx, char.ID, "expert")
	if err != nil || foundCustomRec == nil {
		t.Fatalf("FindRecord for customRec failed: %v", err)
	}
	if foundCustomRec.HighestRound != 5 {
		t.Errorf("expected HighestRound 5, got %d", foundCustomRec.HighestRound)
	}
}

func TestChallengeRepository_ItemDeliveryAndDepotFallback(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()
	repo, err := database.NewChallengeRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	invRepo, err := database.NewInventoryRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	depotRepo, err := database.NewDepotRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	char, err := database.CreateTestCharacter(ctx, db, "Chal Overflow Hero")
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	sess1 := challenge.ChallengeSession{
		ID:                 fmt.Sprintf("chal_ovf_%016x", now.UnixNano()),
		CharacterID:        char.ID,
		TierID:             "novice",
		CurrentRound:       2,
		CharacterCurrentHP: 100,
		Status:             challenge.StatusClaimed,
		CreatedAt:          now,
		UpdatedAt:          now,
	}

	// 1. Finalize with first item -> goes to empty inventory
	if err := repo.FinalizeSession(ctx, sess1, 50, 20, []string{"item-001"}, 1); err != nil {
		t.Fatalf("FinalizeSession 1 failed: %v", err)
	}

	inv, err := invRepo.FindByCharacterID(ctx, char.ID)
	if err != nil {
		t.Fatalf("FindByCharacterID failed: %v", err)
	}
	if len(inv.Items) != 1 || inv.Items[0].DefinitionID != "item-001" {
		t.Fatalf("expected 1 item in inventory, got: %#v", inv.Items)
	}

	// 2. Finalize with second item -> inventory full, overflows to depot
	sess2 := challenge.ChallengeSession{
		ID:                 fmt.Sprintf("chal_ovf_%016x", now.UnixNano()+1),
		CharacterID:        char.ID,
		TierID:             "novice",
		CurrentRound:       3,
		CharacterCurrentHP: 100,
		Status:             challenge.StatusClaimed,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	if err := repo.FinalizeSession(ctx, sess2, 50, 20, []string{"item-002"}, 2); err != nil {
		t.Fatalf("FinalizeSession 2 overflow failed: %v", err)
	}

	invAfter2, err := invRepo.FindByCharacterID(ctx, char.ID)
	if err != nil {
		t.Fatalf("FindByCharacterID after 2 failed: %v", err)
	}
	if len(invAfter2.Items) != 1 {
		t.Fatalf("inventory must remain at capacity 1, got %d items", len(invAfter2.Items))
	}

	var dep depot.Depot
	dep, err = depotRepo.FindByCharacterID(ctx, char.ID)
	if err != nil {
		t.Fatalf("FindByCharacterID depot failed: %v", err)
	}
	if len(dep.Items) != 1 || dep.Items[0].DefinitionID != "item-002" {
		t.Fatalf("expected 1 overflow item in depot, got: %#v", dep.Items)
	}

	// 3. Fill depot to capacity, then finalize third session -> treated as lost drop
	for i := len(dep.Items); i < dep.Capacity; i++ {
		fillItem, _ := coreitem.NewInstance(fmt.Sprintf("filler_%d", i), 1)
		_ = dep.AddItem(fillItem, false)
	}
	if err := depotRepo.Save(ctx, dep); err != nil {
		t.Fatalf("failed to fill depot: %v", err)
	}

	sess3 := challenge.ChallengeSession{
		ID:                 fmt.Sprintf("chal_ovf_%016x", now.UnixNano()+2),
		CharacterID:        char.ID,
		TierID:             "novice",
		CurrentRound:       4,
		CharacterCurrentHP: 100,
		Status:             challenge.StatusClaimed,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	if err := repo.FinalizeSession(ctx, sess3, 50, 20, []string{"item-003"}, 3); err != nil {
		t.Fatalf("FinalizeSession with full depot should succeed as lost drop: %v", err)
	}

	depAfter3, err := depotRepo.FindByCharacterID(ctx, char.ID)
	if err != nil {
		t.Fatalf("FindByCharacterID depot after 3 failed: %v", err)
	}
	if len(depAfter3.Items) != dep.Capacity {
		t.Fatalf("depot items changed, expected %d, got %d", dep.Capacity, len(depAfter3.Items))
	}
}
