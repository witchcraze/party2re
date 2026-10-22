package challenge_test

import (
	"context"
	"testing"

	"github.com/witchcraze/party2re/internal/challenge"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

type stateRoundEngine struct {
	t     *testing.T
	round int
}

func (e *stateRoundEngine) Resolve(corebattle.Request) (corebattle.Result, error) {
	e.t.Fatal("stateful rounds must use the party resolver, including solo")
	return corebattle.Result{}, nil
}

func (e *stateRoundEngine) ResolvePartyBattle(req corebattle.PartyBattleRequest) (corebattle.PartyBattleResult, error) {
	e.round++
	if e.round == 1 {
		if len(req.Allies) != 2 || req.Allies[0].MP != 12 || req.Allies[1].MP != 9 {
			e.t.Fatalf("initial roster: %+v", req.Allies)
		}
		return corebattle.PartyBattleResult{Outcome: corebattle.OutcomeWin, WinnerSide: "allies",
			RemainingHP:   map[string]int{"lead": 0, "ally": 80},
			RemainingMP:   map[string]int{"lead": 3, "ally": 0},
			ConsumedItems: map[string][]corebattle.ConsumedItem{"ally": {{ID: "item-193", Quantity: 1}}}}, nil
	}
	if len(req.Allies) != 1 || req.Allies[0].ID != "ally" || req.Allies[0].HP != 80 || req.Allies[0].MP != 0 {
		e.t.Fatalf("carried roster (dead leader must stay out): %+v", req.Allies)
	}
	return corebattle.PartyBattleResult{Outcome: corebattle.OutcomeWin, WinnerSide: "enemies",
		RemainingHP: map[string]int{"ally": 0}, RemainingMP: map[string]int{"ally": 0}}, nil
}

func TestChallengeParticipantStateAcrossRounds(t *testing.T) {
	for _, live := range []bool{false, true} {
		name := "memory"
		if live {
			name = "valkey"
		}
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			var store challenge.ActiveSessionStore = challenge.NewMemorySessionRepository()
			if live {
				client := openTestValkey(t)
				t.Cleanup(client.Close)
				var err error
				store, err = challenge.NewValkeySessionRepository(client)
				if err != nil {
					t.Fatal(err)
				}
			}
			chars := &mockCharRepo{chars: map[string]corecharacter.Character{}}
			for id, mp := range map[string]int{"lead": 12, "ally": 9} {
				c := createPartyTestChar(id, 30, 200, 100, 80)
				c.Stats.MP, c.Stats.MaxMP = mp, 50
				chars.chars[id] = c
			}
			repo := newMockChallengeRepo()
			svc, err := challenge.NewService(repo, chars, &stateRoundEngine{t: t},
				challenge.WithAdmission(approvedRoster{}), challenge.WithActiveSessionStore(store))
			if err != nil {
				t.Fatal(err)
			}
			run, err := svc.StartPartySession(ctx, "lead", []string{"lead", "ally"}, "1", "Party", "")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := clearRun(ctx, store, "lead"); err != nil {
					t.Error(err)
				}
			})
			_, run, err = svc.AdvanceRound(ctx, "lead", run.ID, run.Revision)
			if err != nil {
				t.Fatal(err)
			}
			if run.Members[0].CharacterCurrentMP == nil || *run.Members[0].CharacterCurrentMP != 3 ||
				run.Members[1].CharacterCurrentMP == nil || *run.Members[1].CharacterCurrentMP != 0 || len(run.Members[1].ConsumedItems) != 1 ||
				run.Members[0].AccumulatedExp != 0 || run.Members[1].AccumulatedExp != run.AccumulatedExp {
				t.Fatalf("post-round ledger: %+v", run.Members)
			}
			// A caller cannot change nested state retained by either store.
			*run.Members[1].CharacterCurrentMP = 99
			run.Members[1].ConsumedItems[0].ID = "foreign"
			observed, err := store.GetActiveSession(ctx, "lead")
			if err != nil || observed == nil || *observed.Members[1].CharacterCurrentMP != 0 || observed.Members[1].ConsumedItems[0].ID != "item-193" {
				t.Fatalf("nested state alias: %+v, %v", observed, err)
			}
			_, run, err = svc.AdvanceRound(ctx, "lead", run.ID, run.Revision)
			if err != nil {
				t.Fatal(err)
			}
			if run.Status != challenge.StatusDefeated || run.Members[1].CharacterCurrentHP != 0 || len(run.Members[1].ConsumedItems) != 1 {
				t.Fatalf("defeat snapshot: %+v", run)
			}
			stored := repo.finalized[run.ID]
			if stored.Members[1].CharacterCurrentHP != 0 || *stored.Members[1].CharacterCurrentMP != 0 {
				t.Fatalf("finalized snapshot: %+v", stored)
			}
		})
	}
}
