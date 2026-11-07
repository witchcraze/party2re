package casino_test

import (
	"context"
	"errors"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/casino"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/id"
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
						f.resetHooks()
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
						f.assertApplied(t, ctx, scenario, coinsB, false, false)
						if scenario == "first-action" {
							if err := f.play(ctx, 1, false); err != nil {
								t.Fatal(err)
							}
							s := f.state(t, ctx)
							if s.coins != [2]int64{510, 490} || s.wins != [2]int{1, 0} || s.room.Round != 0 {
								t.Fatalf("bad healthy contrast: %+v", s)
							}
							wantHooks := []publicationHookCall{{CharacterID: f.actors[0], GameName: f.hookGameName()}}
							if got := f.getHooks(); !reflect.DeepEqual(got, wantHooks) {
								t.Fatalf("healthy follow-up hooks = %+v, want %+v", got, wantHooks)
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
							f.resetHooks()
							f.fault.failAt = fail + 1
							err := play(f)
							f.fault.failAt = 0
							if !errors.Is(err, errPublicationFault) {
								t.Errorf("required store failure lost at %s: %v", calls[fail], err)
							}
							if fail > lastSet {
								secondaryFailure := calls[fail] == "valkey/ZADD" || calls[fail] == "valkey/DEL"
								f.assertApplied(t, ctx, scenario, coinsB, calls[fail] == "valkey/DEL", secondaryFailure)
								if secondaryFailure {
									if !errors.Is(err, casino.ErrSecondarySync) {
										t.Fatalf("secondary failure misclassified: %v", err)
									}
								} else if scenario == "settlement" || scenario == "elimination" {
									if _, err := f.rooms.GetRoom(ctx, f.roomID); err != nil {
										t.Fatal(err)
									}
									if err := play(f); !errors.Is(err, casino.ErrGameNotInRound) {
										t.Fatalf("replay after settled response-read fault = %v, want ErrGameNotInRound", err)
									}
									f.assertApplied(t, ctx, scenario, coinsB, false, false)
								}
								return
							}
							if got := f.getHooks(); len(got) != 0 {
								t.Fatalf("rollback invoked winner hook: %+v", got)
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

func (f *publicationFixture) assertApplied(t *testing.T, ctx context.Context, scenario string, coinsB int64, mappingFailure, secondaryFailure bool) {
	t.Helper()
	s := f.state(t, ctx)
	gotHooks := f.getHooks()
	switch scenario {
	case "first-action", "reselection":
		if s.coins != [2]int64{490, 500} || s.room.Pot != 10 || s.room.Round != 1 || s.wins != [2]int{} {
			t.Fatalf("unpaid/double-paid first selection: %+v", s)
		}
		if len(gotHooks) != 0 {
			t.Fatalf("non-terminal action invoked hook: %+v", gotHooks)
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
		if len(gotHooks) != 0 {
			t.Fatalf("next-round transition invoked hook: %+v", gotHooks)
		}
	default:
		if s.coins != [2]int64{510, coinsB - 10} || s.room.Pot != 0 || s.room.Round != 0 || s.wins != [2]int{1, 0} || s.room.WinnerCharacterID == nil || *s.room.WinnerCharacterID != f.actors[0] {
			t.Fatalf("bad settlement: %+v", s)
		}
		var wantHooks []publicationHookCall
		if !secondaryFailure {
			wantHooks = []publicationHookCall{{CharacterID: f.actors[0], GameName: f.hookGameName()}}
		}
		if !reflect.DeepEqual(gotHooks, wantHooks) {
			t.Fatalf("settlement hooks = %+v, want %+v", gotHooks, wantHooks)
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

func TestActionPublication_MultipleWinnersReadFailure(t *testing.T) {
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
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	for _, game := range []casino.GameType{casino.GameTypeHighLow, casino.GameTypeDoppel} {
		t.Run(string(game), func(t *testing.T) {
			var healthyCalls []string
			for _, failRead := range []bool{false, true} {
				mode := "healthy"
				if failRead {
					mode = "read-fault"
				}
				t.Run(mode, func(t *testing.T) {
					fault := &publicationFault{}
					accounts, err := database.NewCasinoRepository(db)
					if err != nil {
						t.Fatal(err)
					}
					characters, err := database.NewCharacterRepository(db)
					if err != nil {
						t.Fatal(err)
					}
					client := &publicationValkey{Client: raw, fault: fault}
					rooms, err := casino.NewValkeyRoomRepository(client, casino.WithValkeyCharacterRepository(characters))
					if err != nil {
						t.Fatal(err)
					}
					svc, err := casino.NewService(
						publicationAccounts{Repository: accounts, fault: fault},
						casino.WithCharacterRepository(publicationCharacters{CharacterRepository: characters, fault: fault}),
						casino.WithTransactionProvider(database.NewTransactionProvider(db)),
						casino.WithRoomRepository(rooms),
					)
					if err != nil {
						t.Fatal(err)
					}
					var hooks []publicationHookCall
					svc.SetGamePlayedHook(func(_ context.Context, charID, gameName string) error {
						hooks = append(hooks, publicationHookCall{CharacterID: charID, GameName: gameName})
						return nil
					})

					var actors [3]string
					for i := range actors {
						ch, err := database.CreateTestCharacterWithFunds(ctx, db, "MultiWin", 10000)
						if err != nil {
							t.Fatal(err)
						}
						actors[i] = ch.ID
						t.Cleanup(func() {
							for _, query := range []string{"DELETE FROM casino_accounts WHERE character_id = ?", "DELETE FROM character_depots WHERE character_id = ?", "DELETE FROM characters WHERE id = ?"} {
								if _, err := db.ExecContext(context.Background(), query, ch.ID); err != nil {
									t.Error(err)
								}
							}
							if _, err := db.ExecContext(context.Background(), "DELETE FROM players WHERE id = ?", ch.PlayerID); err != nil {
								t.Error(err)
							}
						})
						if _, _, err := svc.ExchangeGoldToCoins(ctx, ch.ID, 500); err != nil {
							t.Fatal(err)
						}
					}

					detail, err := svc.CreateRoom(ctx, actors[0], casino.CreateRoomRequest{
						Name: "Multi-" + id.New()[:8], GameType: game, Speed: casino.SpeedFast, MaxPlayers: 3, Rate: 10,
					})
					if err != nil {
						t.Fatal(err)
					}
					roomID := detail.Room.ID
					client.roomID = roomID
					t.Cleanup(func() {
						fault.failAt = 0
						if err := rooms.DeleteRoom(context.Background(), roomID); err != nil {
							t.Error(err)
						}
					})
					for _, joiner := range actors[1:] {
						if _, err := svc.JoinRoom(ctx, roomID, joiner, "", 0); err != nil {
							t.Fatal(err)
						}
					}

					var wantHooks []publicationHookCall
					var wantCoins [3]int64
					var wantWins [3]int

					switch game {
					case casino.GameTypeHighLow:
						if _, err := svc.StartHighLow(ctx, roomID, actors[0]); err != nil {
							t.Fatal(err)
						}
						for i, card := range []int{12, 0, 6} {
							m, err := rooms.GetMember(ctx, roomID, actors[i])
							if err != nil {
								t.Fatal(err)
							}
							m.Card = card
							if err := rooms.UpdateMember(ctx, *m); err != nil {
								t.Fatal(err)
							}
						}
						if _, err := svc.PlayHighLowAction(ctx, roomID, actors[0], casino.HighLowActionHigh); err != nil {
							t.Fatal(err)
						}
						if _, err := svc.PlayHighLowAction(ctx, roomID, actors[1], casino.HighLowActionLow); err != nil {
							t.Fatal(err)
						}
						fault.resetCalls()
						hooks = nil
						if failRead {
							if len(healthyCalls) == 0 || healthyCalls[len(healthyCalls)-1] != "valkey/GET" {
								t.Fatalf("unexpected healthy call list: %v", healthyCalls)
							}
							fault.failAt = len(healthyCalls)
						}
						_, err = svc.PlayHighLowAction(ctx, roomID, actors[2], casino.HighLowActionHigh)
						wantHooks = []publicationHookCall{
							{CharacterID: actors[0], GameName: "highlow"},
							{CharacterID: actors[1], GameName: "highlow"},
						}
						wantCoins = [3]int64{505, 505, 490}
						wantWins = [3]int{1, 1, 0}

					case casino.GameTypeDoppel:
						if _, err := svc.StartDoppel(ctx, roomID, actors[0]); err != nil {
							t.Fatal(err)
						}
						if _, err := svc.PlayDoppelAction(ctx, roomID, actors[0], 0); err != nil {
							t.Fatal(err)
						}
						if _, err := svc.PlayDoppelAction(ctx, roomID, actors[1], 0); err != nil {
							t.Fatal(err)
						}
						fault.resetCalls()
						hooks = nil
						if failRead {
							if len(healthyCalls) == 0 || healthyCalls[len(healthyCalls)-1] != "valkey/GET" {
								t.Fatalf("unexpected healthy call list: %v", healthyCalls)
							}
							fault.failAt = len(healthyCalls)
						}
						_, err = svc.PlayDoppelAction(ctx, roomID, actors[2], 0)
						wantHooks = []publicationHookCall{
							{CharacterID: actors[1], GameName: "doppel"},
							{CharacterID: actors[2], GameName: "doppel"},
						}
						wantCoins = [3]int64{490, 505, 505}
						wantWins = [3]int{0, 1, 1}
					}

					fault.failAt = 0
					if failRead {
						if !errors.Is(err, errPublicationFault) {
							t.Fatalf("expected injected response-read error, got %v (calls=%v)", err, fault.getCalls())
						}
					} else {
						if err != nil {
							t.Fatalf("unexpected healthy error: %v", err)
						}
						healthyCalls = fault.getCalls()
					}

					if !reflect.DeepEqual(hooks, wantHooks) {
						t.Fatalf("multi-winner hooks = %+v, want %+v", hooks, wantHooks)
					}
					for i, actor := range actors {
						acc, err := accounts.GetAccount(ctx, actor)
						if err != nil {
							t.Fatal(err)
						}
						ch, err := characters.FindByID(ctx, actor)
						if err != nil {
							t.Fatal(err)
						}
						if acc.Coins != wantCoins[i] || ch.CasinoWins != wantWins[i] {
							t.Fatalf("actor %d coins=%d wins=%d, want coins=%d wins=%d", i, acc.Coins, ch.CasinoWins, wantCoins[i], wantWins[i])
						}
					}
				})
			}
		})
	}
}
