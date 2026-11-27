package battle

import (
	"errors"
	"reflect"
	"testing"

	"github.com/witchcraze/party2re/internal/core/random"
)

func TestInvocationLaterSlotsAdmitRegistrationOnly(t *testing.T) {
	s, c, d := invocationFixture()
	d.Entries["prior"] = CallbackDefinition{
		Validate: d.Entries["noop"].Validate,
		Execute: func(s *LifetimeState, c *CallbackContext, _ random.Generator) error {
			s.actor(c.ActorID).MP = 0
			return nil
		},
	}
	var seen []int
	reject := false
	d.Entries["later"] = CallbackDefinition{
		Validate: func(s *LifetimeState, c *CallbackContext) error {
			seen = append(seen, s.actor(c.ActorID).MP)
			if reject {
				return ErrUnsupportedEffect
			}
			return nil
		},
		Execute: d.Entries["noop"].Execute,
	}
	ops := []CallbackInvocation{{Kind: "entry", EntryID: "prior"}, {Kind: "entry", EntryID: "later"}}
	if err := s.ValidateInvocations(&c, d, ops, &targetRNG{t: t}); err != nil || len(seen) != 0 {
		t.Fatal("later slot validated before its predecessor ran", err, seen)
	}
	missing := []CallbackInvocation{ops[0], {Kind: "entry", EntryID: "missing"}}
	if err := s.ValidateInvocations(&c, d, missing, &targetRNG{t: t}); !errors.Is(err, ErrUnsupportedEffect) {
		t.Fatal("unregistered later slot admitted", err)
	}
	before := s.callbackCopy()
	reject = true
	if err := s.ApplyInvocations(&c, d, ops, &targetRNG{t: t}); !errors.Is(err, ErrUnsupportedEffect) || !reflect.DeepEqual(seen, []int{0}) || !reflect.DeepEqual(s, before) {
		t.Fatal("execution-time rejection did not see actual state or restore it", err, seen, s)
	}
	reject, seen = false, nil
	if err := s.ApplyInvocations(&c, d, ops, &targetRNG{t: t}); err != nil || !reflect.DeepEqual(seen, []int{0}) || s.actor("self").MP != 0 {
		t.Fatal(err, seen, s)
	}
}

