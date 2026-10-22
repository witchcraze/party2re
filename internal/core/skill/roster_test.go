package skill

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/core/random"
)

func rosterRegistryFixture() (corebattle.LifetimeState, corebattle.RosterContext, BattleDefinitions) {
	level := 70
	s := corebattle.LifetimeState{Actors: []corebattle.LifetimeActor{
		{Participant: corebattle.Participant{ID: "self", TeamID: "party", HP: 50, MaxHP: 100, Attack: 10, Gear: &corebattle.BattleGearStats{}}, Jobs: corebattle.BattleJobs{JobID: "job-60", OldJobID: "job-59"}, Hit: 95, Tension: 1},
		{Participant: corebattle.Participant{ID: "enemy", TeamID: "npc", HP: 1000, MaxHP: 1000, Gear: &corebattle.BattleGearStats{}}, NPC: true, Hit: 95, Tension: 1},
	}}
	c := corebattle.RosterContext{DamageContext: corebattle.DamageContext{EffectContext: corebattle.EffectContext{TargetContext: corebattle.TargetContext{ActorID: "self", PartyIDs: []string{"self"}, EnemyIDs: []string{"enemy"}}, RequesterLevel: &level}, GuaranteedHit: true, Defeat: func(string) error { return nil }}, EnemyTeamID: "npc"}
	d := BattleDefinitions{RosterSkills: map[string]bool{}, Invocations: corebattle.CallbackCatalog{JobTables: map[int]corebattle.CallbackTable{}, Entries: map[string]corebattle.CallbackDefinition{}}}
	return s, c, d
}

func TestRosterDiscoveryDelegationOldJobAndListedCostWaiver(t *testing.T) {
	s, c, d := rosterRegistryFixture()
	d.RegisterCallingJobs(&c)
	d.Invocations.Entries["calling"] = corebattle.CallbackDefinition{Invocation: corebattle.CallbackInvocation{Kind: "gem-87"}}
	frame := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy"}}
	if err := (BattleDefinitions{}).RosterAction(&s, &c, "job-59-summon", "", noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
		t.Fatal(err)
	}
	ops, err := d.InvocationAction(&s, &frame, []string{"calling"}, noArithmeticDraws{t})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ApplyInvocations(&frame, d.Invocations, ops, random.NewDeterministic(1)); err != nil {
		t.Fatal(err)
	}
	if len(s.Actors) != 3 || s.Actors[2].RosterIdentity != "calling-59" || s.Actors[0].MP != 0 || s.Actors[0].Jobs.OldSP != 0 || len(frame.PartyIDs) != 2 {
		t.Fatal(s, frame)
	}
	// First non-command selection uses the native rain composition without
	// charging listed MP/SP; helper stats and original actor remain separate.
	s.Actors[0].Jobs.OldJobID = "job-62"
	if _, err := s.ApplyRoster(&c, "job-62-summon", "", random.NewDeterministic(1)); err != nil {
		t.Fatal(err)
	}
	frame.TargetContext = c.TargetContext
	d.Invocations.Entries["first"] = corebattle.CallbackDefinition{Invocation: corebattle.CallbackInvocation{Kind: "gem-92"}}
	ops, err = d.InvocationAction(&s, &frame, []string{"first"}, noArithmeticDraws{t})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ApplyInvocations(&frame, d.Invocations, ops, random.NewDeterministic(2)); err != nil || s.Actors[0].MP != 0 || s.Actors[0].HP <= 50 || s.Actors[1].HP >= 1000 || frame.ActorID != "self" {
		t.Fatal(s, err)
	}
}

func TestRosterIndirectUnsupportedRejectsBeforeAnyMutation(t *testing.T) {
	s, c, d := rosterRegistryFixture()
	d.RegisterCallingJobs(&c)
	if _, err := s.ApplyRoster(&c, "job-59-summon", "", random.NewDeterministic(1)); err != nil {
		t.Fatal(err)
	}
	s.Actors[0].Jobs.OldJobID = "job-61"
	if _, err := s.ApplyRoster(&c, "job-61-summon", "", random.NewDeterministic(1)); err != nil {
		t.Fatal(err)
	}
	frame := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy"}}
	d.Invocations.Entries["unsupported"] = corebattle.CallbackDefinition{Invocation: corebattle.CallbackInvocation{Kind: "gem-90"}}
	d.Invocations.Entries["random"] = corebattle.CallbackDefinition{Invocation: corebattle.CallbackInvocation{Kind: "random"}}
	before, _ := json.Marshal(s)
	for _, ids := range [][]string{{"unsupported"}, {"random"}} {
		if _, err := d.InvocationAction(&s, &frame, ids, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
			t.Fatal(ids, err)
		}
		ops := make([]corebattle.CallbackInvocation, len(ids))
		for i, id := range ids {
			ops[i] = corebattle.CallbackInvocation{Kind: "entry", EntryID: id}
		}
		if err := s.ApplyInvocations(&frame, d.Invocations, ops, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
			t.Fatal(ids, err)
		}
	}
	after, _ := json.Marshal(s)
	if string(before) != string(after) {
		t.Fatal("enclosing rejection mutated state")
	}
}

func TestRosterInvocationConsumesOnlySourceUsedArgument(t *testing.T) {
	s, c, d := rosterRegistryFixture()
	d.RegisterCallingJobs(&c)
	frame := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy", "trailing"}}
	def := d.Invocations.Entries["job-59-summon"]
	if err := def.Execute(&s, &frame, random.NewDeterministic(1)); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(frame.Arguments, []string{"enemy", "trailing"}) {
		t.Fatal(frame.Arguments)
	}
	def = d.Invocations.Entries["job-59-attack"]
	if err := def.Execute(&s, &frame, random.NewDeterministic(1)); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(frame.Arguments, []string{"trailing"}) || frame.ActorID != "self" || c.ActorID != "self" {
		t.Fatal(frame, c)
	}
}
