package challenge_test

import (
	"context"
	"errors"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/challenge"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/testutil"
)

func TestChallengeFinalizationIntegration(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}
	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo, err := database.NewChallengeRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	charRepo, err := database.NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	for _, scenario := range []string{"sequential", "concurrent", "rollback", "cleanup failure", "invalid status", "missing session", "wrong owner"} {
		t.Run(scenario, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()
			char, err := database.CreateTestCharacter(ctx, db, "Claim "+id.New()[:8])
			if err != nil {
				t.Fatal(err)
			}
			session := sampleChallengeSession(id.New(), char.ID, "0")
			session.CurrentRound = 2
			session.AccumulatedExp = 7
			session.AccumulatedGold = 23
			session.AccumulatedItems = []string{"potion_minor"}
			if scenario != "missing session" {
				if err := repo.SaveSession(ctx, session); err != nil {
					t.Fatal(err)
				}
			}
			session.Status = challenge.StatusDefeated
			finalize := func() error {
				return repo.FinalizeSession(ctx, session, 7, 23, session.AccumulatedItems, 1)
			}
			assertAwards := func(want int) {
				t.Helper()
				updated, err := charRepo.FindByID(ctx, char.ID)
				if err != nil {
					t.Fatal(err)
				}
				if updated.Money != char.Money+23*want || updated.Experience != char.Experience+7*want {
					t.Fatalf("awards applied incorrectly: money=%d exp=%d, want %d settlements", updated.Money, updated.Experience, want)
				}
				var count int
				if err := db.QueryRowContext(ctx, `SELECT
					(SELECT COUNT(*) FROM inventory_items WHERE character_id = ?) +
					(SELECT COUNT(*) FROM depot_items WHERE character_id = ?)`, char.ID, char.ID).Scan(&count); err != nil {
					t.Fatal(err)
				}
				if count != want {
					t.Fatalf("item count=%d, want %d", count, want)
				}
				record, err := repo.FindRecord(ctx, char.ID, "0")
				if err != nil {
					t.Fatal(err)
				}
				if want == 0 {
					if record != nil {
						t.Fatalf("unexpected record: %+v", record)
					}
				} else if record == nil || record.TotalAttempts != want || record.TotalVictories != 0 || record.HighestRound != 1 {
					t.Fatalf("incorrect record for %d settlements: %+v", want, record)
				}
			}

			switch scenario {
			case "sequential":
				if err := finalize(); err != nil {
					t.Fatal(err)
				}
				// A conflicting retry must not overwrite the first terminal snapshot.
				session.CurrentRound = 99
				if err := finalize(); !errors.Is(err, challenge.ErrSessionNotActive) {
					t.Fatalf("duplicate finalization: %v", err)
				}
				stored, err := repo.FindSessionByID(ctx, session.ID)
				if err != nil || stored.CurrentRound != 2 || stored.Status != challenge.StatusDefeated {
					t.Fatalf("terminal snapshot overwritten: %+v, %v", stored, err)
				}
				assertAwards(1)
				// A different run for the same character still settles normally.
				session.ID = id.New()
				session.Status = challenge.StatusActive
				if err := repo.SaveSession(ctx, session); err != nil {
					t.Fatal(err)
				}
				session.Status = challenge.StatusClaimed
				if err := finalize(); err != nil {
					t.Fatal(err)
				}
				if err := finalize(); !errors.Is(err, challenge.ErrSessionNotActive) {
					t.Fatalf("duplicate claimed session: %v", err)
				}
				assertAwards(2)
			case "concurrent":
				var claimed atomic.Int64
				result := testutil.RunConcurrentStressTest(t, testutil.GetStressConfig(), func(_, _ int) error {
					err := finalize()
					if err == nil {
						claimed.Add(1)
						return nil
					}
					if errors.Is(err, challenge.ErrSessionNotActive) {
						return nil
					}
					return err
				})
				if result.Failures != 0 || claimed.Load() != 1 {
					t.Fatalf("claims=%d failures=%d", claimed.Load(), result.Failures)
				}
				assertAwards(1)
			case "rollback":
				err := repo.FinalizeSession(ctx, session, 7, 23, []string{" "}, 1)
				if !errors.Is(err, coreitem.ErrInvalidInstance) {
					t.Fatalf("expected item error after currency update, got %v", err)
				}
				assertAwards(0)
				abort := errors.New("abort enclosing transaction")
				err = database.RunInTx(ctx, db, func(txCtx context.Context) error {
					if err := repo.FinalizeSession(txCtx, session, 7, 23, session.AccumulatedItems, 1); err != nil {
						return err
					}
					return abort
				})
				if !errors.Is(err, abort) {
					t.Fatal(err)
				}
				assertAwards(0)
				stored, err := repo.FindSessionByID(ctx, session.ID)
				if err != nil || stored.Status != challenge.StatusActive {
					t.Fatalf("claim did not roll back: %+v, %v", stored, err)
				}
				if err := finalize(); err != nil {
					t.Fatal(err)
				}
				assertAwards(1)
			case "cleanup failure":
				client := openTestValkey(t)
				defer client.Close()
				store, err := challenge.NewValkeySessionRepository(client)
				if err != nil {
					t.Fatal(err)
				}
				defer cleanupChallengeValkey(t, client, char.ID)
				session.Status = challenge.StatusActive
				if err := store.SaveActiveSession(ctx, session); err != nil {
					t.Fatal(err)
				}
				failingStore := newMockActiveStore(store)
				failingStore.deleteActiveSessionErr = errors.New("cleanup unavailable")
				svc, err := challenge.NewService(repo, charRepo, finalizationDefeatEngine{}, challenge.WithActiveSessionStore(failingStore))
				if err != nil {
					t.Fatal(err)
				}
				result, _, err := svc.AdvanceRound(ctx, char.ID, session.ID)
				if err != nil || result == nil || !result.SessionEnded {
					t.Fatalf("first settlement: %+v, %v", result, err)
				}
				buffer, err := store.GetActiveSession(ctx, char.ID)
				if err != nil || buffer == nil {
					t.Fatalf("cleanup failure must retain buffer: %+v, %v", buffer, err)
				}
				if _, _, err := svc.AdvanceRound(ctx, char.ID, session.ID); !errors.Is(err, challenge.ErrSessionNotActive) {
					t.Fatalf("retry after failed cleanup: %v", err)
				}
				assertAwards(1)
			case "invalid status":
				for _, status := range []challenge.SessionStatus{challenge.StatusActive, "invalid"} {
					session.Status = status
					if err := finalize(); !errors.Is(err, challenge.ErrSessionNotActive) {
						t.Fatalf("status %q: %v", status, err)
					}
				}
				assertAwards(0)
			case "missing session":
				if err := finalize(); !errors.Is(err, challenge.ErrSessionNotFound) {
					t.Fatalf("missing session: %v", err)
				}
				assertAwards(0)
			case "wrong owner":
				other, err := database.CreateTestCharacter(ctx, db, "Other "+id.New()[:8])
				if err != nil {
					t.Fatal(err)
				}
				session.CharacterID = other.ID
				if err := finalize(); !errors.Is(err, challenge.ErrForbidden) {
					t.Fatalf("wrong owner: %v", err)
				}
				assertAwards(0)
			}
		})
	}
}

type finalizationDefeatEngine struct{}

func (finalizationDefeatEngine) Resolve(req corebattle.Request) (corebattle.Result, error) {
	return corebattle.Result{Outcome: corebattle.OutcomeWin, WinnerID: req.Participants[1].ID}, nil
}
