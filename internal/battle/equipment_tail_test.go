package battle_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/witchcraze/party2re/internal/battle"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreequipment "github.com/witchcraze/party2re/internal/core/equipment"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	corejob "github.com/witchcraze/party2re/internal/core/job"
	coreskill "github.com/witchcraze/party2re/internal/core/skill"
)

type equipmentTailRNG struct{ draws int }

func (r *equipmentTailRNG) Float64() float64 { r.draws++; return 0 }
func (r *equipmentTailRNG) Intn(int) int     { panic("unexpected integer draw") }
func (r *equipmentTailRNG) IntN(n int) int   { return r.Intn(n) }

func TestEquipmentTailAdapterEquipmentAndReload(t *testing.T) {
	for _, n := range []int{164, 202, 206, 208, 209, 246} {
		t.Run(fmt.Sprint(n), func(t *testing.T) {
			ctx := context.Background()
			chars, inventory, equipment := newMockCharRepo(), newMockInvRepo(), newMockEquipRepo()
			char := corecharacter.Character{ID: "self", Level: 30, JobID: "job-01", Stats: corecharacter.Stats{HP: 100, MaxHP: 1000, MP: 100, MaxMP: 1000, Attack: 100, Defense: 100, Agility: 100}}
			chars.characters[char.ID] = char
			item := fmt.Sprintf("item-%d", n)
			if err := inventory.Save(ctx, coreinventory.Inventory{CharacterID: char.ID, Items: []coreitem.Instance{
				{ID: "held", DefinitionID: item, Quantity: 1}, {ID: "bag", DefinitionID: "item-164", Quantity: 1},
			}}); err != nil {
				t.Fatal(err)
			}
			if err := equipment.Save(ctx, coreequipment.Equipment{CharacterID: char.ID, Slots: map[coreitem.Slot]string{coreitem.SlotAccessory: "held"}}); err != nil {
				t.Fatal(err)
			}
			svc := battle.NewService(battle.WithCharacterRepository(chars), battle.WithInventoryRepository(inventory), battle.WithEquipmentRepository(equipment), battle.WithRandomSource(mockRNG{}), battle.WithJobHistoryRepository(&historyReadStub{err: corejob.ErrCharacterJobNotFound}))
			a, err := svc.BuildLifetimeActor(ctx, char.ID, "")
			if err != nil {
				t.Fatal(err)
			}
			if a.Gear.AccessoryID != item || len(a.ItemDefinitionIDs) != 1 || a.ItemDefinitionIDs[0] != item {
				t.Fatal(a)
			}
			state := corebattle.LifetimeState{Actors: []corebattle.LifetimeActor{a,
				{Participant: corebattle.Participant{ID: "ally", TeamID: a.TeamID, HP: 100}},
				{Participant: corebattle.Participant{ID: "helper", TeamID: a.TeamID, HP: 0}, RosterIdentity: "guardian"},
				{Participant: corebattle.Participant{ID: "enemy", TeamID: "enemy", HP: 1000, Gear: &corebattle.BattleGearStats{}}},
			}}
			c := corebattle.EquipmentTailContext{DamageContext: corebattle.DamageContext{EffectContext: corebattle.EffectContext{TargetContext: corebattle.TargetContext{ActorID: "self", PartyIDs: []string{"self", "ally", "helper"}, EnemyIDs: []string{"enemy"}}}}, Completion: corebattle.LifetimeCompletion{Accepted: true, ActorID: "self", EnemiesRemain: true}}
			id := fmt.Sprintf("item-%d-operation-tail", n)
			wantHP, wantMP, wantDraws := 100, 100, 1
			switch n {
			case 164:
				state.Actors[0].Temporary = corebattle.TemporaryDefense
			case 202:
				id, wantDraws, wantHP = "item-202-combat-tail", 1, 136
			case 206:
				state.Actors[0].Status = corebattle.StatusSleep
				wantDraws, wantHP = 4, 370
			case 208:
				id, wantDraws = "item-208-combat-tail", 0
			case 209:
				state.Actors[0].Temporary = corebattle.TemporaryCovering
				wantHP = 127
			case 246:
				id = "item-246-combat-tail"
			}
			c.Snapshot, err = state.CaptureEquipmentTail(c.TargetContext)
			if err != nil {
				t.Fatal(err)
			}
			state.Actors[0].Status = ""
			frozen := struct {
				State    corebattle.LifetimeState
				Snapshot corebattle.EquipmentTailSnapshot
			}{state, c.Snapshot}
			data, err := json.Marshal(frozen)
			if err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(data, &frozen); err != nil {
				t.Fatal(err)
			}
			state, c.Snapshot = frozen.State, frozen.Snapshot
			rng := &equipmentTailRNG{}
			var definitions coreskill.BattleDefinitions
			definitions.RegisterEquipmentTails()
			if n == 202 {
				definitions.RegisterPlayerTails()
				player := corebattle.PlayerTailContext{DamageContext: c.DamageContext, Completion: c.Completion}
				if err := definitions.PlayerTailAction(&state, &player, id, rng); err != nil {
					t.Fatal(err)
				}
				if err := state.ApplyPlayerTail(&player, id, rng); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := definitions.EquipmentTailAction(&state, &c, id, rng); err != nil || rng.draws != 0 {
					t.Fatal(err, rng.draws)
				}
				if err := state.ApplyEquipmentTail(&c, id, rng); err != nil {
					t.Fatal(err)
				}
			}
			if err := state.ApplyEquipmentTail(&c, "guardian-mp-tail", rng); err != nil {
				t.Fatal(err)
			}
			wantMP += 4
			if state.Actors[0].HP != wantHP || state.Actors[0].MP != wantMP || rng.draws != wantDraws+2 || chars.characters[char.ID].Stats.HP != 100 {
				t.Fatal(state, rng.draws)
			}
			if n == 208 && state.Actors[1].Hit != 95 {
				t.Fatal(state)
			}
		})
	}
}
