package casino_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/casino"
	"github.com/witchcraze/party2re/internal/database"
)

// Fail every required store operation, including result reads after the final payload SET.
// Compare all persisted facts, then let the healthy opponent act without replaying the rejected command.
func TestActionPublication_FailureBoundaries(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}
	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	raw := openIsolatedTestValkey(t, 0)
	defer raw.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	for _, game := range []casino.GameType{casino.GameTypeIndian, casino.GameTypeHighLow, casino.GameTypeDoppel} {
		t.Run(string(game), func(t *testing.T) {
			for _, scenario := range []string{"first-action", "settlement", "elimination", "next-round", "reselection"} {
				if (scenario == "next-round" && game == casino.GameTypeDoppel) || (scenario == "reselection" && game != casino.GameTypeDoppel) {
					continue
				}
				t.Run(scenario, func(t *testing.T) {
					coinsB := int64(500)
					if scenario == "elimination" {
						coinsB = 10
					}
					fixture := newPublicationFixture(t, ctx, db, raw, game, coinsB)
					initial := fixture.state(t, ctx)
					prepare := func(t *testing.T) *publicationFixture {
						f := fixture
						f.restore(t, ctx, initial)
						if scenario != "first-action" {
							if err := f.play(ctx, 0, scenario == "next-round"); err != nil {
								t.Fatal(err)
							}
						}
						f.fault.resetCalls()
						return f
					}
					actor := 1
					if scenario == "first-action" || scenario == "reselection" {
						actor = 0
					}
					play := func(f *publicationFixture) error {
						if scenario == "reselection" {
							_, err := f.svc.PlayDoppelAction(ctx, f.roomID, f.actors[0], 2)
							return err
						}
						return f.play(ctx, actor, scenario == "next-round")
					}
					var calls []string
					t.Run("healthy", func(t *testing.T) {
						f := prepare(t)
						if err := play(f); err != nil {
							t.Fatal(err)
						}
						calls = f.fault.getCalls()
						f.assertApplied(t, ctx, scenario, coinsB, false)
						if scenario == "first-action" {
							if err := f.play(ctx, 1, false); err != nil {
								t.Fatal(err)
							}
							s := f.state(t, ctx)
							if s.coins != [2]int64{510, 490} || s.wins != [2]int{1, 0} || s.room.Round != 0 {
								t.Fatalf("bad healthy contrast: %+v", s)
							}
						}
					})
					lastSet := -1
					for i, call := range calls {
						if call == "valkey/SET" {
							lastSet = i
						}
					}
					if lastSet < 0 {
						t.Fatal("no authoritative payload write observed")
					}
					// Include secondary synchronization and result-read failures after publication.
					for fail := len(calls) - 1; fail >= 0; fail-- {
						t.Run(publicationFailureName(fail, calls[fail]), func(t *testing.T) {
							f := prepare(t)
							before := f.state(t, ctx)
							f.fault.resetCalls()
							f.fault.failAt = fail + 1
							err := play(f)
							f.fault.failAt = 0
							if !errors.Is(err, errPublicationFault) {
								t.Errorf("required store failure lost at %s: %v", calls[fail], err)
							}
							if fail > lastSet {
								f.assertApplied(t, ctx, scenario, coinsB, calls[fail] == "valkey/DEL")
								if calls[fail] == "valkey/ZADD" || calls[fail] == "valkey/DEL" {
									if !errors.Is(err, casino.ErrSecondarySync) {
										t.Fatalf("secondary failure misclassified: %v", err)
									}
								}
								return
							}
							f.assertUnchanged(t, ctx, before)
							if scenario == "first-action" {
								f.assertPendingOpponent(t, ctx)
							}
							if scenario == "reselection" {
								if err := f.play(ctx, 1, false); err != nil {
									t.Fatal(err)
								}
								s := f.state(t, ctx)
								if s.coins != [2]int64{510, 490} || s.wins != [2]int{1, 0} {
									t.Fatalf("rejected reselection changed winner: %+v", s)
								}
							}
						})
					}
					writes := 0
					for _, call := range calls {
						if call == "valkey/SET" {
							writes++
						}
					}
					if n := writes; n != 1 {
						t.Errorf("action published %d payloads, want one", n)
					}
				})
			}
		})
	}
}

func (f *publicationFixture) assertApplied(t *testing.T, ctx context.Context, scenario string, coinsB int64, mappingFailure bool) {
	t.Helper()
	s := f.state(t, ctx)
	switch scenario {
	case "first-action", "reselection":
		if s.coins != [2]int64{490, 500} || s.room.Pot != 10 || s.room.Round != 1 || s.wins != [2]int{} {
			t.Fatalf("unpaid/double-paid first selection: %+v", s)
		}
	case "next-round":
		if s.coins != [2]int64{490, 490} || s.room.Pot != 20 || s.room.Round != 2 || s.room.CurrentBet != 20 || s.wins != [2]int{} {
			t.Fatalf("bad next round: %+v", s)
		}
		for _, m := range s.members {
			if m.Action != "" {
				t.Fatalf("next-round action retained: %+v", m)
			}
		}
	default:
		if s.coins != [2]int64{510, coinsB - 10} || s.room.Pot != 0 || s.room.Round != 0 || s.wins != [2]int{1, 0} || s.room.WinnerCharacterID == nil || *s.room.WinnerCharacterID != f.actors[0] {
			t.Fatalf("bad settlement: %+v", s)
		}
		if scenario == "elimination" {
			if len(s.members) != 1 {
				t.Fatalf("loser retained: %+v", s.members)
			}
			if _, err := f.rooms.GetCharacterRoom(ctx, f.actors[1]); !mappingFailure && !errors.Is(err, casino.ErrRoomNotFound) {
				t.Fatalf("loser mapping retained: %v", err)
			}
		}
	}
}
