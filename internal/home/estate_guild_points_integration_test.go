package home_test

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/economy"
	"github.com/witchcraze/party2re/internal/home"
	"github.com/witchcraze/party2re/internal/testutil"
)

// canceledReaderGP injects context cancellation during AddGuildPoints reader lookup.
type canceledReaderGP struct {
	real *database.GuildRepository
}

func (r *canceledReaderGP) AddGuildPoints(ctx context.Context, charID string, points int) error {
	canceledCtx, cancel := context.WithCancel(ctx)
	cancel()
	return r.real.AddGuildPoints(canceledCtx, charID, points)
}

// canceledUpdateGP allows GetGuildByCharacter to succeed but cancels during AddPoints.
type canceledUpdateGP struct {
	real *database.GuildRepository
}

func (u *canceledUpdateGP) AddGuildPoints(ctx context.Context, charID string, points int) error {
	g, _, err := u.real.GetGuildByCharacter(ctx, charID)
	if err != nil {
		return err
	}
	canceledCtx, cancel := context.WithCancel(ctx)
	cancel()
	return u.real.AddPoints(canceledCtx, g.ID, int64(points))
}

// failSaveHomeRepo wraps HomeRepository to simulate failure on SaveHome.
type failSaveHomeRepo struct {
	*database.HomeRepository
	saveErr error
}

func (f *failSaveHomeRepo) SaveHome(ctx context.Context, h home.CharacterHome) error {
	if f.saveErr != nil {
		return f.saveErr
	}
	return f.HomeRepository.SaveHome(ctx, h)
}

// transientFailGP simulates a transient failure on the first call and succeeds thereafter.
type transientFailGP struct {
	real   *database.GuildRepository
	failed atomic.Bool
}

func (t *transientFailGP) AddGuildPoints(ctx context.Context, charID string, points int) error {
	if t.failed.CompareAndSwap(false, true) {
		canceledCtx, cancel := context.WithCancel(ctx)
		cancel()
		return t.real.AddGuildPoints(canceledCtx, charID, points)
	}
	return t.real.AddGuildPoints(ctx, charID, points)
}

