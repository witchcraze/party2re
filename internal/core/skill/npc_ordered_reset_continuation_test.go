package skill

import (
	"encoding/json"
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func TestNPCOrderedReloadContinuesWithoutCallbackReplay(t *testing.T) {
	s, c, d := npcOrderedFixture()
	s.Actors[0].NPC, c.ActorNPC = true, true
	level := 37
	c.RequesterLevel = &level
	c.Command, c.Custom = "frozen-vortex-command", true
	c.EquipmentReset.Seals = &corebattle.WeaponSealTable{DragonAttack: 17, PhoenixWeight: -31}
	s.Actors[0].WeaponSeal = 6
	// An enemy's accessory changes the package cursor before the ally resets.
	s.Actors[1].Gear.AccessoryID, s.Actors[1].ItemDefinitionIDs = "item-159", []string{"item-159"}
	executeNPCOrdered(t, &s, c, d, 3, []string{"ignored"}, 0, 0, 0, 0)
	type checkpoint struct {
		State                       corebattle.LifetimeState
		Target                      corebattle.TargetContext
		DamageCalls, RequesterLevel int
		Seals                       corebattle.WeaponSealTable
		Cursor                      corebattle.TargetCursor
		EntryID                     string
		Arguments                   []string
		CallbackCompleted           bool
	}
	// Ports are rebuilt, never serialized. A completed receipt resumes with
	// the next operation, whose frozen target differs from the first command.
	frozen := checkpoint{s, c.TargetContext, c.DamageCalls, level, *c.EquipmentReset.Seals, c.Cursor, "npc-job-403-position-2", []string{"enemy", "tail"}, true}
	data, err := json.Marshal(frozen)
	if err != nil {
		t.Fatal(err)
	}
	var loaded checkpoint
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatal(err)
	}
	if !loaded.CallbackCompleted || loaded.EntryID != "npc-job-403-position-2" || loaded.State.Actors[1].HP != 550 || loaded.DamageCalls != 1 || loaded.Cursor != (corebattle.TargetCursor{ActorID: "enemy", Source: "item159"}) || loaded.RequesterLevel != 37 || loaded.Seals != *c.EquipmentReset.Seals {
		t.Fatal(loaded)
	}
	resumed := &corebattle.NPCOrderedResetContext{DamageContext: corebattle.DamageContext{
		EffectContext: corebattle.EffectContext{TargetContext: loaded.Target, DamageCalls: loaded.DamageCalls, RequesterLevel: &loaded.RequesterLevel, ActorNPC: true},
		Cursor:        loaded.Cursor, GuaranteedHit: true,
		Defeat: func(string) error { t.Fatal("unexpected defeat"); return nil },
	}, EquipmentReset: &corebattle.EquipmentResetContext{Seals: &loaded.Seals}}
	var rebuilt BattleDefinitions
	rebuilt.RegisterNPCOrderedReset(resumed)
	for _, run := range []struct {
		s *corebattle.LifetimeState
		c *corebattle.NPCOrderedResetContext
		d BattleDefinitions
	}{{&s, c, d}, {&loaded.State, resumed, rebuilt}} {
		executeNPCOrdered(t, run.s, run.c, run.d, 2, loaded.Arguments, 0, 0, 0, 0, 0)
	}
	if !reflect.DeepEqual(s, loaded.State) || s.Actors[1].HP != 421 || resumed.DamageCalls != 4 || resumed.AreaDamageCalls != 0 || resumed.Command != c.Command || !resumed.Custom || !reflect.DeepEqual(resumed.Arguments, []string{"tail"}) || resumed.Cursor != c.Cursor {
		t.Fatal(s, loaded.State, c, resumed)
	}
}
