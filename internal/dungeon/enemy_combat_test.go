package dungeon_test

import (
	"context"
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/dungeon"
	"github.com/witchcraze/party2re/internal/id"
)

func TestDungeonEnemyCombatSurvivesReload(t *testing.T) {
	stage6 := *mapDef(t, dungeon.NativeMap3ID).Variants[0].Pools[1].Boss
	diver := mapDef(t, dungeon.NativeMap14ID).Variants[0].Pools[0].Monsters[4]
	for _, tc := range []struct {
		monster dungeon.DungeonMonster
		mp, cmp int
		status  string
	}{
		{stage6, 60, 0, corebattle.StatusSleep},
		{diver, 97, 80, ""},
	} {
		for _, kind := range []string{"memory", "valkey"} {
			t.Run(tc.monster.ID+"/"+kind, func(t *testing.T) {
				ctx := context.Background()
				store := encounterStore(t, kind)
				hero := strongHero(id.New())
				want := tc.monster.EnemyCombatForTest().Clone()
				calls := 0
				engine := &customPartyBattleEngine{resolvePartyFn: func(req corebattle.PartyBattleRequest) (corebattle.PartyBattleResult, error) {
					enemy := req.Enemies[0]
					if calls == 0 {
						if enemy.HP != tc.monster.HP || enemy.MP != tc.mp || enemy.CMP != tc.cmp || enemy.Status != tc.status {
							t.Fatalf("initial enemy %+v", enemy)
						}
					} else if enemy.HP != tc.monster.HP-10 || enemy.MP != tc.mp-1 || enemy.CMP != tc.cmp/2 || enemy.Status != "" {
						t.Fatalf("resumed enemy %+v", enemy)
					}
					calls++
					return corebattle.PartyBattleResult{
						Outcome:         corebattle.OutcomeDraw,
						RemainingHP:     map[string]int{hero.ID: hero.Stats.HP, enemy.ID: tc.monster.HP - 10},
						RemainingMP:     map[string]int{enemy.ID: tc.mp - 1},
						RemainingCMP:    map[string]int{enemy.ID: tc.cmp / 2},
						RemainingStatus: map[string]string{enemy.ID: ""},
					}, nil
				}}
				catalog := []dungeon.Dungeon{{ID: "payload", RoundLimit: 20, Variants: []dungeon.Floor{{
					Width: 2, Height: 1, Grid: []string{"SB"}, Boss: &tc.monster,
				}}}}
				repo := newMockDungeonRepo()
				chars := &mockCharRepo{chars: map[string]corecharacter.Character{hero.ID: hero}}
				newService := func() *dungeon.Service {
					t.Helper()
					svc, err := dungeon.NewService(repo, chars, engine, dungeon.WithCustomDungeons(catalog), dungeon.WithActiveExpeditionStore(store))
					if err != nil {
						t.Fatal(err)
					}
					return svc
				}
				svc := newService()
				exp, err := svc.StartExpedition(ctx, hero.ID, "payload")
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() {
					if err := store.DeleteActiveExpedition(ctx, hero.ID, exp.ID); err != nil {
						t.Error(err)
					}
				})
				if _, err := svc.Move(ctx, hero.ID, dungeon.DirectionEast); err != nil {
					t.Fatal(err)
				}
				catalog[0].Variants[0].Boss = nil
				svc = newService()
				exploring, err := svc.GetActiveExpedition(ctx, hero.ID)
				if err != nil || exploring == nil || exploring.Encounter == nil {
					t.Fatalf("reload %+v: %v", exploring, err)
				}
				data := exploring.Encounter.Monster.EnemyCombatForTest()
				if data == nil || !reflect.DeepEqual(*data, want) {
					t.Fatalf("saved payload %+v, want %+v", data, want)
				}
				data.Hit = 1
				resumed, err := svc.Fight(ctx, hero.ID)
				if err != nil || calls != 2 || resumed.Expedition.Encounter == nil {
					t.Fatalf("fight %+v, calls %d: %v", resumed, calls, err)
				}
				if data := resumed.Expedition.Encounter.Monster.EnemyCombatForTest(); data == nil || data.Hit != want.Hit {
					t.Fatalf("saved payload shared with observation: %+v", data)
				}
			})
		}
	}
}
