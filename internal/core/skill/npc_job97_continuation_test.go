package skill

import (
	"encoding/json"
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func TestNPC97ResetFrozenReloadAndCompletedCallbackFacts(t *testing.T) {
	s, c, d := npc97Fixture()
	s.Actors[0].NPC, c.ActorNPC = true, true
	s.Actors[0].WeaponSeal = 6
	c.Command, c.Custom = "frozen-ordinary-job97", true
	c.EquipmentReset.Seals = &corebattle.WeaponSealTable{DragonAttack: 17, PhoenixWeight: -31}
	// item159's package cursor survives the following fallen-member reset;
	// every member regains their own saved tension, including seal11.
	s.Actors[1].Gear.AccessoryID, s.Actors[1].ItemDefinitionIDs = "item-159", []string{"item-159"}
	s.Actors[1].WeaponSeal, s.Actors[1].Tension = 11, 5
	s.Actors[2].Tension, s.Actors[2].Temporary = 1.7, corebattle.TemporaryRage
	executeNPC97(t, &s, c, d, 6, nil)
	type checkpoint struct {
		State             corebattle.LifetimeState
		Target            corebattle.TargetContext
		Seals             corebattle.WeaponSealTable
		Entries           []corebattle.CallbackEntry
		Cursor            corebattle.TargetCursor
		Arguments         []string
		CallbackCompleted bool
		End               corebattle.EndProcessing
		DamageCalls       int
	}
	frozen := checkpoint{State: s, Target: c.TargetContext, Seals: *c.EquipmentReset.Seals, Entries: d.Invocations.JobTables[97].Variants[0], Cursor: c.Cursor, Arguments: []string{"ignored", "tail"}, CallbackCompleted: true}
	data, err := json.Marshal(frozen)
	if err != nil {
		t.Fatal(err)
	}
	var loaded checkpoint
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatal(err)
	}
	if !loaded.CallbackCompleted || loaded.State.Actors[0].Tension != 3 || loaded.State.Actors[0].Temporary != corebattle.TemporaryDouble || !reflect.DeepEqual(loaded.Entries, corebattle.NPCJobEntries(97)) {
		t.Fatal(loaded)
	}
	resumed := &corebattle.NPCJob97Context{DamageContext: corebattle.DamageContext{
		EffectContext: corebattle.EffectContext{TargetContext: loaded.Target, ActorNPC: true},
		Cursor:        loaded.Cursor, GuaranteedHit: true,
		Defeat: func(string) error { t.Fatal("unexpected defeat"); return nil },
	}, EquipmentReset: &corebattle.EquipmentResetContext{Seals: &loaded.Seals}}
	var rebuilt BattleDefinitions
	rebuilt.RegisterNPCJob97(resumed)
	for _, run := range []struct {
		s *corebattle.LifetimeState
		c *corebattle.NPCJob97Context
		d BattleDefinitions
	}{{&s, c, d}, {&loaded.State, resumed, rebuilt}} {
		executeNPC97(t, run.s, run.c, run.d, 2, loaded.Arguments, 0, 0, 0)
	}
	if !reflect.DeepEqual(s, loaded.State) || s.Actors[0].Tension != 3 || s.Actors[1].Tension != 5 || s.Actors[2].Tension != 1.7 || s.Actors[0].Attack != 80 || s.Actors[0].Defense != 80 || s.Actors[0].Agility != 71 || s.Actors[0].Temporary != "" || s.Actors[2].HP != 0 || s.Actors[2].Temporary != "" || resumed.Command != c.Command || !resumed.Custom || resumed.Cursor != (corebattle.TargetCursor{ActorID: "enemy", Source: "item159"}) || resumed.Cursor != c.Cursor || resumed.EquipmentReset.Cursor != c.EquipmentReset.Cursor {
		t.Fatal(s, loaded.State, c, resumed)
	}
	// Both paths consume the same frozen arguments and RNG after rehydration.
	for _, run := range []struct {
		s *corebattle.LifetimeState
		c *corebattle.NPCJob97Context
		d BattleDefinitions
	}{{&s, c, d}, {&loaded.State, resumed, rebuilt}} {
		executeNPC97(t, run.s, run.c, run.d, 5, []string{"enemy", "tail"}, 0)
	}
	if !reflect.DeepEqual(s, loaded.State) || resumed.End != c.End || resumed.DamageCalls != c.DamageCalls || !reflect.DeepEqual(resumed.Arguments, []string{"tail"}) {
		t.Fatal(s, loaded.State, c, resumed)
	}
	// A completed callback's persisted facts are loaded as facts; durable
	// receipts deciding not to execute it again belong to the consumer.
	loaded.State, loaded.Cursor = s, resumed.Cursor
	loaded.End, loaded.DamageCalls = resumed.End, resumed.DamageCalls
	after, err := json.Marshal(loaded)
	if err != nil {
		t.Fatal(err)
	}
	var again checkpoint
	if err := json.Unmarshal(after, &again); err != nil || !reflect.DeepEqual(loaded, again) || again.End.ImmediateRiskCalls != 1 || again.DamageCalls != 1 {
		t.Fatal(err, again)
	}
}
