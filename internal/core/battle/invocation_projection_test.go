package battle

import (
	"errors"
	"reflect"
	"testing"

	"github.com/witchcraze/party2re/internal/core/random"
)

func TestInvocationProjectionIsolatesNestedStateAndContextOnRejection(t *testing.T) {
	s, c := rosterFixture()
	weapon, sp := 123, 150
	a := &s.Actors[0]
	a.Gear.WeaponAttack = &weapon
	a.JobHistory = map[string]*int{"job-59": &sp}
	a.Abilities = []string{"original"}
	a.ItemDefinitionIDs = []string{"item-1"}
	a.CustomSkills = []ActionCustomSkill{{Gems: []GemEffect{{Kind: "heal"}}}}
	s.SkillBans = map[string]int{"original": 3}
	s.SkillHistory = map[string]map[string]bool{"self": {"original": true}}
	frame := CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy"}}
	mutate := func(s *LifetimeState, f *CallbackContext) (bool, error) {
		a := &s.Actors[0]
		*a.Gear.WeaponAttack = 0
		*a.JobHistory["job-59"] = 0
		a.Abilities[0], a.ItemDefinitionIDs[0], a.CustomSkills[0].Gems[0].Kind = "changed", "changed", "changed"
		s.SkillBans["original"], s.SkillHistory["self"]["original"] = 0, false
		f.PartyIDs[0], f.EnemyIDs[0], f.Arguments[0] = "changed", "changed", "changed"
		return true, nil
	}
	d := CallbackCatalog{Entries: map[string]CallbackDefinition{
		"prior": {Validate: func(*LifetimeState, *CallbackContext) error { return nil }, Project: mutate,
			Execute: func(*LifetimeState, *CallbackContext, random.Generator) error {
				t.Fatal("external execution during admission")
				return nil
			}},
		"later": {Validate: func(*LifetimeState, *CallbackContext) error { return ErrUnsupportedEffect },
			Execute: func(*LifetimeState, *CallbackContext, random.Generator) error {
				t.Fatal("rejected execution")
				return nil
			}},
	}}
	ops := []CallbackInvocation{{Kind: "entry", EntryID: "prior"}, {Kind: "entry", EntryID: "later"}}
	if err := s.ApplyInvocations(&frame, d, ops, &targetRNG{t: t}); !errors.Is(err, ErrUnsupportedEffect) {
		t.Fatal(err)
	}
	if weapon != 123 || sp != 150 || a.Abilities[0] != "original" || a.ItemDefinitionIDs[0] != "item-1" || a.CustomSkills[0].Gems[0].Kind != "heal" || s.SkillBans["original"] != 3 || !s.SkillHistory["self"]["original"] || !reflect.DeepEqual(frame.PartyIDs, []string{"player"}) || frame.EnemyIDs[0] != "enemy" || frame.Arguments[0] != "enemy" {
		t.Fatal("admission changed an independently observed referent", s, frame)
	}
}

func TestInvocationProjectionCyclesStayConservative(t *testing.T) {
	s, c := rosterFixture()
	s.Actors[0].Jobs.OldJobID = "job-59"
	d := CallbackCatalog{JobTables: map[int]CallbackTable{59: {Variants: [][]CallbackEntry{{{ID: "loop", Name: "summon"}}}}}, Entries: map[string]CallbackDefinition{
		"loop": {Invocation: CallbackInvocation{Kind: "gem-87"}},
		"later": {Validate: func(_ *LifetimeState, c *CallbackContext) error {
			if !c.AfterEffects {
				t.Fatal("cycle claimed a known transition")
			}
			return ErrUnsupportedEffect
		}, Execute: func(*LifetimeState, *CallbackContext, random.Generator) error { return nil }},
	}}
	frame := CallbackContext{TargetContext: c.TargetContext}
	if err := s.ValidateInvocations(&frame, d, []CallbackInvocation{{Kind: "entry", EntryID: "loop"}, {Kind: "entry", EntryID: "later"}}, &targetRNG{t: t}); !errors.Is(err, ErrUnsupportedEffect) {
		t.Fatal(err)
	}
}
