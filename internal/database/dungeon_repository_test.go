package database_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/depot"
	"github.com/witchcraze/party2re/internal/dungeon"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/testutil"
)

func TestDungeonRepository(t *testing.T) {
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

	char, err := database.CreateTestCharacter(ctx, db, "Dungeon Explorer")
	if err != nil {
		t.Fatal(err)
	}

	// 1. GetRecord (initial empty record)
	rec, err := dungeonRepo.GetRecord(ctx, char.ID)
	if err != nil {
		t.Fatalf("GetRecord failed: %v", err)
	}
	if rec.CharacterID != char.ID || rec.HighestDungeonCleared != 0 {
		t.Errorf("unexpected initial record: %#v", rec)
	}

	// 2. SaveActiveExpedition & GetActiveExpedition
	now := time.Now().UTC()
	expID := fmt.Sprintf("exp_%016x", now.UnixNano())
	exp := dungeon.ActiveExpedition{
		ID:               expID,
		CharacterID:      char.ID,
		DungeonID:        "dungeon-01",
		CurrentFloor:     1,
		PosX:             2,
		PosY:             0,
		CurrentHP:        char.Stats.HP,
		TurnsRemaining:   20,
		AccumulatedExp:   150,
		AccumulatedGold:  300,
		AccumulatedItems: []string{"potion", "ether"},
		Status:           dungeon.StatusExploring,
		StartedAt:        now,
		UpdatedAt:        now,
	}

	if err := dungeonRepo.SaveActiveExpedition(ctx, exp); err != nil {
		t.Fatalf("SaveActiveExpedition failed: %v", err)
	}

	fetchedExp, err := dungeonRepo.GetActiveExpedition(ctx, char.ID)
	if err != nil || fetchedExp == nil {
		t.Fatalf("GetActiveExpedition failed: %v", err)
	}
	if fetchedExp.ID != expID || fetchedExp.AccumulatedGold != 300 || len(fetchedExp.AccumulatedItems) != 2 {
		t.Errorf("unexpected fetched expedition: %#v", fetchedExp)
	}

	// 3. FinalizeExpedition
	rec.HighestDungeonCleared = 1
	rec.TotalExpeditions = 1
	rec.TotalFloorsCleared = 2
	rec.TotalChestsOpened = 1
	rec.TotalMonstersSlain = 3

	histID := expID
	history := dungeon.DungeonExpeditionHistory{
		ID:               histID,
		CharacterID:      char.ID,
		DungeonID:        "dungeon-01",
		FloorsReached:    2,
		Outcome:          dungeon.StatusCleared,
		ExpReward:        450,
		GoldReward:       800,
		ItemsRewardCount: 2,
		CreatedAt:        now,
	}

	rewardItems := []coreitem.Instance{
		{
			ID:               fmt.Sprintf("item_%016x", now.UnixNano()),
			DefinitionID:     "potion",
			Quantity:         1,
			EnhancementLevel: 0,
		},
	}

	err = dungeonRepo.FinalizeExpedition(ctx, history, rec, &char, rewardItems)
	if err != nil {
		t.Fatalf("FinalizeExpedition failed: %v", err)
	}

	// 4. Verify Active Expedition Deleted
	clearedExp, err := dungeonRepo.GetActiveExpedition(ctx, char.ID)
	if err != nil || clearedExp != nil {
		t.Errorf("expected active expedition to be nil after finalize, got %#v", clearedExp)
	}

	// 5. Verify Record Updated
	updatedRec, err := dungeonRepo.GetRecord(ctx, char.ID)
	if err != nil || updatedRec.HighestDungeonCleared != 1 || updatedRec.TotalExpeditions != 1 {
		t.Errorf("unexpected updated record: %#v", updatedRec)
	}

	// 6. Verify History
	histories, err := dungeonRepo.GetHistory(ctx, char.ID, 5)
	if err != nil || len(histories) != 1 {
		t.Fatalf("GetHistory failed: %v, len=%d", err, len(histories))
	}
	if histories[0].Outcome != dungeon.StatusCleared || histories[0].GoldReward != 800 {
		t.Errorf("unexpected history entry: %#v", histories[0])
	}
}

