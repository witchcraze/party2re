package home_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/economy"
	"github.com/witchcraze/party2re/internal/home"
	"github.com/witchcraze/party2re/internal/testutil"
	"github.com/witchcraze/party2re/internal/valkey"
)

type wakeCharacterRepository struct {
	*database.CharacterRepository
	afterLock func(context.Context) error
	updateErr error
}

func (r *wakeCharacterRepository) FindByIDForUpdate(ctx context.Context, id string) (corecharacter.Character, error) {
	c, err := r.CharacterRepository.FindByIDForUpdate(ctx, id)
	if err == nil && r.afterLock != nil {
		err = r.afterLock(ctx)
	}
	return c, err
}

func (r *wakeCharacterRepository) Update(ctx context.Context, c corecharacter.Character) error {
	if database.TxFromContext(ctx) == nil {
		return errors.New("Wake recovery write has no SQL transaction")
	}
	if err := r.CharacterRepository.Update(ctx, c); err != nil {
		return err
	}
	return r.updateErr
}

type wakeFullnessHook func(context.Context, string) error

func (f wakeFullnessHook) ResetFullness(ctx context.Context, id string) error { return f(ctx, id) }

func TestWake_SQLRecovery(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" || os.Getenv("PARTY2_VALKEY_ADDR") == "" {
		t.Skip("SQL and Valkey integration environment is not configured")
	}
	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	client, err := valkey.NewClient()
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	timers := timer.NewService(client)
	charRepo, err := database.NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	homeRepo, err := database.NewHomeRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	invRepo, err := database.NewInventoryRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	credit, err := economy.NewService(charRepo, invRepo, economy.WithTransactionProvider(database.NewTransactionProvider(db)))
	if err != nil {
		t.Fatal(err)
	}

	for _, mode := range []string{"credit_before_wake", "credit_during_wake", "recovery_write_failure", "hook_failure", "stress"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			char, err := database.CreateTestCharacter(ctx, db, "WakeAtomic")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				cleanupCtx := context.Background()
				if err := timers.ReleaseLock(cleanupCtx, timer.CategoryAsleep, char.ID); err != nil {
					t.Error(err)
				}
				if _, err := db.ExecContext(cleanupCtx, "DELETE FROM character_depots WHERE character_id = ?", char.ID); err != nil {
					t.Error(err)
				}
				if _, err := db.ExecContext(cleanupCtx, "DELETE FROM characters WHERE id = ?", char.ID); err != nil {
					t.Error(err)
				}
				if _, err := db.ExecContext(cleanupCtx, "DELETE FROM players WHERE id = ?", char.PlayerID); err != nil {
					t.Error(err)
				}
			})
			char.Money, char.Deposit, char.Experience = 20000, 100, 10
			char.Stats.HP, char.Stats.MP, char.Tired = 1, 0, 80
			if err := charRepo.Update(ctx, char); err != nil {
				t.Fatal(err)
			}
			if err := timers.SetLock(ctx, timer.CategoryAsleep, char.ID, time.Hour); err != nil {
				t.Fatal(err)
			}
			addAssets := func() error {
				_, err := credit.ExecuteTransaction(ctx, economy.TransactionRequest{CharacterID: char.ID, Grant: economy.ResourceGrant{Gold: 50, SmallMedals: 2}}, func(tc *economy.TxContext) error {
					tc.Character.Deposit += 7
					tc.Character.Experience += 3
					tc.Character.SP++
					return nil
				})
				return err
			}
			locked := &wakeCharacterRepository{CharacterRepository: charRepo}
			locked.afterLock = func(txCtx context.Context) error {
				if database.TxFromContext(txCtx) == nil {
					return errors.New("Wake recovery read has no SQL transaction")
				}
				return nil
			}
			eco, err := economy.NewService(locked, invRepo, economy.WithTransactionProvider(database.NewTransactionProvider(db)))
			if err != nil {
				t.Fatal(err)
			}
			svc, err := home.NewService(homeRepo, charRepo, home.WithCharacterUpdater(locked), home.WithEconomy(eco), home.WithTimer(timers))
			if err != nil {
				t.Fatal(err)
			}
			wake := func() error { _, err := svc.Wake(ctx, char.ID); return err }
			credits := 1
			hookErr := errors.New("fullness unavailable")
			switch mode {
			case "credit_before_wake":
				if err := addAssets(); err != nil {
					t.Fatal(err)
				}
				if err := wake(); err != nil {
					t.Fatal(err)
				}
			case "recovery_write_failure":
				if err := addAssets(); err != nil {
					t.Fatal(err)
				}
				locked.updateErr = errors.New("recovery persistence failed")
				svc.SetFullnessResetter(wakeFullnessHook(func(context.Context, string) error {
					t.Error("hook ran after failed recovery transaction")
					return nil
				}))
				if err := wake(); !errors.Is(err, locked.updateErr) {
					t.Fatalf("Wake error = %v", err)
				}
			case "credit_during_wake":
				read := make(chan struct{})
				started := make(chan struct{})
				locked.afterLock = func(txCtx context.Context) error {
					if database.TxFromContext(txCtx) == nil {
						return errors.New("Wake recovery read has no SQL transaction")
					}
					close(read)
					select {
					case <-started:
						return nil
					case <-ctx.Done():
						return ctx.Err()
					}
				}
				a, b := testutil.RunRace2(wake, func() error {
					select {
					case <-read:
					case <-ctx.Done():
						return ctx.Err()
					}
					close(started)
					return addAssets()
				})
				if a != nil || b != nil {
					t.Fatalf("Wake/credit: %v / %v", a, b)
				}
			case "hook_failure":
				svc.SetFullnessResetter(wakeFullnessHook(func(hookCtx context.Context, id string) error {
					if database.TxFromContext(hookCtx) != nil {
						return errors.New("hook unexpectedly inside recovery transaction")
					}
					// A fresh transaction here also proves the recovery row lock has been released.
					if err := addAssets(); err != nil {
						return err
					}
					return hookErr
				}))
				if err := wake(); !errors.Is(err, hookErr) {
					t.Fatalf("Wake error = %v", err)
				}
			case "stress":
				cfg := testutil.GetStressConfig()
				credits = cfg.Workers / 2 * cfg.OpsPerWorker
				res := testutil.RunConcurrentStressTest(t, cfg, func(worker, op int) error {
					if worker%2 == 0 {
						return wake()
					}
					return addAssets()
				})
				if res.Failures != 0 || res.Deadlocks != 0 {
					t.Fatalf("stress: %+v", res)
				}
			}
			final, err := charRepo.FindByID(ctx, char.ID)
			if err != nil {
				t.Fatal(err)
			}
			if final.Money != char.Money+50*credits || final.Deposit != char.Deposit+int64(7*credits) || final.Experience != char.Experience+3*credits || final.SP != char.SP+credits || final.SmallMedals != char.SmallMedals+2*credits {
				t.Fatalf("assets lost after %d credits: %+v", credits, final)
			}
			if mode == "recovery_write_failure" {
				if final.Stats != char.Stats || final.Tired != char.Tired {
					t.Fatalf("failed SQL recovery was not rolled back: %+v", final)
				}
			} else if final.Stats.HP != final.Stats.MaxHP || final.Stats.MP != final.Stats.MaxMP || final.Tired != 0 {
				t.Fatalf("vitality not restored: %+v", final)
			}
			asleep, err := timers.IsLocked(ctx, timer.CategoryAsleep, char.ID)
			if err != nil {
				t.Fatal(err)
			}
			if asleep != (mode == "hook_failure" || mode == "recovery_write_failure") {
				t.Fatalf("pending recovery = %v in %s", asleep, mode)
			}
			if mode == "hook_failure" {
				svc.SetFullnessResetter(nil)
				if err := wake(); err != nil {
					t.Fatalf("explicit recovery retry: %v", err)
				}
			}
		})
	}
}
