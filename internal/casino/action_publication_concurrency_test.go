package casino_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/casino"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/testutil"
)

func TestActionPublication_ConcurrentWagers(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}
	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(100)
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	raw := openIsolatedTestValkey(t, 0)
	t.Cleanup(raw.Close)
	for _, game := range []casino.GameType{casino.GameTypeIndian, casino.GameTypeHighLow, casino.GameTypeDoppel} {
		t.Run(string(game), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()
			cfg := testutil.GetStressConfig()
			fixtures := make([]*publicationFixture, cfg.Workers)
			for i := range fixtures {
				fixtures[i] = newPublicationFixture(t, ctx, db, raw, game, 500)
			}
			result := testutil.RunConcurrentStressTest(t, cfg, func(worker, op int) error {
				err := func() error {
					f := fixtures[worker]
					if op > 0 {
						var err error
						switch game {
						case casino.GameTypeIndian:
							_, err = f.svc.StartIndianPoker(ctx, f.roomID, f.actors[0])
						case casino.GameTypeHighLow:
							_, err = f.svc.StartHighLow(ctx, f.roomID, f.actors[0])
						case casino.GameTypeDoppel:
							_, err = f.svc.StartDoppel(ctx, f.roomID, f.actors[0])
						}
						if err != nil {
							return err
						}
						if game != casino.GameTypeDoppel {
							for i, card := range []int{12, 0} {
								member, err := f.rooms.GetMember(ctx, f.roomID, f.actors[i])
								if err != nil {
									return err
								}
								member.Card = card
								if err := f.rooms.UpdateMember(ctx, *member); err != nil {
									return err
								}
							}
						}
					}
					f.fault.resetCalls()
					// Competing identical turns plus the opponent: Doppel may reselect, but never pays twice.
					errs := testutil.RunRace(
						func() error { return f.play(ctx, 0, false) },
						func() error { return f.play(ctx, 0, false) },
						func() error { return f.play(ctx, 1, false) },
					)
					for _, err := range errs {
						if err != nil && !errors.Is(err, casino.ErrAlreadyActed) && !errors.Is(err, casino.ErrGameNotInRound) {
							return err
						}
					}
					room, err := f.rooms.GetRoom(ctx, f.roomID)
					if err != nil {
						return err
					}
					if room.Round != 0 || room.Pot != 0 || room.WinnerCharacterID == nil {
						return fmt.Errorf("incomplete settlement: %+v", room)
					}
					for i, actor := range f.actors {
						acc, err := f.accounts.GetAccount(ctx, actor)
						if err != nil {
							return err
						}
						want := int64(500 - 10*(op+1))
						wins := 0
						if actor == *room.WinnerCharacterID {
							want = int64(500 + 10*(op+1))
							wins = op + 1
						}
						if acc.Coins != want {
							return fmt.Errorf("actor %d duplicate wager/payout: got %d want %d", i, acc.Coins, want)
						}
						ch, err := f.characters.FindByID(ctx, actor)
						if err != nil {
							return err
						}
						if ch.CasinoWins != wins {
							return fmt.Errorf("duplicate wins: got %d want %d", ch.CasinoWins, wins)
						}
					}
					a, err := f.accounts.GetAccount(ctx, f.actors[0])
					if err != nil {
						return err
					}
					b, err := f.accounts.GetAccount(ctx, f.actors[1])
					if err != nil {
						return err
					}
					if a.Coins+b.Coins != 1000 {
						return fmt.Errorf("coins not conserved: %d/%d", a.Coins, b.Coins)
					}
					return nil
				}()
				if err != nil {
					t.Logf("STRESS ERROR (worker=%d, op=%d): %v", worker, op, err)
				}
				return err
			})
			if result.Failures != 0 || result.Deadlocks != 0 {
				t.Fatalf("concurrent wagers failed: %+v", result)
			}
		})
	}
}
