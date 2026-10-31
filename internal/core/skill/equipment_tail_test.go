package skill

import (
	"errors"
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func TestEquipmentTailManifestDiscoveryAndExecution(t *testing.T) {
	rows := []struct {
		id, item string
		tmp      corebattle.TemporaryEffect
		draws    []float64
		hp, mp   int
	}{
		{"item-164-operation-tail", "item-164", corebattle.TemporaryDefense, []float64{0}, 100, 100},
		{"item-209-operation-tail", "item-209", corebattle.TemporaryCovering, []float64{0}, 127, 100},
		{"item-206-operation-tail", "item-206", "", []float64{0, .5}, 370, 100},
		{"item-202-npc-tail", "item-202", "", []float64{0, 0}, 127, 100},
		{"item-208-combat-tail", "item-208", "", nil, 100, 100},
		{"guardian-mp-tail", "", "", []float64{0, 0}, 100, 104},
		{"item-246-combat-tail", "item-246", "", []float64{.5}, 100, 100},
	}
	var ids []string
	for _, row := range rows {
		ids = append(ids, row.id)
	}
	if !reflect.DeepEqual(ids, corebattle.EquipmentTailEntries()) {
		t.Fatal(ids, corebattle.EquipmentTailEntries())
	}
	for _, npc := range []bool{false, true} {
		for _, row := range rows {
			s := corebattle.LifetimeState{Actors: []corebattle.LifetimeActor{
				{Participant: corebattle.Participant{ID: "actor", TeamID: "party", HP: 100, MaxHP: 1000, MP: 100, MaxMP: 1000, Attack: 100, Gear: &corebattle.BattleGearStats{}, ItemDefinitionIDs: []string{row.item}}, NPC: npc, Tension: 1, Temporary: row.tmp, Hit: 80},
				{Participant: corebattle.Participant{ID: "helper", TeamID: "party", HP: 0}, RosterIdentity: "guardian"},
				{Participant: corebattle.Participant{ID: "requester", TeamID: "party", HP: 100}},
				{Participant: corebattle.Participant{ID: "enemy", TeamID: "enemy", HP: 1000, Gear: &corebattle.BattleGearStats{}}},
			}}
			c := corebattle.EquipmentTailContext{DamageContext: corebattle.DamageContext{EffectContext: corebattle.EffectContext{TargetContext: corebattle.TargetContext{ActorID: "actor", PartyIDs: []string{"actor", "helper", "requester"}, EnemyIDs: []string{"enemy"}}, ActorNPC: npc}}, Completion: corebattle.LifetimeCompletion{Accepted: true, ActorID: "actor", NPC: npc, EnemiesRemain: true}, Snapshot: corebattle.EquipmentTailSnapshot{Item206: row.id == "item-206-operation-tail"}}
			var d BattleDefinitions
			if err := d.EquipmentTailAction(&s, &c, row.id, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
				t.Fatal(row.id, err)
			}
			d.RegisterEquipmentTails()
			if len(d.EquipmentTails) != len(rows) || len(d.Invocations.Entries) != 0 || len(d.Invocations.JobTables) != 0 || len(d.PlayerTails) != 0 {
				t.Fatal(d)
			}
			before, context := s.Actors[0], c
			if err := d.EquipmentTailAction(&s, &c, row.id, noArithmeticDraws{t}); err != nil || !reflect.DeepEqual(before, s.Actors[0]) || !reflect.DeepEqual(context, c) {
				t.Fatal(row.id, npc, err)
			}
			draws, wantHP := row.draws, row.hp
			if row.id == "item-202-npc-tail" && !npc {
				draws, wantHP = nil, 100
			}
			rng := &damageGemDraws{t: t, draws: draws}
			if err := s.ApplyEquipmentTail(&c, row.id, rng); err != nil || s.Actors[0].HP != wantHP || s.Actors[0].MP != row.mp || rng.used != len(draws) {
				t.Fatal(row.id, npc, s, rng.used, err)
			}
			if row.id == "item-164-operation-tail" && s.Actors[0].Tension != 1.7 {
				t.Fatal(s)
			}
			if row.id == "item-208-combat-tail" && s.Actors[0].Hit != 95 {
				t.Fatal(s)
			}
			if row.id == "item-246-combat-tail" && s.Actors[0].Temporary != corebattle.TemporaryReflectDance {
				t.Fatal(s)
			}
			for _, id := range []string{"item-110", "gem-101", "gem-102", "item-202-combat-tail"} {
				d.EquipmentTails[id] = true
				if err := d.EquipmentTailAction(&s, &c, id, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
					t.Fatal(id, err)
				}
			}
		}
	}
}