func TestInvocationExecutionRejectionRestoresNestedStateAndContext(t *testing.T) {
	s, c := rosterFixture()
	weapon, sp := 123, 150
	a := &s.Actors[0]
	a.Gear.WeaponAttack = &weapon
	a.JobHistory = map[string]*int{"job-59": &sp}
	a.Abilities = []string{"original"}
	a.ItemDefinitionIDs = []string{"item-1"}
	a.CustomSkills = []ActionCustomSkill{{Gems: []GemEffect{{Kind: "heal"}}}}
	// Callers can hold these slices independently of the actor record.
	abilities, items, customs, gems := a.Abilities, a.ItemDefinitionIDs, a.CustomSkills, a.CustomSkills[0].Gems
	s.SkillBans = map[string]int{"original": 3}
	s.SkillHistory = map[string]map[string]bool{"self": {"original": true}}
	frame := CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy"}}
	mutate := func(s *LifetimeState, f *CallbackContext, _ random.Generator) error {
		a := &s.Actors[0]
		*a.Gear.WeaponAttack = 0
		*a.JobHistory["job-59"] = 0
		a.Abilities[0], a.ItemDefinitionIDs[0], a.CustomSkills[0].Gems[0].Kind = "changed", "changed", "changed"
		s.SkillBans["original"], s.SkillHistory["self"]["original"] = 0, false
		f.PartyIDs[0], f.EnemyIDs[0], f.Arguments[0] = "changed", "changed", "changed"
		return nil
	}
	d := CallbackCatalog{Entries: map[string]CallbackDefinition{
		"prior": {Validate: func(*LifetimeState, *CallbackContext) error { return nil }, Execute: mutate},
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
	if abilities[0] != "original" || items[0] != "item-1" || gems[0].Kind != "heal" || &customs[0].Gems[0] != &gems[0] {
		t.Fatal("rejection kept a change in an independently held slice", abilities, items, customs, gems)
	}
	if weapon != 123 || sp != 150 || a.Abilities[0] != "original" || a.ItemDefinitionIDs[0] != "item-1" || a.CustomSkills[0].Gems[0].Kind != "heal" || s.SkillBans["original"] != 3 || !s.SkillHistory["self"]["original"] || !reflect.DeepEqual(frame.PartyIDs, []string{"player"}) || frame.EnemyIDs[0] != "enemy" || frame.Arguments[0] != "enemy" {
		t.Fatal("rejection kept an independently observed referent change", s, frame)
	}
}

func TestInvocationCyclesCheckLaterRegistration(t *testing.T) {
	s, c := rosterFixture()
	s.Actors[0].Jobs.OldJobID = "job-59"
	d := CallbackCatalog{JobTables: map[int]CallbackTable{59: {Variants: [][]CallbackEntry{{{ID: "loop", Name: "summon"}}}}}, Entries: map[string]CallbackDefinition{
		"loop": {Invocation: CallbackInvocation{Kind: "gem_call_summon"}},
		"later": {Validate: func(*LifetimeState, *CallbackContext) error {
			t.Fatal("cycle claimed a known transition")
			return nil
		}, Execute: func(*LifetimeState, *CallbackContext, random.Generator) error { return nil }},
	}}
	frame := CallbackContext{TargetContext: c.TargetContext}
	if err := s.ValidateInvocations(&frame, d, []CallbackInvocation{{Kind: "entry", EntryID: "loop"}, {Kind: "entry", EntryID: "later"}}, &targetRNG{t: t}); err != nil {
		t.Fatal(err)
	}
	delete(d.Entries, "later")
	if err := s.ValidateInvocations(&frame, d, []CallbackInvocation{{Kind: "entry", EntryID: "loop"}, {Kind: "entry", EntryID: "later"}}, &targetRNG{t: t}); !errors.Is(err, ErrUnsupportedEffect) {
		t.Fatal("unregistered later admitted", err)
	}
}

func TestRandomAndGeneratedAdmissionKeepKnownInitialState(t *testing.T) {
	for _, kind := range []string{"random", "job-82-generated"} {
		s, c, d := invocationFixture()
		for job := 1; job <= 87; job++ {
			d.JobTables[job] = invocationTable(CallbackEntry{ID: "initial"})
		}
		validated := 0
		d.Entries["initial"] = CallbackDefinition{
			Validate: func(*LifetimeState, *CallbackContext) error {
				validated++
				return nil
			},
			Execute: func(*LifetimeState, *CallbackContext, random.Generator) error { return nil },
		}
		d.Entries["unknown"] = CallbackDefinition{
			Validate: func(*LifetimeState, *CallbackContext) error { return nil },
			Execute:  func(*LifetimeState, *CallbackContext, random.Generator) error { return nil },
		}
		if err := s.ValidateInvocations(&c, d, []CallbackInvocation{{Kind: kind}}, &targetRNG{t: t}); err != nil {
			t.Fatal(kind, err)
		}
		// After an unknown predecessor only registration is checked.
		validated = 0
		if err := s.ValidateInvocations(&c, d, []CallbackInvocation{{Kind: "entry", EntryID: "unknown"}, {Kind: kind}}, &targetRNG{t: t}); err != nil || validated != 0 {
			t.Fatal(kind, err, validated)
		}
	}
}

func TestInvocationRetainsNewOperationContext(t *testing.T) {
	// Synthetic callbacks check frame propagation only, not any concrete pool.
	for _, kind := range []string{"entry", "gem_call_summon"} {
		s, c, d := invocationFixture()
		s.actor(c.ActorID).Jobs.OldJobID = "job-59"
		d.Entries["created"] = CallbackDefinition{
			Validate: d.Entries["noop"].Validate,
			Execute: func(_ *LifetimeState, f *CallbackContext, _ random.Generator) error {
				f.Damage = &DamageContext{EffectContext: EffectContext{TargetContext: f.TargetContext, TensionCalls: 1}}
				return nil
			},
		}
		d.JobTables[59] = invocationTable(CallbackEntry{ID: "created", Name: "summon"})
		check := func(_ *LifetimeState, f *CallbackContext) error {
			if f.Damage == nil || f.Damage.TensionCalls != 1 {
				t.Fatal(kind, "lost newly created operation context", f)
			}
			return nil
		}
		d.Entries["later"] = CallbackDefinition{Validate: check, Execute: func(s *LifetimeState, f *CallbackContext, _ random.Generator) error { return check(s, f) }}
		op := CallbackInvocation{Kind: kind}
		if kind == "entry" {
			op.EntryID = "created"
		}
		if err := s.ApplyInvocations(&c, d, []CallbackInvocation{op, {Kind: "entry", EntryID: "later"}}, &targetRNG{t: t}); err != nil || c.Damage == nil || c.Damage.TensionCalls != 1 {
			t.Fatal(kind, err, c)
		}
	}
}