func TestPostReviewSQLHouseGPMutationFailure(t *testing.T) {
	db := homeIsolatedDB(t)
	ctx := context.Background()

	realHomeRepo, err := database.NewHomeRepository(db)
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
	guildRepo, err := database.NewGuildRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	txProvider := database.NewTransactionProvider(db)
	eco, err := economy.NewService(charRepo, invRepo, economy.WithTransactionProvider(txProvider))
	if err != nil {
		t.Fatal(err)
	}

	createGuildWithMember := func(t *testing.T, charID string, initialPoints int64) string {
		t.Helper()
		guildID := fmt.Sprintf("guild_%016x", time.Now().UnixNano())
		_, err := db.ExecContext(ctx, `
			INSERT INTO guilds (id, name, leader_character_id, points, notice, color, created_at, updated_at)
			VALUES (?, ?, ?, ?, 'Notice', '#112233', NOW(), NOW())
		`, guildID, "Guild_"+guildID, charID, initialPoints)
		if err != nil {
			t.Fatalf("failed to insert test guild: %v", err)
		}
		_, err = db.ExecContext(ctx, `
			INSERT INTO guild_members (guild_id, character_id, role, title, is_pending, joined_at)
			VALUES (?, ?, 'leader', 'リーダー', FALSE, NOW())
		`, guildID, charID)
		if err != nil {
			t.Fatalf("failed to insert test guild member: %v", err)
		}
		return guildID
	}

	getGuildPoints := func(t *testing.T, guildID string) int64 {
		t.Helper()
		var points int64
		err := db.QueryRowContext(ctx, `SELECT points FROM guilds WHERE id = ?`, guildID).Scan(&points)
		if err != nil {
			t.Fatalf("failed to read guild points: %v", err)
		}
		return points
	}

	hasActiveHome := func(t *testing.T, charID string) bool {
		t.Helper()
		var count int
		err := db.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM character_homes WHERE character_id = ? AND expires_at > NOW()
		`, charID).Scan(&count)
		if err != nil {
			t.Fatalf("failed to query character home count: %v", err)
		}
		return count > 0
	}

	t.Run("GP prerequisite read failure rolls back money, house, and GP", func(t *testing.T) {
		char, err := database.CreateTestCharacterWithFunds(ctx, db, "GPRollbackRead", 10000)
		if err != nil {
			t.Fatal(err)
		}
		guildID := createGuildWithMember(t, char.ID, 0)

		faultGP := &canceledReaderGP{real: guildRepo}
		svc, err := home.NewService(
			realHomeRepo,
			charRepo,
			home.WithEconomy(eco),
			home.WithGuildPoints(faultGP),
		)
		if err != nil {
			t.Fatal(err)
		}

		_, err = svc.BuildHouse(ctx, char.ID, "town1", "001")
		if err == nil {
			t.Fatal("expected error from BuildHouse on GP read failure, got nil")
		}
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context.Canceled, got %v", err)
		}

		// Verify database rollback: money conserved, no home saved, GP unchanged
		reloadedChar, err := charRepo.FindByID(ctx, char.ID)
		if err != nil {
			t.Fatal(err)
		}
		if reloadedChar.Money != 10000 {
			t.Errorf("expected money to remain 10000, got %d", reloadedChar.Money)
		}
		if hasActiveHome(t, char.ID) {
			t.Error("expected no active home to be committed after GP reader failure")
		}
		if pts := getGuildPoints(t, guildID); pts != 0 {
			t.Errorf("expected guild points to remain 0, got %d", pts)
		}
	})

	t.Run("GP update SQL failure rolls back money, house, and GP", func(t *testing.T) {
		char, err := database.CreateTestCharacterWithFunds(ctx, db, "GPRollbackUpdate", 10000)
		if err != nil {
			t.Fatal(err)
		}
		guildID := createGuildWithMember(t, char.ID, 0)

		faultGP := &canceledUpdateGP{real: guildRepo}
		svc, err := home.NewService(
			realHomeRepo,
			charRepo,
			home.WithEconomy(eco),
			home.WithGuildPoints(faultGP),
		)
		if err != nil {
			t.Fatal(err)
		}

		_, err = svc.BuildHouse(ctx, char.ID, "town1", "001")
		if err == nil {
			t.Fatal("expected error from BuildHouse on GP update failure, got nil")
		}
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context.Canceled, got %v", err)
		}

		// Verify database rollback
		reloadedChar, err := charRepo.FindByID(ctx, char.ID)
		if err != nil {
			t.Fatal(err)
		}
		if reloadedChar.Money != 10000 {
			t.Errorf("expected money to remain 10000, got %d", reloadedChar.Money)
		}
		if hasActiveHome(t, char.ID) {
			t.Error("expected no active home to be committed after GP update failure")
		}
		if pts := getGuildPoints(t, guildID); pts != 0 {
			t.Errorf("expected guild points to remain 0, got %d", pts)
		}
	})

	t.Run("SaveHome failure rolls back GP and money", func(t *testing.T) {
		char, err := database.CreateTestCharacterWithFunds(ctx, db, "SaveHomeRollback", 10000)
		if err != nil {
			t.Fatal(err)
		}
		guildID := createGuildWithMember(t, char.ID, 0)

		saveErr := errors.New("simulated save home failure")
		faultHomeRepo := &failSaveHomeRepo{
			HomeRepository: realHomeRepo,
			saveErr:        saveErr,
		}

		svc, err := home.NewService(
			faultHomeRepo,
			charRepo,
			home.WithEconomy(eco),
			home.WithGuildPoints(guildRepo),
		)
		if err != nil {
			t.Fatal(err)
		}

		_, err = svc.BuildHouse(ctx, char.ID, "town1", "001")
		if err == nil {
			t.Fatal("expected error from BuildHouse on SaveHome failure, got nil")
		}
		if !errors.Is(err, saveErr) {
			t.Fatalf("expected error %v, got %v", saveErr, err)
		}

		// Verify database rollback: GP addition done prior to SaveHome was rolled back
		reloadedChar, err := charRepo.FindByID(ctx, char.ID)
		if err != nil {
			t.Fatal(err)
		}
		if reloadedChar.Money != 10000 {
			t.Errorf("expected money to remain 10000, got %d", reloadedChar.Money)
		}
		if hasActiveHome(t, char.ID) {
			t.Error("expected no active home to be committed after SaveHome failure")
		}
		if pts := getGuildPoints(t, guildID); pts != 0 {
			t.Errorf("expected guild points to be rolled back to 0, got %d", pts)
		}
	})

	t.Run("healthy contrast with guild awards GP and commits house purchase", func(t *testing.T) {
		char, err := database.CreateTestCharacterWithFunds(ctx, db, "HealthyContrast", 10000)
		if err != nil {
			t.Fatal(err)
		}
		guildID := createGuildWithMember(t, char.ID, 0)

		svc, err := home.NewService(
			realHomeRepo,
			charRepo,
			home.WithEconomy(eco),
			home.WithGuildPoints(guildRepo),
		)
		if err != nil {
			t.Fatal(err)
		}

		res, err := svc.BuildHouse(ctx, char.ID, "town1", "001")
		if err != nil {
			t.Fatalf("BuildHouse failed: %v", err)
		}
		if res.TownID != "town1" || res.HouseStyle != "001" {
			t.Errorf("unexpected build result: %+v", res)
		}

		// Verify database persistence
		reloadedChar, err := charRepo.FindByID(ctx, char.ID)
		if err != nil {
			t.Fatal(err)
		}
		if reloadedChar.Money != 9500 {
			t.Errorf("expected money 9500, got %d", reloadedChar.Money)
		}
		if !hasActiveHome(t, char.ID) {
			t.Error("expected active home to be saved")
		}
		if pts := getGuildPoints(t, guildID); pts != 50 {
			t.Errorf("expected 50 guild points in DB, got %d", pts)
		}
	})

	t.Run("unaffiliated character purchases house without error or GP", func(t *testing.T) {
		charNoGuild, err := database.CreateTestCharacterWithFunds(ctx, db, "NoGuildHero", 10000)
		if err != nil {
			t.Fatal(err)
		}

		svc, err := home.NewService(
			realHomeRepo,
			charRepo,
			home.WithEconomy(eco),
			home.WithGuildPoints(guildRepo),
		)
		if err != nil {
			t.Fatal(err)
		}

		res, err := svc.BuildHouse(ctx, charNoGuild.ID, "town2", "005")
		if err != nil {
			t.Fatalf("BuildHouse for unaffiliated character failed: %v", err)
		}
		if res.TownID != "town2" {
			t.Errorf("expected town2, got %s", res.TownID)
		}

		reloadedChar, err := charRepo.FindByID(ctx, charNoGuild.ID)
		if err != nil {
			t.Fatal(err)
		}
		// town2 costs 1500G
		if reloadedChar.Money != 8500 {
			t.Errorf("expected 8500 gold, got %d", reloadedChar.Money)
		}
		if !hasActiveHome(t, charNoGuild.ID) {
			t.Error("expected active home to be saved for unaffiliated character")
		}
	})

	t.Run("transient failure followed by retry applies purchase and GP exactly once", func(t *testing.T) {
		char, err := database.CreateTestCharacterWithFunds(ctx, db, "TransientRetry", 10000)
		if err != nil {
			t.Fatal(err)
		}
		guildID := createGuildWithMember(t, char.ID, 0)

		transientGP := &transientFailGP{real: guildRepo}
		svc, err := home.NewService(
			realHomeRepo,
			charRepo,
			home.WithEconomy(eco),
			home.WithGuildPoints(transientGP),
		)
		if err != nil {
			t.Fatal(err)
		}

		// Attempt 1: transient failure
		_, err = svc.BuildHouse(ctx, char.ID, "town1", "001")
		if err == nil {
			t.Fatal("expected failure on first attempt")
		}

		// Verify state untouched after attempt 1
		c1, err := charRepo.FindByID(ctx, char.ID)
		if err != nil {
			t.Fatal(err)
		}
		if c1.Money != 10000 {
			t.Errorf("expected money to remain 10000, got %d", c1.Money)
		}
		if hasActiveHome(t, char.ID) {
			t.Error("expected no home after failed first attempt")
		}
		if pts := getGuildPoints(t, guildID); pts != 0 {
			t.Errorf("expected 0 GP after failed attempt, got %d", pts)
		}

		// Attempt 2: retry succeeds
		res, err := svc.BuildHouse(ctx, char.ID, "town1", "001")
		if err != nil {
			t.Fatalf("retry BuildHouse failed: %v", err)
		}
		if res.TownID != "town1" {
			t.Errorf("expected town1, got %s", res.TownID)
		}

		// Verify state correctly applied once
		c2, err := charRepo.FindByID(ctx, char.ID)
		if err != nil {
			t.Fatal(err)
		}
		if c2.Money != 9500 {
			t.Errorf("expected 9500 money after successful retry, got %d", c2.Money)
		}
		if !hasActiveHome(t, char.ID) {
			t.Error("expected home to be saved after successful retry")
		}
		if pts := getGuildPoints(t, guildID); pts != 50 {
			t.Errorf("expected 50 GP after successful retry, got %d", pts)
		}

		// Attempt 3: repeat attempt while owning active house rejected with ErrAlreadyOwnsHouse
		_, err = svc.BuildHouse(ctx, char.ID, "town1", "002")
		if !errors.Is(err, home.ErrAlreadyOwnsHouse) {
			t.Errorf("expected ErrAlreadyOwnsHouse on repeated purchase, got %v", err)
		}
		// Confirm money and GP untouched
		c3, err := charRepo.FindByID(ctx, char.ID)
		if err != nil {
			t.Fatal(err)
		}
		if c3.Money != 9500 {
			t.Errorf("expected money to remain 9500, got %d", c3.Money)
		}
		if pts := getGuildPoints(t, guildID); pts != 50 {
			t.Errorf("expected GP to remain 50, got %d", pts)
		}
	})

	t.Run("all 4 towns award cycle_days * 10 GP in real DB", func(t *testing.T) {
		townCases := []struct {
			townID string
			style  string
			price  int
			wantGP int64
		}{
			{"town1", "001", 500, 50},
			{"town2", "005", 1500, 100},
			{"town3", "013", 3000, 150},
			{"town4", "021", 5000, 200},
		}

		for _, tc := range townCases {
			t.Run(tc.townID, func(t *testing.T) {
				char, err := database.CreateTestCharacterWithFunds(ctx, db, "TownGP_"+tc.townID, 10000)
				if err != nil {
					t.Fatal(err)
				}
				gID := createGuildWithMember(t, char.ID, 0)

				svc, err := home.NewService(
					realHomeRepo,
					charRepo,
					home.WithEconomy(eco),
					home.WithGuildPoints(guildRepo),
				)
				if err != nil {
					t.Fatal(err)
				}

				res, err := svc.BuildHouse(ctx, char.ID, tc.townID, tc.style)
				if err != nil {
					t.Fatalf("BuildHouse %s failed: %v", tc.townID, err)
				}
				if res.TownID != tc.townID {
					t.Errorf("expected town %s, got %s", tc.townID, res.TownID)
				}

				c, err := charRepo.FindByID(ctx, char.ID)
				if err != nil {
					t.Fatal(err)
				}
				if c.Money != 10000-tc.price {
					t.Errorf("expected money %d, got %d", 10000-tc.price, c.Money)
				}
				if pts := getGuildPoints(t, gID); pts != tc.wantGP {
					t.Errorf("expected %d GP for %s, got %d", tc.wantGP, tc.townID, pts)
				}
			})
		}
	})

	t.Run("concurrent house purchases conserve assets and respect town capacity", func(t *testing.T) {
		// Town1 has MaxHouses = 10. We spawn 15 characters belonging to the same guild.
		const totalContenders = 15
		chars := make([]corecharacter.Character, totalContenders)
		guildID := fmt.Sprintf("stress_g_%016x", time.Now().UnixNano())

		for i := 0; i < totalContenders; i++ {
			c, err := database.CreateTestCharacterWithFunds(ctx, db, fmt.Sprintf("StressBuilder%02d", i), 10000)
			if err != nil {
				t.Fatal(err)
			}
			chars[i] = c
			if i == 0 {
				_, err = db.ExecContext(ctx, `
					INSERT INTO guilds (id, name, leader_character_id, points, notice, color, created_at, updated_at)
					VALUES (?, ?, ?, 0, 'StressGuild', '#112233', NOW(), NOW())
				`, guildID, "StressGuild", c.ID)
				if err != nil {
					t.Fatal(err)
				}
			}
			_, err = db.ExecContext(ctx, `
				INSERT INTO guild_members (guild_id, character_id, role, title, is_pending, joined_at)
				VALUES (?, ?, 'member', 'メンバー', FALSE, NOW())
			`, guildID, c.ID)
			if err != nil {
				t.Fatal(err)
			}
		}

		svc, err := home.NewService(
			realHomeRepo,
			charRepo,
			home.WithEconomy(eco),
			home.WithGuildPoints(guildRepo),
		)
		if err != nil {
			t.Fatal(err)
		}

		var successes int64
		var capacityRejections int64

		stressCfg := testutil.ConcurrencyStressConfig{
			Workers:      5,
			OpsPerWorker: 3,
		}

		var opIndex int64
		stressRes := testutil.RunConcurrentStressTest(t, stressCfg, func(workerID int, op int) error {
			idx := atomic.AddInt64(&opIndex, 1) - 1
			if idx >= int64(len(chars)) {
				return nil
			}
			charID := chars[idx].ID

			_, err := svc.BuildHouse(ctx, charID, "town1", "001")
			if err != nil {
				if errors.Is(err, home.ErrTownMaxHousesReached) {
					atomic.AddInt64(&capacityRejections, 1)
					return nil
				}
				return err
			}
			atomic.AddInt64(&successes, 1)
			return nil
		})

		if stressRes.Deadlocks > 0 {
			t.Fatalf("deadlocks encountered during concurrent house construction: %d", stressRes.Deadlocks)
		}

		// Town1 capacity is 10
		if successes > 10 {
			t.Fatalf("expected at most 10 successes in town1, got %d", successes)
		}
		if successes+capacityRejections != int64(totalContenders) {
			t.Errorf("expected sum of successes (%d) and rejections (%d) to equal %d",
				successes, capacityRejections, totalContenders)
		}

		// Verify asset conservation across all characters
		var totalDeducted int
		for _, c := range chars {
			reloaded, err := charRepo.FindByID(ctx, c.ID)
			if err != nil {
				t.Fatal(err)
			}
			hasHome := hasActiveHome(t, c.ID)
			if hasHome {
				if reloaded.Money != 9500 {
					t.Errorf("character %s with house has money %d, want 9500", c.ID, reloaded.Money)
				}
				totalDeducted += 500
			} else {
				if reloaded.Money != 10000 {
					t.Errorf("character %s without house has money %d, want 10000", c.ID, reloaded.Money)
				}
			}
		}

		expectedTotalDeducted := int(successes) * 500
		if totalDeducted != expectedTotalDeducted {
			t.Errorf("total deducted = %d, want %d", totalDeducted, expectedTotalDeducted)
		}

		// Verify guild points conservation
		expectedGP := successes * 50
		if pts := getGuildPoints(t, guildID); pts != expectedGP {
			t.Errorf("guild points = %d, want %d", pts, expectedGP)
		}
	})
}
