package dungeon_test

import (
	"context"
	"os"
	"testing"
	"time"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/dungeon"
	"github.com/witchcraze/party2re/internal/testutil"
)

type staleEscapeKey struct{}

type pausedFinalizeRepository struct {
	dungeon.Repository
	entered chan struct{}
	resume  chan struct{}
}

func (r *pausedFinalizeRepository) FinalizeExpedition(ctx context.Context, h dungeon.DungeonExpeditionHistory, rec dungeon.CharacterDungeonRecord, c *corecharacter.Character, items []coreitem.Instance) error {
	if ctx.Value(staleEscapeKey{}) != nil {
		close(r.entered)
		select {
		case <-r.resume:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return r.Repository.FinalizeExpedition(ctx, h, rec, c, items)
}

func TestDungeonIntegration_StaleEscapePreservesReplacement(t *testing.T) {
	for _, failCleanup := range []bool{false, true} {
		name := "normal-cleanup"
		if failCleanup {
			name = "failed-cleanup-retry"
		}
		t.Run(name, func(t *testing.T) {
			if os.Getenv("PARTY2_DB_DSN") == "" {
				t.Skip("PARTY2_DB_DSN is not configured")
			}
			db, err := database.OpenFromEnvironment()
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { db.Close() })
			client := openTestValkey(t)
			t.Cleanup(client.Close)
			store, err := dungeon.NewValkeyExpeditionRepository(client)
			if err != nil {
				t.Fatal(err)
			}
			repo, err := database.NewDungeonRepository(db)
			if err != nil {
				t.Fatal(err)
			}
			chars, err := database.NewCharacterRepository(db)
			if err != nil {
				t.Fatal(err)
			}
			bank, err := database.NewBankRepository(db)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			char, err := database.CreateTestCharacterWithFunds(ctx, db, "Stale cleanup", 1000)
			if err != nil {
				t.Fatal(err)
			}
			follower, err := database.CreateTestCharacter(ctx, db, "Cleanup member")
			if err != nil {
				t.Fatal(err)
			}
			member := follower.ID
			t.Cleanup(func() {
				cleanupDungeonValkey(t, client, char.ID)
				if err := client.Do(context.Background(), client.B().Del().Key("party2:dungeon:{char:"+member+"}:member").Build()).Error(); err != nil {
					t.Error(err)
				}
				if err := chars.Delete(context.Background(), char.ID); err != nil {
					t.Error(err)
				}
				if _, err := db.ExecContext(context.Background(), "DELETE FROM players WHERE id = ?", char.PlayerID); err != nil {
					t.Error(err)
				}
				if err := chars.Delete(context.Background(), member); err != nil {
					t.Error(err)
				}
				if _, err := db.ExecContext(context.Background(), "DELETE FROM players WHERE id = ?", follower.PlayerID); err != nil {
					t.Error(err)
				}
			})
			if _, err := db.ExecContext(ctx, "UPDATE characters SET level = 10 WHERE id = ?", char.ID); err != nil {
				t.Fatal(err)
			}
			paused := &pausedFinalizeRepository{Repository: repo, entered: make(chan struct{}), resume: make(chan struct{})}
			flakyStore := &failOnceDeleteStore{inner: store}
			if failCleanup {
				flakyStore.failCalls.Store(1)
			}
			svc, err := dungeon.NewService(paused, chars, corebattle.Engine{}, dungeon.WithActiveExpeditionStore(flakyStore))
			if err != nil {
				t.Fatal(err)
			}
			old, err := svc.StartExpedition(ctx, char.ID, "dungeon-01")
			if err != nil {
				t.Fatal(err)
			}
			old.AccumulatedGold = 100
			if err := store.SaveActiveExpedition(ctx, *old); err != nil {
				t.Fatal(err)
			}
			var replacementID string
			// The race harness joins both workers even when a barrier assertion fails.
			errs := testutil.RunRace(func() error {
				_, err := svc.Escape(context.WithValue(ctx, staleEscapeKey{}, true), char.ID)
				return err
			}, func() error {
				defer close(paused.resume)
				select {
				case <-paused.entered:
				case <-ctx.Done():
					return ctx.Err()
				}
				if _, err := svc.Escape(ctx, char.ID); err != nil {
					return err
				}
				if failCleanup {
					if lingering, err := store.GetActiveExpedition(ctx, char.ID); err != nil || lingering == nil || lingering.ID != old.ID {
						t.Errorf("failed cleanup must retain the old run: %+v %v", lingering, err)
					}
					if _, err := svc.Escape(ctx, char.ID); err != nil {
						return err
					}
				}
				next, err := svc.StartExpedition(ctx, char.ID, "dungeon-01")
				if err != nil {
					return err
				}
				next.PosX, next.TurnsRemaining, next.AccumulatedGold = 2, 17, 77
				replacementID = next.ID
				next.Members = append(next.Members, dungeon.ExpeditionMember{CharacterID: member, Name: follower.Name, Level: follower.Level, CurrentHP: follower.Stats.HP, MaxHP: follower.Stats.MaxHP, JobID: follower.JobID})
				if err := store.SaveActiveExpedition(ctx, *next); err != nil {
					return err
				}
				// Save the replacement in SQL too: the fallback cleanup must preserve it.
				if err := repo.SaveActiveExpedition(ctx, *next); err != nil {
					return err
				}
				if saved, err := store.GetActiveExpedition(ctx, member); err != nil || saved == nil || saved.ID != next.ID || saved.AccumulatedGold != 77 {
					t.Errorf("replacement must be readable before old request resumes: %+v %v", saved, err)
				}
				_, err = bank.Deposit(ctx, char.ID, 25)
				return err
			})
			for _, err := range errs {
				if err != nil {
					t.Fatal(err)
				}
			}
			for _, source := range []interface {
				GetActiveExpedition(context.Context, string) (*dungeon.ActiveExpedition, error)
			}{store, repo} {
				next, err := source.GetActiveExpedition(ctx, char.ID)
				if err != nil || next == nil || next.ID != replacementID || next.PosX != 2 || next.TurnsRemaining != 17 || next.AccumulatedGold != 77 {
					t.Fatalf("replacement lost: %+v, err=%v", next, err)
				}
			}
			next, err := store.GetActiveExpedition(ctx, member)
			if err != nil || next == nil || next.ID != replacementID {
				t.Fatalf("replacement member index lost: %+v, err=%v", next, err)
			}
			history, err := repo.GetHistory(ctx, char.ID, 10)
			if err != nil || len(history) != 1 || history[0].ID != old.ID || history[0].GoldReward != 100 {
				t.Fatalf("old run must settle once: %+v, err=%v", history, err)
			}
			if _, err := svc.Escape(ctx, char.ID); err != nil {
				t.Fatalf("replacement rewards must remain payable: %v", err)
			}
			after, err := chars.FindByID(ctx, char.ID)
			if err != nil || after.Money != 1152 || after.Deposit != 25 {
				t.Fatalf("wallet must retain both payouts and bank update: %+v, err=%v", after, err)
			}
			history, err = repo.GetHistory(ctx, char.ID, 10)
			if err != nil || len(history) != 2 {
				t.Fatalf("expected one history per run: %+v, err=%v", history, err)
			}
			if active, err := store.GetActiveExpedition(ctx, member); err != nil || active != nil {
				t.Fatalf("normal cleanup must release membership: %+v, err=%v", active, err)
			}
		})
	}
}
