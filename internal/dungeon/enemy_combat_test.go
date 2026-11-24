package dungeon_test

import (
	"context"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/dungeon"
	"github.com/witchcraze/party2re/internal/id"
)

// A boss group keeps each member's rolled state and own payload across
// save/reload; a defeated member stays out of later battles.
func TestDungeonEnemyGroupSurvivesReload(t *testing.T) {
	stage6 := mapDef(t, dungeon.NativeMap3ID).Variants[0].Pools[1].Bosses[0]
	diver := mapDef(t, dungeon.NativeMap14ID).Variants[0].Pools[0].Monsters[4]
	for _, kind := range []string{"memory", "valkey"} {
		t.Run(kind, func(t *testing.T) {
			ctx := context.Background()
			store := encounterStore(t, kind)
			hero := strongHero(id.New())
			var first []corebattle.Participant
			calls := 0
			engine := &customPartyBattleEngine{resolvePartyFn: func(req corebattle.PartyBattleRequest) (corebattle.PartyBattleResult, error) {
				calls++
				if calls == 1 {
					first = req.Enemies
					if len(first) != 2 || first[0].ID == first[1].ID || first[0].Status != corebattle.StatusSleep || first[1].CMP == 0 {
						t.Fatalf("initial enemies %+v", first)
					}
					// Party size 1 at rand 0.99 scales by 1.097.
					if first[0].HP != int(float64(stage6.HP)*(0.9+0.3*0.99-0.1)) {
						t.Fatalf("stage 6 boss HP %d not rolled from %d", first[0].HP, stage6.HP)
					}
					return corebattle.PartyBattleResult{
						Outcome:         corebattle.OutcomeDraw,
						RemainingHP:     map[string]int{hero.ID: hero.Stats.HP, first[0].ID: first[0].HP - 10, first[1].ID: 0},
						RemainingMP:     map[string]int{first[0].ID: first[0].MP - 1},
						RemainingCMP:    map[string]int{first[1].ID: first[1].CMP / 2},
						RemainingStatus: map[string]string{first[0].ID: ""},
					}, nil
				}
				if len(req.Enemies) != 1 || req.Enemies[0].ID != first[0].ID || req.Enemies[0].HP != first[0].HP-10 ||
					req.Enemies[0].MP != first[0].MP-1 || req.Enemies[0].Status != "" {
					t.Fatalf("resumed enemies %+v", req.Enemies)
				}
				return corebattle.PartyBattleResult{Outcome: corebattle.OutcomeDraw, RemainingHP: map[string]int{hero.ID: hero.Stats.HP}}, nil
			}}
			catalog := []dungeon.Dungeon{{ID: "payload", RoundLimit: 20, Variants: []dungeon.Floor{{
				Width: 2, Height: 1, Grid: []string{"SB"}, Pools: []dungeon.EncounterPool{{Bosses: []dungeon.DungeonMonster{stage6, diver}}},
			}}}}
			repo := newMockDungeonRepo()
			chars := &mockCharRepo{chars: map[string]corecharacter.Character{hero.ID: hero}}
			newService := func() *dungeon.Service {
				t.Helper()
				svc, err := dungeon.NewService(repo, chars, engine, dungeon.WithCustomDungeons(catalog), dungeon.WithActiveExpeditionStore(store), dungeon.WithRNG(&scriptedRNG{}))
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
			catalog[0].Variants[0].Pools = nil
			svc = newService()
			exploring, err := svc.GetActiveExpedition(ctx, hero.ID)
			if err != nil || exploring == nil || exploring.Encounter == nil {
				t.Fatalf("reload %+v: %v", exploring, err)
			}
			enc := exploring.Encounter
			if len(enc.Monsters) != 2 || enc.Enemies[1].HP != 0 || enc.Enemies[1].CMP != first[1].CMP/2 {
				t.Fatalf("saved group %+v", enc)
			}
			boss, chick := enc.Monsters[0].EnemyCombatForTest(), enc.Monsters[1].EnemyCombatForTest()
			if boss == nil || chick == nil || boss.Jobs.JobID != "job-21" || boss.MaxHP != first[0].HP ||
				chick.NPCLoadout == nil || chick.NPCLoadout.Gems[0] != "gem_sky_atk_1" || chick.MaxCMP != first[1].MaxCMP {
				t.Fatalf("saved payloads %+v / %+v", boss, chick)
			}
			if _, err := svc.Move(ctx, hero.ID, dungeon.DirectionWest); err == nil {
				t.Fatal("moved while an enemy lives")
			}
			boss.Hit = 1
			resumed, err := svc.Fight(ctx, hero.ID)
			if err != nil || calls != 2 || resumed.Expedition.Encounter == nil {
				t.Fatalf("fight %+v, calls %d: %v", resumed, calls, err)
			}
			if data := resumed.Expedition.Encounter.Monsters[0].EnemyCombatForTest(); data == nil || data.Hit == 1 {
				t.Fatalf("saved payload shared with observation: %+v", data)
			}
		})
	}
}
