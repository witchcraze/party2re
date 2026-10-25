package dungeon_test

import (
	"context"
	"errors"
	"os"
	"sync"
	"sync/atomic"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/dungeon"
	"github.com/witchcraze/party2re/internal/testutil"
	vk "github.com/witchcraze/party2re/internal/valkey"
)

func TestDungeonIntegrationFlow(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()
	charRepo, err := database.NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	dungeonRepo, err := database.NewDungeonRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	battleEngine := corebattle.Engine{}
	var opts []dungeon.Option
	if os.Getenv("PARTY2_VALKEY_ADDR") != "" {
		vkClient, err := vk.NewClient()
		if err != nil {
			t.Fatal(err)
		}
		defer vkClient.Close()
		vStore, err := dungeon.NewValkeyExpeditionRepository(vkClient)
		if err != nil {
			t.Fatal(err)
		}
		opts = append(opts, dungeon.WithActiveExpeditionStore(vStore))
	}
	service, err := dungeon.NewService(dungeonRepo, charRepo, battleEngine, opts...)
	if err != nil {
		t.Fatal(err)
	}

	// 1. Create a high-level test character capable of exploring Dungeon 1
	char, err := database.CreateTestCharacter(ctx, db, "Legendary Dungeon Crawler")
	if err != nil {
		t.Fatal(err)
	}

	// Buff stats
	updateStatsQuery := `
		UPDATE characters
		SET level = 30, hp = 800, max_hp = 800, attack = 200, defense = 150, agility = 100
		WHERE id = ?
	`
	if _, err := db.ExecContext(ctx, updateStatsQuery, char.ID); err != nil {
		t.Fatal(err)
	}

	// 2. List Dungeons
	overviews, err := service.ListDungeons(ctx, char.ID)
	if err != nil {
		t.Fatalf("ListDungeons failed: %v", err)
	}
	if len(overviews) != 3 {
		t.Errorf("expected 3 default dungeons, got %d", len(overviews))
	}
	if !overviews[0].IsUnlocked {
		t.Errorf("expected Dungeon 1 to be unlocked for level 30 character")
	}

	// 3. Start Expedition on Dungeon 1
	exp, err := service.StartExpedition(ctx, char.ID, "dungeon-01")
	if err != nil {
		t.Fatalf("StartExpedition failed: %v", err)
	}
	if exp.CurrentFloor != 1 || exp.Status != dungeon.StatusExploring {
		t.Errorf("unexpected started expedition: %#v", exp)
	}

	// 4. Navigate on Floor 1:
	// Grid Floor 1:
	// S00T
	// 1101
	// 0X0D
	// 1000
	// (0,0) S -> East (1,0) '0' -> East (2,0) '0' -> South (2,1) '0' -> South (2,2) '0' -> East (3,2) 'D'
	_, err = service.Move(ctx, char.ID, dungeon.DirectionEast) // to (1, 0)
	if err != nil {
		t.Fatalf("step 1 failed: %v", err)
	}
	_, err = service.Move(ctx, char.ID, dungeon.DirectionEast) // to (2, 0)
	if err != nil {
		t.Fatalf("step 2 failed: %v", err)
	}
	_, err = service.Move(ctx, char.ID, dungeon.DirectionSouth) // to (2, 1)
	if err != nil {
		t.Fatalf("step 3 failed: %v", err)
	}
	_, err = service.Move(ctx, char.ID, dungeon.DirectionSouth) // to (2, 2)
	if err != nil {
		t.Fatalf("step 4 failed: %v", err)
	}
	stairRes, err := service.Move(ctx, char.ID, dungeon.DirectionEast) // to (3, 2) 'D' (Down stairs to Floor 2!)
	if err != nil {
		t.Fatalf("step 5 stairs failed: %v", err)
	}
	if stairRes.EventType != dungeon.EventStairs || stairRes.Expedition.CurrentFloor != 2 {
		t.Fatalf("expected stairs to floor 2, got %#v", stairRes)
	}

	// 5. Navigate on Floor 2 to Boss:
	// Grid Floor 2:
	// S0X0
	// 1010
	// 00T0
	// 101B
	// (0,0) S -> South (0,1) wall '1' -> East (1,0) '0' -> South (1,1) '0' -> South (1,2) '0' -> East (2,2) 'T' -> East (3,2) '0' -> South (3,3) 'B' (Boss!)
	_, err = service.Move(ctx, char.ID, dungeon.DirectionEast) // to (1, 0)
	if err != nil {
		t.Fatalf("floor 2 step 1 failed: %v", err)
	}
	_, err = service.Move(ctx, char.ID, dungeon.DirectionSouth) // to (1, 1)
	if err != nil {
		t.Fatalf("floor 2 step 2 failed: %v", err)
	}
	_, err = service.Move(ctx, char.ID, dungeon.DirectionSouth) // to (1, 2)
	if err != nil {
		t.Fatalf("floor 2 step 3 failed: %v", err)
	}
	_, err = service.Move(ctx, char.ID, dungeon.DirectionEast) // to (2, 2) 'T' (Treasure)
	if err != nil {
		t.Fatalf("floor 2 step 4 treasure failed: %v", err)
	}
	_, err = service.Move(ctx, char.ID, dungeon.DirectionEast) // to (3, 2) '0'
	if err != nil {
		t.Fatalf("floor 2 step 5 failed: %v", err)
	}
	bossRes, err := service.Move(ctx, char.ID, dungeon.DirectionSouth) // to (3, 3) 'B' (Boss Fight & Clear!)
	if err != nil {
		t.Fatalf("floor 2 boss fight failed: %v", err)
	}
	if !bossRes.IsFinished || bossRes.Expedition.Status != dungeon.StatusCleared {
		t.Fatalf("expected dungeon clear, got %#v", bossRes)
	}

	// 6. Verify Active Expedition Removed
	activeExp, _ := service.GetActiveExpedition(ctx, char.ID)
	if activeExp != nil {
		t.Errorf("expected active expedition to be removed after clear")
	}

	// 7. Verify History & Record in Database
	history, err := service.GetHistory(ctx, char.ID, 5)
	if err != nil || len(history) != 1 {
		t.Fatalf("expected 1 history record, got %d (err: %v)", len(history), err)
	}
	if history[0].Outcome != dungeon.StatusCleared {
		t.Errorf("expected history outcome cleared, got %v", history[0].Outcome)
	}

	record, err := service.GetRecord(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}
	if record.HighestDungeonCleared != 1 || record.TotalExpeditions != 1 {
		t.Errorf("unexpected record statistics: %#v", record)
	}
}

