package dungeon_test

import (
	"context"
	"errors"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/random"
	"github.com/witchcraze/party2re/internal/dungeon"
	"github.com/witchcraze/party2re/internal/id"
)

func TestActiveEncounterSharedResolver(t *testing.T) {
	ctx := context.Background()
	char := createTestChar("hero", 10, 5000, 10, 0)
	monster := dungeon.DungeonMonster{ID: "enemy", HP: 750, Attack: 0, ExpReward: 17, GoldReward: 23}
	catalog := []dungeon.Dungeon{{ID: "real", RoundLimit: 21, Variants: []dungeon.Floor{{FloorNumber: 1, Width: 2, Height: 1, Grid: []string{"S0"}, Monsters: []dungeon.DungeonMonster{monster}}}}}
	expectedEnemyHP := 750
	calls := 0
	engine := &customPartyBattleEngine{resolvePartyFn: func(req corebattle.PartyBattleRequest) (corebattle.PartyBattleResult, error) {
		calls++
		if req.Enemies[0].HP != expectedEnemyHP {
			t.Fatalf("enemy restored: got %d want %d", req.Enemies[0].HP, expectedEnemyHP)
		}
		res, err := (corebattle.Engine{}).ResolvePartyBattle(req)
		if err != nil {
			return res, err
		}
		hp := res.RemainingHP["enemy"]
		if hp >= expectedEnemyHP {
			t.Fatal("real engine made no combat progress")
		}
		expectedEnemyHP = hp
		return res, nil
	}}
	svc, err := dungeon.NewService(newMockDungeonRepo(), &mockCharRepo{chars: map[string]corecharacter.Character{char.ID: char}}, engine, dungeon.WithCustomDungeons(catalog), dungeon.WithRNG(random.NewDeterministic(1711)))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = svc.StartExpedition(ctx, char.ID, "real"); err != nil {
		t.Fatal(err)
	}
	res, err := svc.Move(ctx, char.ID, dungeon.DirectionEast)
	if err != nil {
		t.Fatal(err)
	}
	if res.BattleResult == nil || res.BattleResult.Outcome != corebattle.OutcomeDraw || res.Expedition.Encounter == nil {
		t.Fatalf("expected real turn-limit draw: %+v", res)
	}
	for i := 0; res.Expedition.Encounter != nil && i < 10; i++ {
		res, err = svc.Fight(ctx, char.ID)
		if err != nil {
			t.Fatal(err)
		}
	}
	if res.Expedition.Encounter != nil || calls < 2 || res.Expedition.AccumulatedExp != 17 || res.Expedition.AccumulatedGold != 23 {
		t.Fatalf("real combat did not finish once: %+v", res)
	}
}

func TestActiveEncounterPartyLeaderFallen(t *testing.T) {
	for _, kind := range []string{"memory", "valkey"} {
		t.Run(kind, func(t *testing.T) {
			ctx := context.Background()
			store := encounterStore(t, kind)
			leader := createTestChar(id.New(), 10, 100, 20, 10)
			ally := createTestChar(id.New(), 10, 100, 20, 10)
			leader.Stats.MP = 10
			leader.Stats.MaxMP = 10
			ally.Stats.MP = 10
			ally.Stats.MaxMP = 10
			repo := newMockDungeonRepo()
			chars := &mockCharRepo{chars: map[string]corecharacter.Character{leader.ID: leader, ally.ID: ally}}
			monster := dungeon.DungeonMonster{ID: "enemy", HP: 100, Attack: 10, ExpReward: 17, GoldReward: 23}
			catalog := []dungeon.Dungeon{{ID: "party", RoundLimit: 21, Variants: []dungeon.Floor{{FloorNumber: 1, Width: 2, Height: 1, Grid: []string{"S0"}, Monsters: []dungeon.DungeonMonster{monster}}}}}
			calls := 0
			engine := &customPartyBattleEngine{resolvePartyFn: func(req corebattle.PartyBattleRequest) (corebattle.PartyBattleResult, error) {
				calls++
				if calls == 1 {
					if len(req.Allies) != 2 {
						t.Fatal("party roster missing")
					}
					return corebattle.PartyBattleResult{Outcome: corebattle.OutcomeDraw, RemainingHP: map[string]int{leader.ID: 0, ally.ID: 60, "enemy": 30}, RemainingMP: map[string]int{ally.ID: 4}, ConsumedItems: map[string][]corebattle.ConsumedItem{ally.ID: {{ID: "one-use", Quantity: 1}}}}, nil
				}
				if len(req.Allies) != 1 || req.Allies[0].ID != ally.ID || req.Allies[0].HP != 60 || req.Allies[0].MP != 4 || req.Enemies[0].HP != 30 {
					t.Fatalf("party continuation reset: %+v", req)
				}
				return corebattle.PartyBattleResult{Outcome: corebattle.OutcomeWin, WinnerSide: "allies", RemainingHP: map[string]int{ally.ID: 50, "enemy": 0}, RemainingMP: map[string]int{ally.ID: 2}}, nil
			}}
			svc, err := dungeon.NewService(repo, chars, engine, dungeon.WithActiveExpeditionStore(store), dungeon.WithCustomDungeons(catalog), dungeon.WithAdmission(approvedRoster{}), dungeon.WithRNG(&minimalRandomSource{}))
			if err != nil {
				t.Fatal(err)
			}
			exp, err := svc.StartPartyExpedition(ctx, leader.ID, []string{leader.ID, ally.ID}, "party", "party-id")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := store.DeleteActiveExpedition(ctx, leader.ID, exp.ID); err != nil {
					t.Error(err)
				}
			})
			if _, err = svc.Move(ctx, leader.ID, dungeon.DirectionEast); err != nil {
				t.Fatal(err)
			}
			if _, err = svc.Fight(ctx, ally.ID); !errors.Is(err, dungeon.ErrForbidden) {
				t.Fatalf("nonleader continued: %v", err)
			}
			active, err := store.GetActiveExpedition(ctx, leader.ID)
			if err != nil {
				t.Fatal(err)
			}
			if active.Members[1].CurrentMP != 4 || len(active.Members[1].ConsumedItems) != 1 {
				t.Fatalf("lost MP/item state: %+v", active.Members)
			}
			res, err := svc.Fight(ctx, leader.ID)
			if err != nil {
				t.Fatal(err)
			}
			if res.Expedition.Encounter != nil || res.Expedition.Members[0].AccumulatedGold != 0 || res.Expedition.Members[1].AccumulatedGold != 23 {
				t.Fatalf("party reward eligibility: %+v", res.Expedition)
			}
			if _, err = svc.Escape(ctx, leader.ID); err != nil {
				t.Fatal(err)
			}
			if repo.savedChars[ally.ID].Stats.HP != 50 || repo.savedChars[ally.ID].Stats.MP != 2 || repo.savedChars[ally.ID].Money != 23 {
				t.Fatalf("party finalization: %+v", repo.savedChars)
			}
		})
	}
}
