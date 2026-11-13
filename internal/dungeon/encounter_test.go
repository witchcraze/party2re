package dungeon_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/dungeon"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/testutil"
)

func encounterStore(t *testing.T, kind string) dungeon.ActiveExpeditionStore {
	t.Helper()
	if kind == "memory" {
		return dungeon.NewMemoryExpeditionRepository()
	}
	client := openTestValkey(t)
	t.Cleanup(client.Close)
	store, err := dungeon.NewValkeyExpeditionRepository(client)
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func TestActiveEncounterLifecycle(t *testing.T) {
	for _, kind := range []string{"memory", "valkey"} {
		for _, ending := range []string{"win", "wipeout", "escape"} {
			t.Run(kind+"/"+ending, func(t *testing.T) {
				ctx := context.Background()
				store := encounterStore(t, kind)
				char := createTestChar(id.New(), 10, 100, 30, 20)
				charRepo := &mockCharRepo{chars: map[string]corecharacter.Character{char.ID: char}}
				repo := newMockDungeonRepo()
				monster := dungeon.DungeonMonster{ID: "enemy", Name: "Enemy", HP: 100, Attack: 10, ExpReward: 17, GoldReward: 23, DropItemID: "potion"}
				floor := dungeon.Floor{FloorNumber: 1, Width: 3, Height: 3, StartX: 1, StartY: 1, Grid: []string{"1T1", "ES0", "1D1"}, Monsters: []dungeon.DungeonMonster{monster}}
				catalog := []dungeon.Dungeon{{ID: "encounter", RoundLimit: 21, Variants: []dungeon.Floor{floor}}}
				calls := 0
				engine := &customPartyBattleEngine{resolvePartyFn: func(req corebattle.PartyBattleRequest) (corebattle.PartyBattleResult, error) {
					calls++
					wantEnemyHP := 100 - (calls-1)*30
					if req.Enemies[0].HP != wantEnemyHP {
						t.Fatalf("enemy healed on resume: got %d want %d", req.Enemies[0].HP, wantEnemyHP)
					}
					if calls > 1 && (req.Enemies[0].MP != 4 || req.Enemies[0].Status != "poison") {
						t.Fatalf("lost enemy MP/status: %+v", req.Enemies[0])
					}
					wantAllyHP := 100 - (calls-1)*10
					if req.Allies[0].HP != wantAllyHP {
						t.Fatalf("ally HP reset: got %d want %d", req.Allies[0].HP, wantAllyHP)
					}
					hp := map[string]int{char.ID: wantAllyHP - 10, "enemy": wantEnemyHP - 30}
					outcome, side := corebattle.OutcomeDraw, ""
					if calls == 3 {
						outcome, side = corebattle.OutcomeWin, "allies"
						hp["enemy"] = 0
					}
					if calls == 3 && ending == "wipeout" {
						outcome, side = corebattle.OutcomeDefeat, "enemies"
						hp[char.ID] = 0
						hp["enemy"] = 40
					}
					return corebattle.PartyBattleResult{Outcome: outcome, WinnerSide: side, RemainingHP: hp, RemainingMP: map[string]int{"enemy": 4}, RemainingStatus: map[string]string{"enemy": "poison"}}, nil
				}}
				hookCalls := 0
				newService := func() *dungeon.Service {
					svc, err := dungeon.NewService(repo, charRepo, engine, dungeon.WithActiveExpeditionStore(store), dungeon.WithCustomDungeons(catalog), dungeon.WithRNG(&minimalRandomSource{}))
					if err != nil {
						t.Fatal(err)
					}
					svc.SetMonsterDefeatedHook(func(context.Context, string, int) error {
						hookCalls++
						return errors.New("medal progress unavailable")
					})
					return svc
				}
				svc := newService()
				exp, err := svc.StartExpedition(ctx, char.ID, "encounter")
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() {
					if err := store.DeleteActiveExpedition(ctx, char.ID, exp.ID); err != nil {
						t.Error(err)
					}
				})
				if _, err = svc.Fight(ctx, char.ID); !errors.Is(err, dungeon.ErrNoActiveCombat) {
					t.Fatalf("fight without encounter: %v", err)
				}
				first, err := svc.Move(ctx, char.ID, dungeon.DirectionEast)
				if err != nil {
					t.Fatal(err)
				}
				if first.Expedition.Encounter == nil || first.Expedition.Encounter.Enemies[0].HP != 70 {
					t.Fatalf("missing active enemy: %+v", first.Expedition)
				}
				// Mutating a returned snapshot must not mutate stored combat state.
				first.Expedition.Encounter.Enemies[0].HP = 999
				svc = newService()
				before, err := store.GetActiveExpedition(ctx, char.ID)
				if err != nil {
					t.Fatal(err)
				}
				for _, dir := range []dungeon.Direction{dungeon.DirectionNorth, dungeon.DirectionSouth, dungeon.DirectionEast, dungeon.DirectionWest} {
					if _, err = svc.Move(ctx, char.ID, dir); !errors.Is(err, dungeon.ErrCombatActive) {
						t.Fatalf("move %s: %v", dir, err)
					}
				}
				after, err := store.GetActiveExpedition(ctx, char.ID)
				if err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(before, after) || calls != 1 {
					t.Fatal("rejected movement changed state or resolved battle")
				}
				if before.AccumulatedGold != 0 || before.AccumulatedExp != 0 || len(before.ConsumedEvents) != 0 {
					t.Fatal("draw granted rewards/events")
				}
				if ending == "escape" {
					res, err := svc.Escape(ctx, char.ID)
					if err != nil || !res.IsFinished || res.Expedition.Status != dungeon.StatusEscaped {
						t.Fatalf("escape: %+v %v", res, err)
					}
					if len(repo.histories[char.ID]) != 1 || repo.histories[char.ID][0].GoldReward != 0 {
						t.Fatal("escape rewarded unfinished enemy")
					}
					return
				}
				second, err := svc.Fight(ctx, char.ID)
				if err != nil || second.Expedition.Encounter == nil || second.Expedition.Encounter.Enemies[0].HP != 40 {
					t.Fatalf("second draw: %+v %v", second, err)
				}
				final, err := svc.Fight(ctx, char.ID)
				if err != nil {
					t.Fatal(err)
				}
				if ending == "win" {
					if final.Expedition.Encounter != nil || final.Expedition.PosX != 2 || final.Expedition.AccumulatedExp != 17 || final.Expedition.AccumulatedGold != 23 || len(final.Expedition.AccumulatedItems) != 1 {
						t.Fatalf("victory: %+v", final)
					}
					if _, err = svc.Fight(ctx, char.ID); !errors.Is(err, dungeon.ErrNoActiveCombat) {
						t.Fatalf("repeat fight: %v", err)
					}
					if _, err = svc.Escape(ctx, char.ID); err != nil {
						t.Fatal(err)
					}
				} else if !final.IsFinished {
					t.Fatalf("terminal fight did not settle: %+v", final)
				}
				if len(repo.histories[char.ID]) != 1 {
					t.Fatal("settlement not exactly once")
				}
				wantHooks := 0
				if ending == "win" {
					wantHooks = 1
				}
				if hookCalls != wantHooks {
					t.Fatalf("progress attempts=%d want=%d", hookCalls, wantHooks)
				}
				if _, err = svc.Escape(ctx, char.ID); !errors.Is(err, dungeon.ErrNoActiveExpedition) {
					t.Fatalf("repeat settlement: %v", err)
				}
			})
		}
	}
}

