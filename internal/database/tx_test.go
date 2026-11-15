package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"sync/atomic"
	"testing"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/depot"
	"github.com/witchcraze/party2re/internal/economy"
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

		char, err := corecharacter.New("C1_" + suffix[len(suffix)-5:])
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
			if _, err := bankRepo.Deposit(txCtx, char.ID, 2000); err != nil {
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
		if savedChar.Deposit != 2000 {
			t.Fatalf("expected character deposit 2000, got %d", savedChar.Deposit)
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

		char, err := corecharacter.New("C2_" + suffix[len(suffix)-5:])
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
			if _, err := bankRepo.Deposit(txCtx, char.ID, 2000); err != nil {
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

func TestRunInTx_ContextCancelAndCommitErrorRollbackScopeContract(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	chars, err := NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	players, err := NewPlayerRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	newChar := func(t *testing.T, suffix string) corecharacter.Character {
		t.Helper()
		c, err := CreateTestCharacterWithFunds(context.Background(), db, suffix, 100)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := chars.Delete(context.Background(), c.ID); err != nil {
				t.Error(err)
			}
			if err := players.Delete(context.Background(), c.PlayerID); err != nil {
				t.Error(err)
			}
		})
		return c
	}

	t.Run("context cancel keeps row lock held through OnRollback and rolls back SQL", func(t *testing.T) {
		for _, tc := range []struct {
			name            string
			execAfterCancel bool
		}{
			{"cancel before subsequent SQL", true},
			{"cancel right before commit", false},
		} {
			t.Run(tc.name, func(t *testing.T) {
				c := newChar(t, "TxCancel")
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()

				var hookCalls atomic.Int32
				var hookSawLock atomic.Bool
				var hookCtxErr error

				err := RunInTx(ctx, db, func(txCtx context.Context) error {
					if _, err := ExecutorFromContext(txCtx, db).ExecContext(txCtx,
						`SELECT id FROM characters WHERE id = ? FOR UPDATE`, c.ID); err != nil {
						return err
					}
					economy.OnRollback(txCtx, func(rbCtx context.Context) error {
						hookCalls.Add(1)
						hookCtxErr = rbCtx.Err()
						_, lockErr := db.ExecContext(context.Background(),
							`SELECT id FROM characters WHERE id = ? FOR UPDATE NOWAIT`, c.ID)
						hookSawLock.Store(lockErr != nil)
						return nil
					})
					if _, err := ExecutorFromContext(txCtx, db).ExecContext(txCtx,
						`UPDATE characters SET money = money - 10 WHERE id = ?`, c.ID); err != nil {
						return err
					}
					cancel()
					if tc.execAfterCancel {
						_, err := ExecutorFromContext(txCtx, db).ExecContext(txCtx,
							`UPDATE characters SET money = money - 10 WHERE id = ?`, c.ID)
						return err
					}
					return nil
				})
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("want context.Canceled, got %v", err)
				}
				if hookCalls.Load() != 1 || !hookSawLock.Load() || hookCtxErr != nil {
					t.Fatalf("hookCalls=%d hookSawLock=%v hookCtxErr=%v; want 1, true, nil",
						hookCalls.Load(), hookSawLock.Load(), hookCtxErr)
				}
				reloaded, err := chars.FindByID(context.Background(), c.ID)
				if err != nil {
					t.Fatal(err)
				}
				if reloaded.Money != c.Money {
					t.Fatalf("SQL not rolled back: money=%d want=%d", reloaded.Money, c.Money)
				}
			})
		}
	})

	t.Run("commit error skips OnRollback and propagates directly", func(t *testing.T) {
		c := newChar(t, "TxCommitErr")
		var hookCalls atomic.Int32

		err := RunInTx(context.Background(), db, func(txCtx context.Context) error {
			if _, err := ExecutorFromContext(txCtx, db).ExecContext(txCtx,
				`SELECT id FROM characters WHERE id = ? FOR UPDATE`, c.ID); err != nil {
				return err
			}
			economy.OnRollback(txCtx, func(context.Context) error {
				hookCalls.Add(1)
				return nil
			})
			if _, err := ExecutorFromContext(txCtx, db).ExecContext(txCtx,
				`UPDATE characters SET money = money - 25 WHERE id = ?`, c.ID); err != nil {
				return err
			}
			// Simulate SQL committing right as runInNewTx reaches tx.Commit(), causing tx.Commit() to return an error.
			return TxFromContext(txCtx).Commit()
		})
		if !errors.Is(err, sql.ErrTxDone) {
			t.Fatalf("want sql.ErrTxDone on commit error, got %v", err)
		}
		if hookCalls.Load() != 0 {
			t.Fatalf("OnRollback ran %d times on commit error; want 0", hookCalls.Load())
		}
		reloaded, err := chars.FindByID(context.Background(), c.ID)
		if err != nil {
			t.Fatal(err)
		}
		if reloaded.Money != c.Money-25 {
			t.Fatalf("committed SQL state unexpected: money=%d want=%d", reloaded.Money, c.Money-25)
		}
	})

	t.Run("pool wait respects context cancel while active tx holds row lock through OnRollback and returns conn", func(t *testing.T) {
		limitedDB, err := OpenFromEnvironment()
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = limitedDB.Close() })
		limitedDB.SetMaxOpenConns(1)

		c := newChar(t, "TxPoolWait")
		ctx1, cancel1 := context.WithCancel(context.Background())
		defer cancel1()

		tx1Locked := make(chan struct{})
		releaseTx1 := make(chan struct{})
		var hookCalls atomic.Int32
		var hookSawLock atomic.Bool
		err1Ch := make(chan error, 1)

		go func() {
			err1Ch <- RunInTx(ctx1, limitedDB, func(txCtx context.Context) error {
				if _, err := ExecutorFromContext(txCtx, limitedDB).ExecContext(txCtx,
					`SELECT id FROM characters WHERE id = ? FOR UPDATE`, c.ID); err != nil {
					close(tx1Locked)
					return err
				}
				economy.OnRollback(txCtx, func(rbCtx context.Context) error {
					hookCalls.Add(1)
					if rbCtx.Err() != nil {
						return rbCtx.Err()
					}
					_, lockErr := db.ExecContext(context.Background(),
						`SELECT id FROM characters WHERE id = ? FOR UPDATE NOWAIT`, c.ID)
					hookSawLock.Store(lockErr != nil)
					return nil
				})
				close(tx1Locked)
				<-releaseTx1
				return nil
			})
		}()

		<-tx1Locked

		// Exhausted MaxOpenConns(1) pool: second caller waiting for a connection must unblock when its ctx expires.
		waitCtx, cancelWait := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancelWait()
		waitErr := RunInTx(waitCtx, limitedDB, func(txCtx context.Context) error {
			return errors.New("waiter callback should not run while pool is exhausted")
		})
		if !errors.Is(waitErr, context.DeadlineExceeded) {
			close(releaseTx1)
			<-err1Ch
			t.Fatalf("want context.DeadlineExceeded during pool wait, got %v", waitErr)
		}

		// Canceling tx1 after releasing it must still keep the SQL row lock through OnRollback.
		cancel1()
		close(releaseTx1)
		if err1 := <-err1Ch; !errors.Is(err1, context.Canceled) {
			t.Fatalf("want tx1 error context.Canceled, got %v", err1)
		}
		if hookCalls.Load() != 1 || !hookSawLock.Load() {
			t.Fatalf("hookCalls=%d hookSawLock=%v; want 1, true", hookCalls.Load(), hookSawLock.Load())
		}

		// Confirm the single pooled connection was returned after rollback and close.
		followCtx, cancelFollow := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancelFollow()
		if err := RunInTx(followCtx, limitedDB, func(txCtx context.Context) error {
			_, err := ExecutorFromContext(txCtx, limitedDB).ExecContext(txCtx, `SELECT 1`)
			return err
		}); err != nil {
			t.Fatalf("single-conn pool failed after rollback (conn leak?): %v", err)
		}
	})
}
