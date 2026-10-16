package home_test

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/economy"
	"github.com/witchcraze/party2re/internal/home"
	"github.com/witchcraze/party2re/internal/tavern"
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

type wakeCostumeHook func(context.Context, string) error

func (f wakeCostumeHook) ResetCostume(ctx context.Context, id string) error { return f(ctx, id) }

type failingTavernStatusReader struct {
	*database.TavernRepository
	getErr error
}

func (r *failingTavernStatusReader) GetCharacterStatus(ctx context.Context, id string) (tavern.TavernCharacterStatus, error) {
	if r.getErr != nil {
		return tavern.TavernCharacterStatus{}, r.getErr
	}
	return r.TavernRepository.GetCharacterStatus(ctx, id)
}

func (r *failingTavernStatusReader) ResetFullness(ctx context.Context, id string) error {
	if r.getErr != nil {
		return r.getErr
	}
	return r.TavernRepository.ResetFullness(ctx, id)
}

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
			char.PendingWake = true
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
				var lockOnce sync.Once
				locked.afterLock = func(txCtx context.Context) error {
					if database.TxFromContext(txCtx) == nil {
						return errors.New("Wake recovery read has no SQL transaction")
					}
					var err error
					lockOnce.Do(func() {
						close(read)
						select {
						case <-started:
						case <-ctx.Done():
							err = ctx.Err()
						}
					})
					return err
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
						err := wake()
						if errors.Is(err, home.ErrWakeInProgress) {
							return nil
						}
						return err
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
			if final.PendingWake != (mode == "hook_failure" || mode == "recovery_write_failure") {
				t.Fatalf("pending wake = %v in %s", final.PendingWake, mode)
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

func TestWake_RealTavernFullnessReset(t *testing.T) {
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
	txProvider := database.NewTransactionProvider(db)
	eco, err := economy.NewService(charRepo, invRepo, economy.WithTransactionProvider(txProvider))
	if err != nil {
		t.Fatal(err)
	}

	realTavernRepo, err := database.NewTavernRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	failingRepo := &failingTavernStatusReader{TavernRepository: realTavernRepo}
	catalog, err := tavern.LoadDefaultCatalog()
	if err != nil {
		t.Fatal(err)
	}
	tavernSvc, err := tavern.NewService(catalog, failingRepo, charRepo, txProvider)
	if err != nil {
		t.Fatal(err)
	}

	homeSvc, err := home.NewService(homeRepo, charRepo, home.WithCharacterUpdater(charRepo), home.WithEconomy(eco), home.WithTimer(timers))
	if err != nil {
		t.Fatal(err)
	}
	homeSvc.SetFullnessResetter(tavernSvc)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	char, err := database.CreateTestCharacter(ctx, db, "WakeTavern")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx := context.Background()
		_ = timers.ReleaseLock(cleanupCtx, timer.CategoryAsleep, char.ID)
		_, _ = db.ExecContext(cleanupCtx, "DELETE FROM tavern_character_status WHERE character_id = ?", char.ID)
		_, _ = db.ExecContext(cleanupCtx, "DELETE FROM character_depots WHERE character_id = ?", char.ID)
		_, _ = db.ExecContext(cleanupCtx, "DELETE FROM characters WHERE id = ?", char.ID)
		_, _ = db.ExecContext(cleanupCtx, "DELETE FROM players WHERE id = ?", char.PlayerID)
	})

	// Setup depleted vitality and ready Asleep in Valkey
	char.Stats.HP, char.Stats.MP, char.Tired = 1, 0, 80
	if err := charRepo.Update(ctx, char); err != nil {
		t.Fatal(err)
	}
	if err := timers.SetLock(ctx, timer.CategoryAsleep, char.ID, time.Hour); err != nil {
		t.Fatal(err)
	}

	// Seed real SQL tavern status with meals and gold spent
	now := time.Now().UTC().Truncate(time.Second)
	seedStatus := tavern.TavernCharacterStatus{
		CharacterID:     char.ID,
		IsFull:          true,
		TotalMealsEaten: 7,
		TotalGoldSpent:  1234,
		LastEatenAt:     &now,
	}
	if err := realTavernRepo.UpsertCharacterStatus(ctx, seedStatus); err != nil {
		t.Fatal(err)
	}

	// 1. Inject storage failure on GetCharacterStatus
	sentinelErr := errors.New("injected storage read failure")
	failingRepo.getErr = sentinelErr

	res, err := homeSvc.Wake(ctx, char.ID)
	if !errors.Is(err, sentinelErr) {
		t.Fatalf("expected sentinelErr from Wake, got %v (res=%+v)", err, res)
	}

	// Assert pending recovery retained
	asleep, err := timers.IsLocked(ctx, timer.CategoryAsleep, char.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !asleep {
		t.Errorf("expected CategoryAsleep lock retained after hook failure")
	}

	// Assert prior vitality commit remains intact
	updatedChar, err := charRepo.FindByID(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updatedChar.Stats.HP != updatedChar.Stats.MaxHP || updatedChar.Stats.MP != updatedChar.Stats.MaxMP || updatedChar.Tired != 0 {
		t.Errorf("expected vitality recovery committed before hook failure: %+v", updatedChar)
	}

	// Assert historical fields in tavern_character_status remain unchanged (NO whole-status UPSERT zeros)
	persistedStatus, err := realTavernRepo.GetCharacterStatus(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !persistedStatus.IsFull || persistedStatus.TotalMealsEaten != 7 || persistedStatus.TotalGoldSpent != 1234 || persistedStatus.LastEatenAt == nil {
		t.Errorf("expected historical counters preserved, got: %+v", persistedStatus)
	}

	// 2. Healthy contrast: disable failure injection and retry Wake
	failingRepo.getErr = nil
	res, err = homeSvc.Wake(ctx, char.ID)
	if err != nil {
		t.Fatalf("expected successful Wake on healthy retry: %v", err)
	}
	if !res.Success {
		t.Errorf("expected Wake success = true")
	}

	// Fullness cleared, counters preserved
	finalStatus, err := realTavernRepo.GetCharacterStatus(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}
	if finalStatus.IsFull {
		t.Errorf("expected IsFull=false after successful Wake")
	}
	if finalStatus.TotalMealsEaten != 7 || finalStatus.TotalGoldSpent != 1234 {
		t.Errorf("expected counters preserved, got: %+v", finalStatus)
	}

	// CategoryAsleep lock released
	asleep, err = timers.IsLocked(ctx, timer.CategoryAsleep, char.ID)
	if err != nil {
		t.Fatal(err)
	}
	if asleep {
		t.Errorf("expected CategoryAsleep lock released after successful Wake")
	}
}

func TestSleepWake_CycleConsistency(t *testing.T) {
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
	txProvider := database.NewTransactionProvider(db)
	eco, err := economy.NewService(charRepo, invRepo, economy.WithTransactionProvider(txProvider))
	if err != nil {
		t.Fatal(err)
	}

	newChar := func(t *testing.T, ctx context.Context, name string) corecharacter.Character {
		t.Helper()
		char, err := database.CreateTestCharacter(ctx, db, name)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			cleanupCtx := context.Background()
			_ = timers.ReleaseLock(cleanupCtx, timer.CategorySleep, char.ID)
			_ = timers.ReleaseLock(cleanupCtx, timer.CategoryAsleep, char.ID)
			_ = timers.ReleaseLock(cleanupCtx, timer.CategoryWaking, char.ID)
			_ = timers.ReleaseLock(cleanupCtx, timer.CategoryDungeonOnce, char.ID)
			_, _ = db.ExecContext(cleanupCtx, "DELETE FROM character_depots WHERE character_id = ?", char.ID)
			_, _ = db.ExecContext(cleanupCtx, "DELETE FROM characters WHERE id = ?", char.ID)
			_, _ = db.ExecContext(cleanupCtx, "DELETE FROM players WHERE id = ?", char.PlayerID)
		})
		return char
	}

	t.Run("sleep_rejected_during_wake_and_preserves_next_cycle", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		char := newChar(t, ctx, "WakeCycle1")
		char.Stats.HP, char.Stats.MP, char.Tired = 1, 0, 80
		if err := charRepo.Update(ctx, char); err != nil {
			t.Fatal(err)
		}

		svc, err := home.NewService(homeRepo, charRepo, home.WithCharacterUpdater(charRepo), home.WithEconomy(eco), home.WithTimer(timers))
		if err != nil {
			t.Fatal(err)
		}

		// 1. Start first sleep cycle and simulate timer expiry.
		if _, err := svc.Sleep(ctx, char.ID, char.ID); err != nil {
			t.Fatalf("first Sleep: %v", err)
		}
		if err := timers.ReleaseLock(ctx, timer.CategorySleep, char.ID); err != nil {
			t.Fatalf("ReleaseLock(CategorySleep): %v", err)
		}

		// 2. Calling Sleep before Wake completes must return ErrAlreadySleeping.
		if _, err := svc.Sleep(ctx, char.ID, char.ID); !errors.Is(err, home.ErrAlreadySleeping) {
			t.Fatalf("Sleep while PendingWake active err = %v, want ErrAlreadySleeping", err)
		}

		// 3. Start first Wake and pause inside costume hook while racing a Sleep call.
		enteredHook := make(chan struct{})
		releaseHook := make(chan struct{})
		var once sync.Once
		svc.SetCostumeResetter(wakeCostumeHook(func(hookCtx context.Context, _ string) error {
			once.Do(func() { close(enteredHook) })
			select {
			case <-releaseHook:
				return nil
			case <-hookCtx.Done():
				return hookCtx.Err()
			}
		}))

		wakeErr, sleepErr := testutil.RunRace2(
			func() error {
				_, err := svc.Wake(ctx, char.ID)
				return err
			},
			func() error {
				select {
				case <-enteredHook:
				case <-ctx.Done():
					return ctx.Err()
				}
				_, err := svc.Sleep(ctx, char.ID, char.ID)
				close(releaseHook)
				return err
			},
		)
		if wakeErr != nil {
			t.Fatalf("first Wake err = %v", wakeErr)
		}
		if !errors.Is(sleepErr, home.ErrWakeInProgress) {
			t.Fatalf("Sleep during Wake err = %v, want ErrWakeInProgress", sleepErr)
		}

		// 4. Start second sleep cycle after first Wake completes; verify SQL pending_wake, duration, and asleep.
		svc.SetCostumeResetter(nil)
		afterWake, err := charRepo.FindByID(ctx, char.ID)
		if err != nil {
			t.Fatal(err)
		}
		afterWake.Stats.HP, afterWake.Stats.MP, afterWake.Tired = 15, 5, 60
		if err := charRepo.Update(ctx, afterWake); err != nil {
			t.Fatal(err)
		}

		if _, err := svc.Sleep(ctx, char.ID, char.ID); err != nil {
			t.Fatalf("second Sleep: %v", err)
		}
		afterSecondSleep, err := charRepo.FindByID(ctx, char.ID)
		if err != nil {
			t.Fatal(err)
		}
		if !afterSecondSleep.PendingWake {
			t.Fatalf("expected SQL PendingWake=true after second Sleep")
		}
		if rem, err := timers.GetRemainingLock(ctx, timer.CategorySleep, char.ID); err != nil || rem <= 0 {
			t.Fatalf("expected active CategorySleep duration lock, got rem=%v err=%v", rem, err)
		}
		if asleep, err := timers.IsLocked(ctx, timer.CategoryAsleep, char.ID); err != nil || !asleep {
			t.Fatalf("expected active CategoryAsleep lock, got %v err=%v", asleep, err)
		}

		if err := timers.ReleaseLock(ctx, timer.CategorySleep, char.ID); err != nil {
			t.Fatalf("ReleaseLock(CategorySleep) 2nd cycle: %v", err)
		}
		status, err := svc.GetSleepStatus(ctx, char.ID)
		if err != nil || !status.CanWake {
			t.Fatalf("status after 2nd Sleep expiry = %+v, err = %v; want CanWake=true", status, err)
		}
		res, err := svc.Wake(ctx, char.ID)
		if err != nil || !res.Success {
			t.Fatalf("second Wake = %+v, %v", res, err)
		}
		finalChar, err := charRepo.FindByID(ctx, char.ID)
		if err != nil {
			t.Fatal(err)
		}
		if finalChar.Stats.HP != finalChar.Stats.MaxHP || finalChar.Stats.MP != finalChar.Stats.MaxMP || finalChar.Tired != 0 || finalChar.PendingWake {
			t.Fatalf("finalChar after 2nd Wake = %+v; want full vitality and PendingWake=false", finalChar)
		}
	})

	t.Run("wake_aborts_if_sleep_lock_appears_during_hook", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		char := newChar(t, ctx, "WakeCycle2")
		char.Stats.HP, char.Stats.MP, char.Tired = 10, 5, 50
		char.PendingWake = true
		if err := charRepo.Update(ctx, char); err != nil {
			t.Fatal(err)
		}
		if err := timers.SetLock(ctx, timer.CategoryAsleep, char.ID, time.Hour); err != nil {
			t.Fatal(err)
		}
		if err := timers.SetLock(ctx, timer.CategoryDungeonOnce, char.ID, time.Hour); err != nil {
			t.Fatal(err)
		}

		svc, err := home.NewService(homeRepo, charRepo, home.WithCharacterUpdater(charRepo), home.WithEconomy(eco), home.WithTimer(timers))
		if err != nil {
			t.Fatal(err)
		}
		svc.SetFullnessResetter(wakeFullnessHook(func(hookCtx context.Context, id string) error {
			return timers.SetLock(hookCtx, timer.CategorySleep, id, time.Minute)
		}))

		if _, err := svc.Wake(ctx, char.ID); !errors.Is(err, home.ErrStillSleeping) {
			t.Fatalf("Wake err = %v, want ErrStillSleeping", err)
		}
		persisted, err := charRepo.FindByID(ctx, char.ID)
		if err != nil {
			t.Fatal(err)
		}
		if !persisted.PendingWake {
			t.Fatalf("expected PendingWake=true preserved in SQL")
		}
		if asleep, err := timers.IsLocked(ctx, timer.CategoryAsleep, char.ID); err != nil || !asleep {
			t.Fatalf("CategoryAsleep locked = %v, err = %v; want true", asleep, err)
		}
		if dungeonOnce, err := timers.IsLocked(ctx, timer.CategoryDungeonOnce, char.ID); err != nil || !dungeonOnce {
			t.Fatalf("CategoryDungeonOnce locked = %v, err = %v; want true", dungeonOnce, err)
		}
	})

	t.Run("concurrent_sleep_and_wake_stress", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		char := newChar(t, ctx, "WakeCycleStress")

		svc, err := home.NewService(homeRepo, charRepo, home.WithCharacterUpdater(charRepo), home.WithEconomy(eco), home.WithTimer(timers))
		if err != nil {
			t.Fatal(err)
		}

		cfg := testutil.GetStressConfig()
		res := testutil.RunConcurrentStressTest(t, cfg, func(worker, op int) error {
			if worker%2 == 0 {
				_, err := svc.Sleep(ctx, char.ID, char.ID)
				if err == nil {
					_ = timers.ReleaseLock(ctx, timer.CategorySleep, char.ID)
					return nil
				}
				if errors.Is(err, home.ErrAlreadySleeping) || errors.Is(err, home.ErrWakeInProgress) {
					return nil
				}
				return err
			}
			_, err := svc.Wake(ctx, char.ID)
			if err == nil || errors.Is(err, home.ErrStillSleeping) || errors.Is(err, home.ErrWakeInProgress) {
				return nil
			}
			return err
		})
		if res.Failures != 0 || res.Deadlocks != 0 {
			t.Fatalf("stress: %+v", res)
		}

		// Settle any active sleep duration and verify final Wake leaves a clean state.
		if err := timers.ReleaseLock(ctx, timer.CategorySleep, char.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := svc.Wake(ctx, char.ID); err != nil {
			t.Fatalf("settling Wake: %v", err)
		}
		finalChar, err := charRepo.FindByID(ctx, char.ID)
		if err != nil {
			t.Fatal(err)
		}
		if finalChar.PendingWake {
			t.Fatalf("expected PendingWake=false after settling Wake")
		}
		if asleep, err := timers.IsLocked(ctx, timer.CategoryAsleep, char.ID); err != nil || asleep {
			t.Fatalf("expected CategoryAsleep unlocked after settling Wake, got %v (err=%v)", asleep, err)
		}
	})
}