func TestEncounterStepCAS(t *testing.T) {
	for _, kind := range []string{"memory", "valkey"} {
		t.Run(kind, func(t *testing.T) {
			ctx := context.Background()
			store := encounterStore(t, kind)
			exp := sampleExpedition(id.New(), id.New(), "encounter")
			if err := store.SaveActiveExpedition(ctx, exp); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := store.DeleteActiveExpedition(ctx, exp.CharacterID, exp.ID); err != nil {
					t.Error(err)
				}
			})
			enc := &dungeon.ActiveEncounter{Monster: dungeon.DungeonMonster{ID: "enemy", HP: 100}, Enemies: []corebattle.Participant{corebattle.MustNewParticipant("enemy", 70, 10, 10)}, EventType: dungeon.EventBattle, X: 1}
			params := dungeon.StepParams{ExpectedExpeditionID: exp.ID, ExpectedRevision: &exp.Revision, Combat: true, Encounter: enc, TurnsDelta: -1, Now: time.Now().UTC()}
			if _, err := store.Step(ctx, exp.CharacterID, params); err != nil {
				t.Fatal(err)
			}
			// An old move cannot slip through after a concurrent encounter publication.
			staleMove := params
			staleMove.Combat = false
			staleMove.Encounter = nil
			staleMove.GoldDelta = 999
			staleMove.EventKey = "treasure"
			if _, err := store.Step(ctx, exp.CharacterID, staleMove); !errors.Is(err, dungeon.ErrCombatActive) {
				t.Fatalf("stale move: %v", err)
			}
			if _, err := store.Step(ctx, exp.CharacterID, params); !errors.Is(err, dungeon.ErrExpeditionStateChanged) {
				t.Fatalf("stale combat: %v", err)
			}
			active, err := store.GetActiveExpedition(ctx, exp.CharacterID)
			if err != nil {
				t.Fatal(err)
			}
			params.ExpectedRevision = &active.Revision
			params.Encounter = nil
			params.ExpDelta = 17
			params.GoldDelta = 23
			params.RewardItemID = "potion"
			errs := testutil.RunRace(func() error {
				_, err := store.Step(ctx, exp.CharacterID, params)
				if errors.Is(err, dungeon.ErrExpeditionStateChanged) {
					return nil
				}
				return err
			}, func() error {
				_, err := store.Step(ctx, exp.CharacterID, params)
				if errors.Is(err, dungeon.ErrExpeditionStateChanged) {
					return nil
				}
				return err
			})
			for _, err := range errs {
				if err != nil {
					t.Fatal(err)
				}
			}
			active, err = store.GetActiveExpedition(ctx, exp.CharacterID)
			if err != nil {
				t.Fatal(err)
			}
			if active.Encounter != nil || active.AccumulatedExp != 17 || active.AccumulatedGold != 23 || len(active.AccumulatedItems) != 1 || len(active.ConsumedEvents) != 0 || active.TurnsRemaining != 48 {
				t.Fatalf("duplicate or lost update: %+v", active)
			}
		})
	}
}