func TestDungeonRepository_ItemDeliveryAndDepotFallback(t *testing.T) {
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
	invRepo, err := database.NewInventoryRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	depotRepo, err := database.NewDepotRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	char, err := database.CreateTestCharacter(ctx, db, "Dungeon Overflow Hero")
	if err != nil {
		t.Fatal(err)
	}

	rec, err := dungeonRepo.GetRecord(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()

	// 1. Finalize with first item -> goes into inventory
	hist1 := dungeon.DungeonExpeditionHistory{
		ID:               fmt.Sprintf("hist_%016x", now.UnixNano()),
		CharacterID:      char.ID,
		DungeonID:        "dungeon-01",
		FloorsReached:    1,
		Outcome:          dungeon.StatusCleared,
		ExpReward:        100,
		GoldReward:       200,
		ItemsRewardCount: 1,
		CreatedAt:        now,
	}
	item1 := []coreitem.Instance{
		{
			ID:           fmt.Sprintf("item_%016x", now.UnixNano()),
			DefinitionID: "potion",
			Quantity:     1,
		},
	}
	if err := dungeonRepo.FinalizeExpedition(ctx, hist1, rec, &char, item1); err != nil {
		t.Fatalf("FinalizeExpedition item1 failed: %v", err)
	}

	inv, err := invRepo.FindByCharacterID(ctx, char.ID)
	if err != nil {
		t.Fatalf("FindByCharacterID failed: %v", err)
	}
	if len(inv.Items) != 1 || inv.Items[0].DefinitionID != "potion" {
		t.Fatalf("expected 1 item in inventory, got: %#v", inv.Items)
	}

	// 2. Finalize with second item -> inventory full, overflows to depot
	hist2 := dungeon.DungeonExpeditionHistory{
		ID:               fmt.Sprintf("hist_%016x", now.UnixNano()+1),
		CharacterID:      char.ID,
		DungeonID:        "dungeon-01",
		FloorsReached:    2,
		Outcome:          dungeon.StatusCleared,
		ExpReward:        100,
		GoldReward:       200,
		ItemsRewardCount: 1,
		CreatedAt:        now,
	}
	item2 := []coreitem.Instance{
		{
			ID:           fmt.Sprintf("item_%016x", now.UnixNano()+1),
			DefinitionID: "ether",
			Quantity:     1,
		},
	}
	if err := dungeonRepo.FinalizeExpedition(ctx, hist2, rec, &char, item2); err != nil {
		t.Fatalf("FinalizeExpedition item2 overflow failed: %v", err)
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
	if len(dep.Items) != 1 || dep.Items[0].DefinitionID != "ether" {
		t.Fatalf("expected 1 overflow item in depot, got: %#v", dep.Items)
	}

	// 3. Fill depot to capacity, then deliver third item -> treated as lost drop
	for i := len(dep.Items); i < dep.Capacity; i++ {
		fillItem, _ := coreitem.NewInstance(fmt.Sprintf("filler_%d", i), 1)
		_ = dep.AddItem(fillItem, false)
	}
	if err := depotRepo.Save(ctx, dep); err != nil {
		t.Fatalf("failed to fill depot: %v", err)
	}

	hist3 := dungeon.DungeonExpeditionHistory{
		ID:               fmt.Sprintf("hist_%016x", now.UnixNano()+2),
		CharacterID:      char.ID,
		DungeonID:        "dungeon-01",
		FloorsReached:    3,
		Outcome:          dungeon.StatusCleared,
		ExpReward:        100,
		GoldReward:       200,
		ItemsRewardCount: 1,
		CreatedAt:        now,
	}
	item3 := []coreitem.Instance{
		{
			ID:           fmt.Sprintf("item_%016x", now.UnixNano()+2),
			DefinitionID: "elixir",
			Quantity:     1,
		},
	}
	if err := dungeonRepo.FinalizeExpedition(ctx, hist3, rec, &char, item3); err != nil {
		t.Fatalf("FinalizeExpedition with full depot should succeed as lost drop: %v", err)
	}

	depAfter3, err := depotRepo.FindByCharacterID(ctx, char.ID)
	if err != nil {
		t.Fatalf("FindByCharacterID depot after 3 failed: %v", err)
	}
	if len(depAfter3.Items) != dep.Capacity {
		t.Fatalf("depot item count changed, expected %d, got %d", dep.Capacity, len(depAfter3.Items))
	}
}

func TestDungeonRepository_IdempotentClaimAndWalletPreservation(t *testing.T) {
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
	invRepo, err := database.NewInventoryRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	depotRepo, err := database.NewDepotRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	char, err := database.CreateTestCharacterWithFunds(ctx, db, "Dungeon Claim Hero", 1000)
	if err != nil {
		t.Fatal(err)
	}
	rec, err := dungeonRepo.GetRecord(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}

	// Simulate an external wallet update (e.g. +500G and +3 SmallMedals) that committed after
	// the dungeon service read its stale character snapshot (`staleChar`).
	staleChar := char
	if _, err := db.ExecContext(ctx, `UPDATE characters SET money = money + 500, small_medals = small_medals + 3 WHERE id = ?`, char.ID); err != nil {
		t.Fatal(err)
	}

	runID := "exp_claim_" + id.New()
	now := time.Now().UTC()
	rec.HighestDungeonCleared = 2
	rec.TotalExpeditions = 1
	rec.TotalFloorsCleared = 3

	hist := dungeon.DungeonExpeditionHistory{
		ID:               runID,
		CharacterID:      char.ID,
		DungeonID:        "dungeon-02",
		FloorsReached:    3,
		Outcome:          dungeon.StatusEscaped,
		ExpReward:        200,
		GoldReward:       400,
		MedalsReward:     2,
		ItemsRewardCount: 1,
		CreatedAt:        now,
	}
	rewardItems := []coreitem.Instance{
		{
			ID:           id.New(),
			DefinitionID: "potion",
			Quantity:     1,
		},
	}

	// Simulate stale caller snapshot that already added rewards onto stale money (1000 + 400 = 1400).
	// FinalizeExpedition MUST reload and lock the latest DB row (1500) and apply +400 -> 1900!
	staleChar.Money += 400
	staleChar.SmallMedals += 2

	if err := dungeonRepo.FinalizeExpedition(ctx, hist, rec, &staleChar, rewardItems); err != nil {
		t.Fatalf("first FinalizeExpedition failed: %v", err)
	}

	// Duplicate claim with the same run ID must return ErrExpeditionAlreadyFinished and roll back / skip duplicate grants
	dupItems := []coreitem.Instance{
		{
			ID:           id.New(),
			DefinitionID: "ether",
			Quantity:     1,
		},
	}
	err = dungeonRepo.FinalizeExpedition(ctx, hist, rec, &staleChar, dupItems)
	if !errors.Is(err, dungeon.ErrExpeditionAlreadyFinished) {
		t.Fatalf("expected ErrExpeditionAlreadyFinished on duplicate run ID, got %v", err)
	}

	// Verify character wallet preserved concurrent +500G and added +400G exactly once (1000 + 500 + 400 = 1900)
	updatedChar, err := charRepo.FindByID(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updatedChar.Money != 1900 {
		t.Errorf("expected money 1900 (preserving concurrent +500 and single +400 reward), got %d", updatedChar.Money)
	}
	if updatedChar.SmallMedals != 5 {
		t.Errorf("expected small medals 5 (3 + 2), got %d", updatedChar.SmallMedals)
	}

	// Verify only 1 item delivered across inventory and depot
	inv, err := invRepo.FindByCharacterID(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}
	dep, err := depotRepo.FindByCharacterID(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(inv.Items)+len(dep.Items) != 1 {
		t.Errorf("expected exactly 1 item delivered, got inv=%d depot=%d", len(inv.Items), len(dep.Items))
	}

	// Verify 1 history and 1 expedition count in record
	histories, err := dungeonRepo.GetHistory(ctx, char.ID, 10)
	if err != nil || len(histories) != 1 {
		t.Fatalf("expected 1 history record, got %d (err: %v)", len(histories), err)
	}
	finalRec, err := dungeonRepo.GetRecord(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}
	if finalRec.TotalExpeditions != 1 || finalRec.TotalFloorsCleared != 3 {
		t.Errorf("expected TotalExpeditions=1, TotalFloorsCleared=3, got %+v", finalRec)
	}
}

func TestDungeonRepository_RollbackOnRewardFailure(t *testing.T) {
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

	// Create character and trigger failure inside deliverRewardItems after history insert & character update
	char, err := database.CreateTestCharacterWithFunds(ctx, db, "Dungeon Rollback Hero", 1000)
	if err != nil {
		t.Fatal(err)
	}
	rec, err := dungeonRepo.GetRecord(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}

	runID := "exp_rb_" + id.New()
	now := time.Now().UTC()
	hist := dungeon.DungeonExpeditionHistory{
		ID:               runID,
		CharacterID:      char.ID,
		DungeonID:        "dungeon-01",
		FloorsReached:    1,
		Outcome:          dungeon.StatusEscaped,
		ExpReward:        50,
		GoldReward:       250,
		MedalsReward:     2,
		ItemsRewardCount: 1,
		CreatedAt:        now,
	}
	invalidRewardItems := []coreitem.Instance{
		{
			ID:           id.New(),
			DefinitionID: "potion",
			Quantity:     0, // triggers ErrInvalidQuantity during deliverRewardItems after history & character writes
		},
	}

	err = dungeonRepo.FinalizeExpedition(ctx, hist, rec, &char, invalidRewardItems)
	if err == nil {
		t.Fatal("expected FinalizeExpedition to fail on invalid reward item delivery")
	}

	// Verify history was rolled back
	histories, err := dungeonRepo.GetHistory(ctx, char.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(histories) != 0 {
		t.Errorf("expected 0 histories after rollback, got %d", len(histories))
	}

	// Verify character money and medals unchanged
	afterChar, err := charRepo.FindByID(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}
	if afterChar.Money != 1000 || afterChar.SmallMedals != 0 {
		t.Errorf("expected money=1000 and medals=0 after rollback, got money=%d medals=%d", afterChar.Money, afterChar.SmallMedals)
	}
}

func TestDungeonRepository_ConcurrentEscapeAndWalletUpdate(t *testing.T) {
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
	invRepo, err := database.NewInventoryRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	depotRepo, err := database.NewDepotRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	bankRepo, err := database.NewBankRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	char, err := database.CreateTestCharacterWithFunds(ctx, db, "Concurrent Escape Hero", 5000)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "UPDATE characters SET deposit = 5000 WHERE id = ?", char.ID); err != nil {
		t.Fatal(err)
	}

	// Allow ample depot capacity
	if _, err := db.ExecContext(ctx, "UPDATE character_depots SET capacity = 50 WHERE character_id = ?", char.ID); err != nil {
		t.Fatal(err)
	}

	rec, err := dungeonRepo.GetRecord(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}

	runID := "exp_race_" + id.New()
	now := time.Now().UTC()

	// Race 4 concurrent FinalizeExpedition calls for the SAME runID alongside 4 concurrent Bank Deposits/Withdrawals
	const finalizeWorkers = 4
	const bankWorkers = 4
	var ready sync.WaitGroup
	ready.Add(finalizeWorkers + bankWorkers)
	start := make(chan struct{})

	ops := make([]func() error, 0, finalizeWorkers+bankWorkers)
	for i := 0; i < finalizeWorkers; i++ {
		workerIdx := i
		ops = append(ops, func() error {
			ready.Done()
			<-start
			staleSnapshot := char
			staleSnapshot.Money += 300
			staleSnapshot.SmallMedals += 1
			h := dungeon.DungeonExpeditionHistory{
				ID:               runID,
				CharacterID:      char.ID,
				DungeonID:        "dungeon-01",
				FloorsReached:    2,
				Outcome:          dungeon.StatusEscaped,
				ExpReward:        50,
				GoldReward:       300,
				MedalsReward:     1,
				ItemsRewardCount: 1,
				CreatedAt:        now,
			}
			r := rec
			r.TotalExpeditions = 1
			r.TotalFloorsCleared = 2
			items := []coreitem.Instance{
				{
					ID:           fmt.Sprintf("item_race_%s_%d", id.New()[:8], workerIdx),
					DefinitionID: "potion",
					Quantity:     1,
				},
			}
			return dungeonRepo.FinalizeExpedition(ctx, h, r, &staleSnapshot, items)
		})
	}

	for i := 0; i < bankWorkers; i++ {
		workerIdx := i
		ops = append(ops, func() error {
			ready.Done()
			<-start
			if workerIdx%2 == 0 {
				_, err := bankRepo.Deposit(ctx, char.ID, 100)
				return err
			}
			_, _, _, err := bankRepo.Withdraw(ctx, char.ID, 50)
			return err
		})
	}

	go func() {
		ready.Wait()
		close(start)
	}()

	errs := testutil.RunRace(ops...)

	finalizeSuccesses := 0
	alreadyFinished := 0
	for i := 0; i < finalizeWorkers; i++ {
		e := errs[i]
		if e == nil {
			finalizeSuccesses++
		} else if errors.Is(e, dungeon.ErrExpeditionAlreadyFinished) {
			alreadyFinished++
		} else {
			t.Fatalf("unexpected FinalizeExpedition error in worker %d: %v", i, e)
		}
	}
	if finalizeSuccesses != 1 || alreadyFinished != finalizeWorkers-1 {
		t.Fatalf("expected 1 success and %d ErrExpeditionAlreadyFinished, got success=%d alreadyFinished=%d",
			finalizeWorkers-1, finalizeSuccesses, alreadyFinished)
	}

	for i := finalizeWorkers; i < finalizeWorkers+bankWorkers; i++ {
		if errs[i] != nil {
			t.Fatalf("unexpected bank worker %d error: %v", i, errs[i])
		}
	}

	// Net bank movement: 2 deposits of 100 (-200 wallet, +200 deposit) and 2 withdrawals of 50 (+100 wallet, -100 deposit)
	// Plus 1 expedition reward of +300 wallet, +1 small medal.
	// Expected wallet: 5000 - 200 + 100 + 300 = 5200
	// Expected deposit: 5000 + 200 - 100 = 5100
	finalChar, err := charRepo.FindByID(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}
	if finalChar.Money != 5200 {
		t.Errorf("expected wallet 5200, got %d", finalChar.Money)
	}
	if finalChar.Deposit != 5100 {
		t.Errorf("expected deposit 5100, got %d", finalChar.Deposit)
	}
	if finalChar.SmallMedals != 1 {
		t.Errorf("expected small medals 1, got %d", finalChar.SmallMedals)
	}

	// Verify 1 history entry, 1 item delivered, 1 expedition recorded
	histories, err := dungeonRepo.GetHistory(ctx, char.ID, 10)
	if err != nil || len(histories) != 1 {
		t.Fatalf("expected 1 history record, got %d (err: %v)", len(histories), err)
	}
	inv, err := invRepo.FindByCharacterID(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}
	dep, err := depotRepo.FindByCharacterID(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(inv.Items)+len(dep.Items) != 1 {
		t.Errorf("expected 1 total reward item delivered, got inv=%d depot=%d", len(inv.Items), len(dep.Items))
	}
	finalRec, err := dungeonRepo.GetRecord(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}
	if finalRec.TotalExpeditions != 1 || finalRec.TotalFloorsCleared != 2 {
		t.Errorf("expected TotalExpeditions=1, TotalFloorsCleared=2, got %+v", finalRec)
	}
}
