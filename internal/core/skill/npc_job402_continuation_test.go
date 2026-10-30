package skill

import (
	"encoding/json"
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func TestNPC402FrozenReloadPreservesPaidManaAndChargeWithoutReplay(t *testing.T) {
	s, c, d := npc402Fixture()
	s.Actors[0].NPC, c.ActorNPC = true, true
	s.Actors[2].MP = 10
	c.Command, c.Custom = "frozen-job402", true
	executeNPC402(t, &s, c, d, 5, []string{"enemy", "tail"}, 0)
	type checkpoint struct {
		State             corebattle.LifetimeState
		Target            corebattle.TargetContext
		Reset             corebattle.EquipmentResetContext
		Entries           []corebattle.CallbackEntry
		Arguments         []string
		DamageCalls       int
		CallbackCompleted bool
	}
	frozen := checkpoint{State: s, Target: c.TargetContext, Reset: c.Death.EquipmentResetContext,
		Entries: d.Invocations.JobTables[402].Variants[0], Arguments: []string{"enemy", "enemy", "enemy", "tail"},
		DamageCalls: c.DamageCalls, CallbackCompleted: true}
	data, err := json.Marshal(frozen)
	if err != nil {
		t.Fatal(err)
	}
	var loaded checkpoint
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatal(err)
	}
	if !loaded.CallbackCompleted || loaded.State.Actors[2].MP != 5 || loaded.State.Actors[1].CMP != 80 || loaded.State.Actors[1].CMPNeedCharge != 6 || !reflect.DeepEqual(loaded.Entries, corebattle.NPCJobEntries(402)) {
		t.Fatal(loaded)
	}
	resumed := &corebattle.NPCJob402Context{DamageContext: corebattle.DamageContext{
		EffectContext: corebattle.EffectContext{TargetContext: loaded.Target, ActorNPC: true, DamageCalls: loaded.DamageCalls}, GuaranteedHit: true,
	}, Death: &corebattle.DefeatContext{EquipmentResetContext: loaded.Reset}}
	var rebuilt BattleDefinitions
	rebuilt.RegisterNPCJob402(resumed)
	for _, run := range []struct {
		s *corebattle.LifetimeState
		c *corebattle.NPCJob402Context
		d BattleDefinitions
	}{
		{&s, c, d}, {&loaded.State, resumed, rebuilt},
	} {
		executeNPC402(t, run.s, run.c, run.d, 0, loaded.Arguments, 0, 0, 0)
	}
	if !reflect.DeepEqual(s, loaded.State) || loaded.State.Actors[2].MP != 5 || loaded.State.Actors[1].HP != 562 || resumed.DamageCalls != 4 || resumed.Command != c.Command || !resumed.Custom || !reflect.DeepEqual(resumed.Arguments, []string{"tail"}) {
		t.Fatal(s, loaded, resumed)
	}
	// Reloading committed facts leaves the completed body unexecuted. The run
	// service owns durable receipts and chooses the next callback to execute.
	loaded.DamageCalls = resumed.DamageCalls
	after, err := json.Marshal(loaded)
	if err != nil {
		t.Fatal(err)
	}
	var again checkpoint
	if err := json.Unmarshal(after, &again); err != nil || !reflect.DeepEqual(loaded, again) {
		t.Fatal(err, again)
	}
}
