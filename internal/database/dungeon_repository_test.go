package database_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/dungeon"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/testutil"
)

func TestDungeonRepository(t *testing.T) {
	db := database.OpenTestDB(t)

	ctx := context.Background()
	dungeonRepo, err := database.NewDungeonRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	char, err := database.CreateTestCharacter(ctx, db, "Dungeon Explorer")
	if err != nil {
		t.Fatal(err)
	}

	// 2. SaveActiveExpedition & GetActiveExpedition
	now := time.Now().UTC()
	expID := fmt.Sprintf("exp_%016x", now.UnixNano())
	exp := dungeon.ActiveExpedition{
		ID:          expID,
		CharacterID: char.ID,
		DungeonID:   "dungeon-01",
		PosX:        2,
		PosY:        0,
		CurrentHP:   char.Stats.HP,
		Round:       30, RoundLimit: 50,
		AccumulatedExp:  150,
		AccumulatedGold: 300,
		Status:          dungeon.StatusExploring,
		StartedAt:       now,
		UpdatedAt:       now,
	}

	if err := dungeonRepo.SaveActiveExpedition(ctx, exp); err != nil {
		t.Fatalf("SaveActiveExpedition failed: %v", err)
	}

	fetchedExp, err := dungeonRepo.GetActiveExpedition(ctx, char.ID)
	if err != nil || fetchedExp == nil {
		t.Fatalf("GetActiveExpedition failed: %v", err)
	}
	if fetchedExp.ID != expID || fetchedExp.AccumulatedGold != 300 {
		t.Errorf("unexpected fetched expedition: %#v", fetchedExp)
	}

	// 3. FinalizeExpedition

	histID := expID
	history := dungeon.DungeonExpeditionHistory{
		ID:          histID,
		CharacterID: char.ID,
		DungeonID:   "dungeon-01",
		Outcome:     dungeon.StatusEscaped,
		ExpReward:   450,
		GoldReward:  800,
		CreatedAt:   now,
	}

	err = dungeonRepo.FinalizeExpedition(ctx, history, &char)
	if err != nil {
		t.Fatalf("FinalizeExpedition failed: %v", err)
	}

	// 4. Verify Active Expedition Deleted
	clearedExp, err := dungeonRepo.GetActiveExpedition(ctx, char.ID)
	if err != nil || clearedExp != nil {
		t.Errorf("expected active expedition to be nil after finalize, got %#v", clearedExp)
	}

	// 6. Verify History
	histories, err := dungeonRepo.GetHistory(ctx, char.ID, 5)
	if err != nil || len(histories) != 1 {
		t.Fatalf("GetHistory failed: %v, len=%d", err, len(histories))
	}
	if histories[0].Outcome != dungeon.StatusEscaped || histories[0].GoldReward != 800 {
		t.Errorf("unexpected history entry: %#v", histories[0])
	}
}

func TestDungeonRepository_IdempotentClaimAndWalletPreservation(t *testing.T) {
	db := database.OpenTestDB(t)

	ctx := context.Background()
	dungeonRepo, err := database.NewDungeonRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	charRepo, err := database.NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	char, err := database.CreateTestCharacterWithFunds(ctx, db, "Dungeon Claim Hero", 1000)
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

	hist := dungeon.DungeonExpeditionHistory{
		ID:          runID,
		CharacterID: char.ID,
		DungeonID:   "dungeon-02",
		Outcome:     dungeon.StatusEscaped,
		ExpReward:   200,
		GoldReward:  400,
		CreatedAt:   now,
	}

	// Simulate stale caller snapshot that already added rewards onto stale money (1000 + 400 = 1400).
	// FinalizeExpedition MUST reload and lock the latest DB row (1500) and apply +400 -> 1900!
	staleChar.Money += 400
	staleChar.SmallMedals += 2

	stock := dungeon.ExpeditionMember{CharacterID: char.ID, CurrentHP: char.Stats.HP, AccumulatedExp: 200, AccumulatedGold: 400}
	if err := dungeonRepo.FinalizeExpedition(ctx, hist, &staleChar, stock); err != nil {
		t.Fatalf("first FinalizeExpedition failed: %v", err)
	}

	// Duplicate claim with the same run ID must return ErrExpeditionAlreadyFinished and roll back / skip duplicate grants
	err = dungeonRepo.FinalizeExpedition(ctx, hist, &staleChar, stock)
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
	if updatedChar.SmallMedals != 3 {
		t.Errorf("expected small medals 3 (preserving concurrent +3), got %d", updatedChar.SmallMedals)
	}

	// Verify 1 history and 1 expedition count in record
	histories, err := dungeonRepo.GetHistory(ctx, char.ID, 10)
	if err != nil || len(histories) != 1 {
		t.Fatalf("expected 1 history record, got %d (err: %v)", len(histories), err)
	}
}

func TestDungeonRepository_ConcurrentEscapeAndWalletUpdate(t *testing.T) {
	db := database.OpenTestDB(t)

	ctx := context.Background()
	dungeonRepo, err := database.NewDungeonRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	charRepo, err := database.NewCharacterRepository(db)
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
		ops = append(ops, func() error {
			ready.Done()
			<-start
			staleSnapshot := char
			staleSnapshot.Money += 300
			h := dungeon.DungeonExpeditionHistory{
				ID:          runID,
				CharacterID: char.ID,
				DungeonID:   "dungeon-01",
				Outcome:     dungeon.StatusEscaped,
				ExpReward:   50,
				GoldReward:  300,
				CreatedAt:   now,
			}
			return dungeonRepo.FinalizeExpedition(ctx, h, &staleSnapshot, dungeon.ExpeditionMember{CharacterID: char.ID, CurrentHP: char.Stats.HP, AccumulatedExp: 50, AccumulatedGold: 300})
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
	// Plus 1 expedition reward of +300 wallet.
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

	// Verify 1 history entry, 1 expedition recorded
	histories, err := dungeonRepo.GetHistory(ctx, char.ID, 10)
	if err != nil || len(histories) != 1 {
		t.Fatalf("expected 1 history record, got %d (err: %v)", len(histories), err)
	}
}