type failOnceDeleteStore struct {
	inner     dungeon.ActiveExpeditionStore
	failCalls atomic.Int32
}

func (f *failOnceDeleteStore) GetActiveExpedition(ctx context.Context, charID string) (*dungeon.ActiveExpedition, error) {
	return f.inner.GetActiveExpedition(ctx, charID)
}

func (f *failOnceDeleteStore) SaveActiveExpedition(ctx context.Context, exp dungeon.ActiveExpedition) error {
	return f.inner.SaveActiveExpedition(ctx, exp)
}

func (f *failOnceDeleteStore) DeleteActiveExpedition(ctx context.Context, charID string) error {
	if f.failCalls.Add(-1) >= 0 {
		return errors.New("simulated valkey delete outage")
	}
	return f.inner.DeleteActiveExpedition(ctx, charID)
}

func (f *failOnceDeleteStore) Step(ctx context.Context, charID string, params dungeon.StepParams) (dungeon.StepOutcome, error) {
	return f.inner.Step(ctx, charID, params)
}

func TestDungeonIntegration_ConcurrentEscapeAndCleanupRetry(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}
	if os.Getenv("PARTY2_VALKEY_ADDR") == "" {
		t.Skip("PARTY2_VALKEY_ADDR is not configured")
	}

	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	vkClient, err := vk.NewClient()
	if err != nil {
		t.Fatal(err)
	}
	defer vkClient.Close()

	vStore, err := dungeon.NewValkeyExpeditionRepository(vkClient)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	charRepo, err := database.NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
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
	bankRepo, err := database.NewBankRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("ValkeyDeleteFailureAndRetry", func(t *testing.T) {
		flakyStore := &failOnceDeleteStore{inner: vStore}
		flakyStore.failCalls.Store(1) // first DeleteActiveExpedition fails

		svc, err := dungeon.NewService(dungeonRepo, charRepo, corebattle.Engine{}, dungeon.WithActiveExpeditionStore(flakyStore))
		if err != nil {
			t.Fatal(err)
		}

		char, err := database.CreateTestCharacterWithFunds(ctx, db, "Retry Escape Hero", 1000)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(ctx, "UPDATE characters SET level = 10 WHERE id = ?", char.ID); err != nil {
			t.Fatal(err)
		}

		exp, err := svc.StartExpedition(ctx, char.ID, "dungeon-01")
		if err != nil {
			t.Fatalf("StartExpedition failed: %v", err)
		}
		exp.AccumulatedGold = 250
		exp.AccumulatedExp = 80
		exp.AccumulatedMedals = 1
		exp.AccumulatedItems = []string{"potion"}
		if err := vStore.SaveActiveExpedition(ctx, *exp); err != nil {
			t.Fatalf("SaveActiveExpedition failed: %v", err)
		}

		// First Escape: SQL commits, Valkey delete fails
		if _, err := svc.Escape(ctx, char.ID); err != nil {
			t.Fatalf("first Escape failed: %v", err)
		}
		// Active expedition still lingers in Valkey
		if lingering, err := svc.GetActiveExpedition(ctx, char.ID); err != nil || lingering == nil {
			t.Fatalf("expected active expedition to linger after failed Valkey delete, got %v, err=%v", lingering, err)
		}

		// Second Escape (retry): SQL hits ErrExpeditionAlreadyFinished (idempotent), Valkey delete succeeds
		if _, err := svc.Escape(ctx, char.ID); err != nil {
			t.Fatalf("retry Escape failed: %v", err)
		}
		if afterRetryExp, err := svc.GetActiveExpedition(ctx, char.ID); err != nil || afterRetryExp != nil {
			t.Fatalf("expected active expedition to be nil after retry cleanup, got %v (err=%v)", afterRetryExp, err)
		}

		afterChar, err := charRepo.FindByID(ctx, char.ID)
		if err != nil {
			t.Fatal(err)
		}
		if afterChar.Money != 1250 || afterChar.SmallMedals != 1 {
			t.Errorf("expected money=1250 and medals=1 (single grant), got money=%d medals=%d", afterChar.Money, afterChar.SmallMedals)
		}
		histories, err := svc.GetHistory(ctx, char.ID, 10)
		if err != nil || len(histories) != 1 {
			t.Fatalf("expected 1 history record, got %d (err=%v)", len(histories), err)
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
			t.Errorf("expected 1 item delivered across inv+depot, got inv=%d depot=%d", len(inv.Items), len(dep.Items))
		}
	})

	t.Run("ConcurrentEscapeAndBankMovement", func(t *testing.T) {
		svc, err := dungeon.NewService(dungeonRepo, charRepo, corebattle.Engine{}, dungeon.WithActiveExpeditionStore(vStore))
		if err != nil {
			t.Fatal(err)
		}

		char, err := database.CreateTestCharacterWithFunds(ctx, db, "Concurrent Service Escape Hero", 2000)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(ctx, "UPDATE characters SET level = 10, deposit = 2000 WHERE id = ?", char.ID); err != nil {
			t.Fatal(err)
		}

		exp, err := svc.StartExpedition(ctx, char.ID, "dungeon-01")
		if err != nil {
			t.Fatalf("StartExpedition failed: %v", err)
		}
		exp.AccumulatedGold = 300
		exp.AccumulatedExp = 60
		exp.AccumulatedMedals = 2
		exp.AccumulatedItems = []string{"potion"}
		if err := vStore.SaveActiveExpedition(ctx, *exp); err != nil {
			t.Fatalf("SaveActiveExpedition failed: %v", err)
		}

		const escapeWorkers = 4
		const bankWorkers = 4
		var ready sync.WaitGroup
		ready.Add(escapeWorkers + bankWorkers)
		start := make(chan struct{})

		ops := make([]func() error, 0, escapeWorkers+bankWorkers)
		for i := 0; i < escapeWorkers; i++ {
			ops = append(ops, func() error {
				ready.Done()
				<-start
				_, err := svc.Escape(ctx, char.ID)
				return err
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

		escapeSuccesses := 0
		for i := 0; i < escapeWorkers; i++ {
			e := errs[i]
			if e == nil {
				escapeSuccesses++
			} else if !errors.Is(e, dungeon.ErrNoActiveExpedition) {
				t.Fatalf("unexpected Escape error in worker %d: %v", i, e)
			}
		}
		if escapeSuccesses < 1 {
			t.Fatalf("expected at least 1 Escape call to succeed, got %d", escapeSuccesses)
		}
		for i := escapeWorkers; i < escapeWorkers+bankWorkers; i++ {
			if errs[i] != nil {
				t.Fatalf("unexpected bank worker %d error: %v", i, errs[i])
			}
		}

		// Wallet: 2000 - 200 (2 deposits) + 100 (2 withdrawals) + 300 (1 escape reward) = 2200
		// Deposit: 2000 + 200 - 100 = 2100
		afterChar, err := charRepo.FindByID(ctx, char.ID)
		if err != nil {
			t.Fatal(err)
		}
		if afterChar.Money != 2200 || afterChar.Deposit != 2100 || afterChar.SmallMedals != 2 {
			t.Errorf("expected money=2200 deposit=2100 medals=2, got money=%d deposit=%d medals=%d",
				afterChar.Money, afterChar.Deposit, afterChar.SmallMedals)
		}

		histories, err := svc.GetHistory(ctx, char.ID, 10)
		if err != nil || len(histories) != 1 {
			t.Fatalf("expected 1 history record, got %d (err=%v)", len(histories), err)
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
			t.Errorf("expected 1 item delivered across inv+depot, got inv=%d depot=%d", len(inv.Items), len(dep.Items))
		}
	})
}
