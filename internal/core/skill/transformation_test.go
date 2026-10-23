package skill

import (
	"errors"
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/core/random"
)

func transformationRegistryFixture() (corebattle.LifetimeState, corebattle.TransformationContext, BattleDefinitions) {
	s := corebattle.LifetimeState{Actors: []corebattle.LifetimeActor{
		{Participant: corebattle.Participant{ID: "self", TeamID: "party", HP: 100, MaxHP: 100, MP: 200, MaxMP: 200, Attack: 100, Defense: 100, Agility: 100}, Jobs: corebattle.BattleJobs{JobID: "job-82", SP: 111, OldJobID: "job-36", OldSP: 100}},
		{Participant: corebattle.Participant{ID: "ally", TeamID: "party", HP: 99, MaxHP: 999, MP: 3, MaxMP: 20, Attack: 80, Defense: 70, Agility: 60}, Jobs: corebattle.BattleJobs{JobID: "job-79", SP: 77}},
		{Participant: corebattle.Participant{ID: "enemy", TeamID: "enemy", HP: 999, MaxHP: 999}},
	}}
	for i := range s.Actors {
		a := &s.Actors[i]
		a.Tension = 1
		a.BaseStats = corebattle.BattleBaseStats{Attack: a.Attack, Defense: a.Defense, Agility: a.Agility}
	}
	s.Actors[0].Icon, s.Actors[1].Icon = "self.svg", "ally.svg"
	c := corebattle.TransformationContext{EffectContext: corebattle.EffectContext{TargetContext: corebattle.TargetContext{ActorID: "self", PartyIDs: []string{"self", "ally"}, EnemyIDs: []string{"enemy"}}}, RequesterGender: "m", RequesterIcon: "self.svg"}
	var d BattleDefinitions
	return s, c, d
}

func TestTransformationCurrentJobDelegationRebuildsAfterFormWithoutCharging(t *testing.T) {
	s, c, d := transformationRegistryFixture()
	s.Actors[0].MP = 0 // Delegation ignores ordinary SP/listed MP admission.
	c.Damage = func(corebattle.TargetContext, corebattle.TargetEffect, random.Generator) error {
		t.Fatal("unselected damage body executed")
		return nil
	}
	d.RegisterTransformations(&c)
	frame := corebattle.CallbackContext{TargetContext: c.TargetContext}
	ops := []corebattle.CallbackInvocation{{Kind: "entry", EntryID: "job-82-form"}, {Kind: "tool-84"}}
	rng := &damageGemDraws{t: t, draws: []float64{0, 0, .99}}
	if err := s.ApplyInvocations(&frame, d.Invocations, ops, rng); err != nil || rng.used != 3 || s.Actors[0].Jobs.JobID != "job-300" || s.Actors[0].MP != 0 || s.Actors[0].Attack != 150 || s.Actors[0].Agility != 150 {
		t.Fatal(err, s, rng)
	}
	// The same selected tune cannot bypass missing descendants elsewhere in
	// the full transformed table; preflight rejects before selecting a child.
	s, c, d = transformationRegistryFixture()
	d.RegisterTransformations(&c)
	frame.TargetContext = c.TargetContext
	if err := s.ApplyInvocations(&frame, d.Invocations, ops, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) || s.Actors[0].Jobs.JobID != "job-82" || s.Actors[0].MP != 200 {
		t.Fatal(err, s)
	}
}

func TestTransformationRegistrationAndOrderedTables(t *testing.T) {
	s, c, d := transformationRegistryFixture()
	d.RegisterTransformations(&c)
	for job, count := range map[int]int{36: 5, 79: 5, 82: 7, 300: 4} {
		table := d.Invocations.JobTables[job]
		for _, entries := range table.Variants {
			if len(entries) != count {
				t.Fatal(job, entries)
			}
		}
	}
	if len(d.TransformationSkills) != 8 || len(d.Invocations.Entries) != 8 {
		t.Fatal(d)
	}
	for _, id := range []string{"job-36-copy", "job-79-breath", "job-79-speed", "job-82-form", "job-300-tune", "job-300-strike", "job-300-burst", "job-300-form"} {
		if !d.TransformationSkills[id] || d.Invocations.Entries[id].Execute == nil {
			t.Fatal(id)
		}
	}
	if _, err := (BattleDefinitions{}).TransformationAction(&s, &c, "job-82-form", "", noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
		t.Fatal(err)
	}
	frame := corebattle.CallbackContext{TargetContext: c.TargetContext}
	if err := s.ValidateInvocations(&frame, d.Invocations, []corebattle.CallbackInvocation{{Kind: "tool-84"}}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
		t.Fatal("partial job table was certified", err)
	}
	before := s.Actors[0]
	s.Actors[0].Jobs.JobID = "job-01"
	if err := s.ApplyInvocations(&frame, d.Invocations, []corebattle.CallbackInvocation{{Kind: "entry", EntryID: "job-82-form"}}, noArithmeticDraws{t}); err != nil || s.Actors[0].Icon != before.Icon || s.Actors[0].Jobs.JobID != "job-01" {
		t.Fatal("source no-effect rejected or mutated", err, s)
	}
}

func TestTransformationProjectionAdmitsChangedJobsAndCopyEligibility(t *testing.T) {
	for _, copyAllowed := range []bool{false, true} {
		s, c, d := transformationRegistryFixture()
		d.RegisterTransformations(&c)
		s.Actors[0].ItemDefinitionIDs = []string{"item-201"}
		s.Actors[0].Agility = 250
		s.Actors[1].HP = 0
		if copyAllowed {
			s.Actors[1].HP = 99
		}
		before := s.Actors[0]
		frame := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"ally", "tail"}}
		ops := []corebattle.CallbackInvocation{{Kind: "entry", EntryID: "job-36-copy"}, {Kind: "entry", EntryID: "job-79-speed"}}
		err := s.ApplyInvocations(&frame, d.Invocations, ops, noArithmeticDraws{t})
		if copyAllowed {
			if !errors.Is(err, corebattle.ErrUnsupportedEffect) || !reflect.DeepEqual(s.Actors[0], before) || c.StatUpCalls != 0 || len(frame.Arguments) != 2 {
				t.Fatal("copy reopened an unsupported agility tail", err, s, c)
			}
		} else if err != nil || s.Actors[0].Agility != 250 || c.StatUpCalls != 1 {
			t.Fatal("fallen-copy no-effect lost later capped stat guard", err, s, c)
		}
	}
}
