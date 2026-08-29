package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/depot"
	"github.com/witchcraze/party2re/internal/id"
)

func TestTxContextHelpers(t *testing.T) {
	ctx := context.Background()
	if tx := TxFromContext(ctx); tx != nil {
		t.Fatalf("expected nil tx from background context, got %v", tx)
	}

	// ExecutorFromContext without tx in context should return fallback
	var dummyDB sqlContextExecutor = (*sql.DB)(nil)
	exec := ExecutorFromContext(ctx, dummyDB)
	if exec != dummyDB {
		t.Fatalf("expected fallback executor, got %v", exec)
	}
}

func TestMultiModuleTransactionOrchestrationCommitAndRollback(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()
	now := time.Now().UTC()

	playerRepo, err := NewPlayerRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	charRepo, err := NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	bankRepo, err := NewBankRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	depotRepo, err := NewDepotRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	invRepo, err := NewInventoryRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	itemDefRepo, err := NewItemDefinitionRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	// Define item definition for test
	itemDefID := "test_tx_item_" + id.New()
	itemDef, err := coreitem.NewDefinition(itemDefID, "Orchestration Test Item", 100)
	if err != nil {
		t.Fatal(err)
	}
	if err := itemDefRepo.Save(ctx, itemDef); err != nil {
		t.Fatal(err)
	}

	t.Run("MultiModuleAtomicCommit", func(t *testing.T) {
		suffix := fmt.Sprintf("%d", time.Now().UnixNano())
		player, err := coreplayer.New("orch_p1_"+suffix, "password123", now)
		if err != nil {
			t.Fatal(err)
		}

		char, err := corecharacter.New("OrchChar1_" + suffix)
		if err != nil {
			t.Fatal(err)
		}
		char.PlayerID = player.ID
		char.Money = 5000

		// Orchestrate multi-module atomic flow across 5 repositories in a single RunInTx
		err = RunInTx(ctx, db, func(txCtx context.Context) error {
			// 1. Save Player
			if err := playerRepo.Save(txCtx, player); err != nil {
				return fmt.Errorf("playerRepo.Save: %w", err)
			}
			// 2. Save Character
			if err := charRepo.Save(txCtx, char); err != nil {
				return fmt.Errorf("charRepo.Save: %w", err)
			}
			// 3. Deposit money into Bank (Deposit internally calls RunInTx, re-using ambient tx)
			if _, _, err := bankRepo.Deposit(txCtx, player.ID, char.ID, 2000); err != nil {
				return fmt.Errorf("bankRepo.Deposit: %w", err)
			}
			// 4. Deposit item into Depot
			depotItem, err := coreitem.NewInstance(itemDefID, 5)
			if err != nil {
				return err
			}
			dep, err := depot.NewDepot(char.ID)
			if err != nil {
				return err
			}
			if err := dep.AddItem(depotItem); err != nil {
				return err
			}
			if err := depotRepo.Save(txCtx, dep); err != nil {
				return fmt.Errorf("depotRepo.Save: %w", err)
			}
			// 5. Grant inventory item
			invItem, err := coreitem.NewInstance(itemDefID, 10)
			if err != nil {
				return err
			}
			inv, err := coreinventory.New(char.ID)
			if err != nil {
				return err
			}
			if err := inv.Add(invItem); err != nil {
				return err
			}
			if err := invRepo.Save(txCtx, inv); err != nil {
				return fmt.Errorf("invRepo.Save: %w", err)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("expected RunInTx to succeed, got %v", err)
		}

		// Verify state across all domains after commit using clean background context
		savedPlayer, err := playerRepo.FindByID(ctx, player.ID)
		if err != nil || savedPlayer.ID != player.ID {
			t.Fatalf("player was not saved: %v", err)
		}

		savedChar, err := charRepo.FindByID(ctx, char.ID)
		if err != nil || savedChar.ID != char.ID {
			t.Fatalf("character was not saved: %v", err)
		}
		if savedChar.Money != 3000 {
			t.Fatalf("expected character money 3000 after 2000 deposit, got %d", savedChar.Money)
		}

		account, err := bankRepo.GetAccount(ctx, player.ID)
		if err != nil || account.Balance != 2000 {
			t.Fatalf("expected bank balance 2000, got %v, err=%v", account, err)
		}

		dep, err := depotRepo.FindByCharacterID(ctx, char.ID)
		if err != nil || len(dep.Items) != 1 || dep.Items[0].Quantity != 5 {
			t.Fatalf("expected 1 depot item with qty 5, got %+v, err=%v", dep, err)
		}

		inv, err := invRepo.FindByCharacterID(ctx, char.ID)
		if err != nil || len(inv.Items) != 1 || inv.Items[0].Quantity != 10 {
			t.Fatalf("expected 1 inv item with qty 10, got %+v, err=%v", inv, err)
		}
	})

	t.Run("MultiModuleAtomicRollback", func(t *testing.T) {
		suffix := fmt.Sprintf("%d", time.Now().UnixNano())
		player, err := coreplayer.New("orch_p2_"+suffix, "password123", now)
		if err != nil {
			t.Fatal(err)
		}

		char, err := corecharacter.New("OrchChar2_" + suffix)
		if err != nil {
			t.Fatal(err)
		}
		char.PlayerID = player.ID
		char.Money = 5000

		simulatedErr := errors.New("simulated failure at end of transaction")

		// Orchestrate multi-module atomic flow that fails at the final step
		err = RunInTx(ctx, db, func(txCtx context.Context) error {
			if err := playerRepo.Save(txCtx, player); err != nil {
				return err
			}
			if err := charRepo.Save(txCtx, char); err != nil {
				return err
			}
			if _, _, err := bankRepo.Deposit(txCtx, player.ID, char.ID, 2000); err != nil {
				return err
			}
			depotItem, err := coreitem.NewInstance(itemDefID, 5)
			if err != nil {
				return err
			}
			dep, err := depot.NewDepot(char.ID)
			if err != nil {
				return err
			}
			if err := dep.AddItem(depotItem); err != nil {
				return err
			}
			if err := depotRepo.Save(txCtx, dep); err != nil {
				return err
			}
			// Fail before completing transaction
			return simulatedErr
		})

		if !errors.Is(err, simulatedErr) {
			t.Fatalf("expected simulated error, got %v", err)
		}

		// Verify that NONE of the mutations survived the rollback
		foundPlayer, err := playerRepo.FindByID(ctx, player.ID)
		if err == nil && foundPlayer.ID != "" {
			t.Fatalf("expected player to be rolled back, but found: %+v", foundPlayer)
		}

		foundChar, err := charRepo.FindByID(ctx, char.ID)
		if err == nil && foundChar.ID != "" {
			t.Fatalf("expected character to be rolled back, but found: %+v", foundChar)
		}

		account, err := bankRepo.GetAccount(ctx, player.ID)
		if err == nil && account.Balance != 0 {
			t.Fatalf("expected no bank account or 0 balance, got %+v", account)
		}

		dep, err := depotRepo.FindByCharacterID(ctx, char.ID)
		if err == nil && len(dep.Items) > 0 {
			t.Fatalf("expected empty depot, got %+v", dep)
		}
	})

	t.Run("NestedRunInTxReusesContext", func(t *testing.T) {
		nestCount := 0
		err := RunInTx(ctx, db, func(txCtx1 context.Context) error {
			nestCount++
			tx1 := TxFromContext(txCtx1)
			if tx1 == nil {
				t.Fatal("expected tx in outer context")
			}

			return RunInTx(txCtx1, db, func(txCtx2 context.Context) error {
				nestCount++
				tx2 := TxFromContext(txCtx2)
				if tx2 == nil {
					t.Fatal("expected tx in inner context")
				}
				if tx1 != tx2 {
					t.Fatalf("expected inner tx to be identical to outer tx (reused ambient tx), got %v vs %v", tx1, tx2)
				}
				return nil
			})
		})
		if err != nil {
			t.Fatalf("expected nested RunInTx to succeed, got %v", err)
		}
		if nestCount != 2 {
			t.Fatalf("expected nestCount 2, got %d", nestCount)
		}
	})
}
