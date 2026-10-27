package skill

import (
	"encoding/json"
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func TestNPC98ResetFrozenReloadWithoutCallbackReplay(t *testing.T) {
	s, c, d := npc98Fixture()
	s.Actors[0].NPC, c.ActorNPC = true, true
	s.Actors[0].WeaponSeal = 6
	c.Command, c.Custom = "frozen-ordinary-job98", true
	c.EquipmentReset.Seals = &corebattle.WeaponSealTable{DragonAttack: 17, PhoenixWeight: -31}
	s.Actors[0].Defense, s.Actors[1].Tension, s.Actors[2].Tension = 200, 5, 1.7
	s.Actors[1].Status, s.Actors[1].Temporary = corebattle.StatusMagicSeal, corebattle.TemporaryOneHitDefense
	s.Actors[2].Temporary = corebattle.TemporaryRage
	executeNPC98(t, &s, c, d, 4, nil)
	type checkpoint struct {
		State             corebattle.LifetimeState
		Target            corebattle.TargetContext
		Seals             corebattle.WeaponSealTable
		Entries           []corebattle.CallbackEntry
		NextPosition      int
		Arguments         []string
		CallbackCompleted bool
		DamageCalls       int
	}
	frozen := checkpoint{State: s, Target: c.TargetContext, Seals: *c.EquipmentReset.Seals, Entries: d.Invocations.JobTables[98].Variants[0], NextPosition: 1, Arguments: []string{"ignored", "tail"}, CallbackCompleted: true}
	data, err := json.Marshal(frozen)
	if err != nil {
		t.Fatal(err)
	}
	var loaded checkpoint
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatal(err)
	}
	if !loaded.CallbackCompleted || loaded.State.Actors[0].Tension != 3 || loaded.State.Actors[0].Temporary != corebattle.TemporaryDouble || !reflect.DeepEqual(loaded.Entries, corebattle.NPCJobEntries(98)) {
		t.Fatal(loaded)
	}
	resumed := &corebattle.NPCJob98Context{DamageContext: corebattle.DamageContext{
		EffectContext: corebattle.EffectContext{TargetContext: loaded.Target, ActorNPC: true},
		Defeat:        func(string) error { t.Fatal("reset must not defeat"); return nil },
	}, EquipmentReset: &corebattle.EquipmentResetContext{Seals: &loaded.Seals}}
	var rebuilt BattleDefinitions
	rebuilt.RegisterNPCJob98(resumed)
	for _, run := range []struct {
		s *corebattle.LifetimeState
		c *corebattle.NPCJob98Context
		d BattleDefinitions
	}{{&s, c, d}, {&loaded.State, resumed, rebuilt}} {
		executeNPC98(t, run.s, run.c, run.d, loaded.NextPosition, loaded.Arguments)
	}
	if !reflect.DeepEqual(s, loaded.State) || loaded.State.Actors[0].Tension != 3 || loaded.State.Actors[1].Tension != 5 || loaded.State.Actors[2].Tension != 1.7 || loaded.State.Actors[0].Defense != 80 || loaded.State.Actors[0].Agility != 71 || loaded.State.Actors[0].Temporary != "" || loaded.State.Actors[1].Status != "" || loaded.State.Actors[1].Temporary != "" || loaded.State.Actors[2].Temporary != "" || loaded.State.Actors[2].HP != 0 || resumed.Command != c.Command || !resumed.Custom || resumed.Cursor != c.Cursor {
		t.Fatal(s, loaded.State, c, resumed)
	}
	for _, run := range []struct {
		s *corebattle.LifetimeState
		c *corebattle.NPCJob98Context
		d BattleDefinitions
	}{{&s, c, d}, {&loaded.State, resumed, rebuilt}} {
		executeNPC98(t, run.s, run.c, run.d, 7, nil, 0)
	}
	loaded.DamageCalls = resumed.DamageCalls
	if !reflect.DeepEqual(s, loaded.State) || loaded.State.Actors[1].HP != 730 || loaded.State.Actors[1].Temporary != corebattle.TemporaryDouble || loaded.DamageCalls != 1 {
		t.Fatal(s, loaded.State, resumed)
	}
	// Loading the post-damage checkpoint preserves facts without reapplying
	// callbacks. Durable deduplication remains the run service's job.
	after, err := json.Marshal(loaded)
	if err != nil {
		t.Fatal(err)
	}
	var again checkpoint
	if err := json.Unmarshal(after, &again); err != nil || !reflect.DeepEqual(loaded, again) {
		t.Fatal(err, again)
	}
}
