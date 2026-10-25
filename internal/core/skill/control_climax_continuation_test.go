package skill

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/core/random"
)

func TestControlAdmissionPurityAndRegistration(t *testing.T) {
	for _, job := range []int{53, 58, 70} {
		for _, entry := range corebattle.ControlClimaxEntries(job) {
			if entry.ID == "job-53-entry-6" || entry.ID == "job-70-entry-3" {
				continue
			}
			for _, reject := range []bool{false, true} {
				s, c, d := controlFixture()
				c.PriorTemporary = map[string]corebattle.TemporaryEffect{"self": corebattle.TemporaryDouble}
				s.Actors[0].Jobs.JobID = fmt.Sprintf("job-%d", job)
				calls := 0
				c.Defeat = func(string) error { calls++; return nil }
				c.MPDrain = &corebattle.ControlMPDrain{
					Validate: func(*corebattle.LifetimeState, *corebattle.EffectContext, corebattle.TargetEffect) error { return nil },
					Apply: func(*corebattle.LifetimeState, *corebattle.EffectContext, corebattle.TargetEffect, random.Generator) (corebattle.EffectApplication, error) {
						calls++
						return corebattle.EffectApplication{}, nil
					},
				}
				if reject {
					delete(d.Invocations.Entries, entry.ID)
				}
				frame := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy", "tail"}}
				before, err := json.Marshal(s)
				if err != nil {
					t.Fatal(err)
				}
				original := frame
				gear := *s.Actors[1].Gear
				_, err = d.InvocationAction(&s, &frame, []string{entry.ID}, noArithmeticDraws{t})
				if (err != nil) != reject {
					t.Fatal(entry.ID, reject, err)
				}
				after, err := json.Marshal(s)
				if err != nil {
					t.Fatal(err)
				}
				if string(before) != string(after) || !reflect.DeepEqual(frame, original) || !reflect.DeepEqual(gear, *s.Actors[1].Gear) || c.DamageCalls != 0 || c.StatUpCalls != 0 || c.StatusCalls != 0 || calls != 0 || c.Arguments != nil || c.PriorTemporary["self"] != corebattle.TemporaryDouble {
					t.Fatal(entry.ID, reject, s, c, frame)
				}
			}
		}
	}
	s, c, d := controlFixture()
	f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy"}}
	for _, definitions := range []BattleDefinitions{{}, d} {
		if _, err := definitions.InvocationAction(&s, &f, []string{"job-58-entry-3"}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
			t.Fatal(err)
		}
	}
	// Full table contributions retain the other owners' entries and registrations.
	if len(d.Invocations.JobTables[53].Variants[0]) != 7 || len(d.Invocations.JobTables[58].Variants[0]) != 7 || len(d.Invocations.JobTables[70].Variants[0]) != 5 {
		t.Fatal(d)
	}
	if _, ok := d.Invocations.Entries["job-53-entry-6"]; ok {
		t.Fatal("field was certified")
	}
	s.Actors[0].Jobs.JobID = "job-53"
	d.Invocations.Entries["tool"] = corebattle.CallbackDefinition{Invocation: corebattle.CallbackInvocation{Kind: "tool-84"}}
	if _, err := d.InvocationAction(&s, &f, []string{"tool"}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
		t.Fatal(err)
	}
	rc := &corebattle.RevivalContext{TargetContext: c.TargetContext}
	d.RegisterRevivals(rc)
	d.RegisterControlClimax(c)
	s.Actors[2].MaxHP = 100
	executeControl(t, &s, c, d, "job-70-entry-3", nil, 0)
	if s.Actors[2].HP != 50 {
		t.Fatal(s)
	}
}

func TestControlOrderedAdmission(t *testing.T) {
	// Halo strips reflection, exposing the missing target gear before damage.
	s, c, d := controlFixture()
	s.Actors[0].Status = corebattle.StatusSleep
	s.Actors[1].Temporary = corebattle.TemporaryReflectMagic
	s.Actors[1].Gear = nil
	f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy"}}
	if _, err := d.InvocationAction(&s, &f, []string{"job-70-entry-4"}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
		t.Fatal(err)
	}
	// Transferred nullification replaces reflection, so the caster's missing
	// gear is unreachable. This accepted no-effect must not be over-rejected.
	s, c, d = controlFixture()
	s.Actors[0].Temporary = corebattle.TemporaryNullMagic
	s.Actors[0].Gear = nil
	s.Actors[1].Temporary = corebattle.TemporaryReflectMagic
	f.TargetContext = c.TargetContext
	executeControl(t, &s, c, d, "job-58-entry-6", []string{"enemy"})
	if s.Actors[1].Temporary != corebattle.TemporaryNullMagic || s.Actors[0].Temporary != "" || c.DamageCalls != 0 {
		t.Fatal(s, c)
	}
	// A first hit activates195; the later area hit must admit the now-reachable ally.
	s, c, d = controlFixture()
	s.Actors[1].ItemDefinitionIDs = []string{"item-195"}
	s.Actors[2].HP, s.Actors[2].Gear = 100, nil
	f.TargetContext = c.TargetContext
	if _, err := d.InvocationAction(&s, &f, []string{"job-70-entry-4"}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
		t.Fatal(err)
	}
	// An unknown earlier callback cannot authorize a later body against stale facts.
	s, c, d = controlFixture()
	f.TargetContext = c.TargetContext
	if _, err := d.InvocationAction(&s, &f, []string{"job-58-entry-1", "job-70-entry-4"}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
		t.Fatal(err)
	}
	if s.Actors[0].Status != "" || s.Actors[0].Attack != 100 {
		t.Fatal(s)
	}
	// A reflected magic seal replaces sleep; a later cure leaves the actor
	// eligible for195 confusion before the third status helper.
	s, c, d = controlFixture()
	s.Actors[0].Status = corebattle.StatusSleep
	s.Actors[1].Temporary = corebattle.TemporaryReflectMagic
	s.Actors[2].HP, s.Actors[2].Gear = 100, nil
	for _, id := range []string{"second", "third"} {
		a := s.Actors[1]
		a.ID = id
		a.Temporary = ""
		s.Actors = append(s.Actors, a)
		c.EnemyIDs = append(c.EnemyIDs, id)
	}
	c.Item195Activated = true
	f.TargetContext = c.TargetContext
	before, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	ops := []corebattle.CallbackInvocation{{Kind: "entry", EntryID: "job-58-entry-2"}}
	if err := s.ApplyInvocations(&f, d.Invocations, ops, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
		t.Fatal(err)
	}
	after, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) || c.StatusCalls != 0 {
		t.Fatal(s, c)
	}
}

func TestControlPossessionRestoresActorOnErrorAndAlias(t *testing.T) {
	s, c, d := controlFixture()
	s.Actors[1].HP = 1
	s.Actors[1].NPC = true
	sentinel := errors.New("defeat failed")
	c.Defeat = func(id string) error {
		if c.ActorID != "enemy" || !c.ActorNPC || id != "enemy" {
			t.Fatal(c, id)
		}
		return sentinel
	}
	f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy", "tail"}}
	ops, err := d.InvocationAction(&s, &f, []string{"job-53-entry-5"}, noArithmeticDraws{t})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ApplyInvocations(&f, d.Invocations, ops, &damageGemDraws{t: t, draws: []float64{0, .5, 0, 0}}); !errors.Is(err, sentinel) {
		t.Fatal(err)
	}
	if c.ActorID != "self" || c.ActorNPC || f.ActorID != "self" || c.Arguments[0] != "tail" {
		t.Fatal(c, f)
	}
	// A breath reflection possesses the requester itself; no context change leaks.
	s, c, d = controlFixture()
	s.Actors[1].Temporary = corebattle.TemporaryReflectBreath
	executeControl(t, &s, c, d, "job-53-entry-5", []string{"enemy"}, 0, .5, 0)
	if c.ActorID != "self" || s.Actors[1].HP != 855 || s.Actors[0].HP != 500 {
		t.Fatal(s, c)
	}
}

func TestControlReloadAndDelegatedCosts(t *testing.T) {
	s, c, d := controlFixture()
	executeControl(t, &s, c, d, "job-53-entry-5", []string{"enemy"}, 0, .5, 0, 0)
	type checkpoint struct {
		State       corebattle.LifetimeState
		Target      corebattle.TargetContext
		Prior       map[string]corebattle.TemporaryEffect
		Arguments   []string
		Entry       string
		DamageCalls int
	}
	frozen := checkpoint{s, c.TargetContext, map[string]corebattle.TemporaryEffect{"self": corebattle.TemporaryDouble}, []string{"enemy", "tail"}, "job-58-entry-6", c.DamageCalls}
	data, err := json.Marshal(frozen)
	if err != nil {
		t.Fatal(err)
	}
	var loaded checkpoint
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatal(err)
	}
	resumed := &corebattle.ControlClimaxContext{DamageContext: corebattle.DamageContext{EffectContext: corebattle.EffectContext{TargetContext: loaded.Target, DamageCalls: loaded.DamageCalls}, GuaranteedHit: true, Defeat: func(string) error { return nil }}, PriorTemporary: loaded.Prior}
	rebuilt := BattleDefinitions{}
	rebuilt.RegisterControlClimax(resumed)
	c.PriorTemporary = frozen.Prior
	for _, run := range []struct {
		s *corebattle.LifetimeState
		c *corebattle.ControlClimaxContext
		d BattleDefinitions
	}{{&s, c, d}, {&loaded.State, resumed, rebuilt}} {
		executeControl(t, run.s, run.c, run.d, loaded.Entry, loaded.Arguments, 0)
	}
	if !reflect.DeepEqual(s, loaded.State) || s.Actors[1].HP != 729 || c.DamageCalls != 2 || resumed.DamageCalls != 2 || !reflect.DeepEqual(c.Arguments, resumed.Arguments) {
		t.Fatal(s, loaded.State, c, resumed)
	}
	// Generated entries keep their listed MP and use the existing outer owner:
	// gem86 debits after the extended callback, while a naked entry does not.
	s, c, d = controlFixture()
	s.Actors[0].Jobs.JobID, s.Actors[0].Jobs.SP = "job-58", 160
	d.Invocations.Entries["extended"] = corebattle.CallbackDefinition{Invocation: corebattle.CallbackInvocation{Kind: "gem-86"}}
	executeControl(t, &s, c, d, "extended", []string{"enemy"}, 0)
	if s.Actors[0].MP != 42 || s.Actors[0].CMP != 90 || s.Actors[1].HP != 837 {
		t.Fatal(s)
	}
	// Tool delegation ignores the selected child's ordinary SP/listed MP.
	s, c, d = controlFixture()
	s.Actors[0].Jobs.JobID, s.Actors[0].Jobs.SP, s.Actors[0].MP = "job-70", 0, 0
	d.RegisterRevivals(&corebattle.RevivalContext{TargetContext: c.TargetContext})
	d.Invocations.Entries["tool"] = corebattle.CallbackDefinition{Invocation: corebattle.CallbackInvocation{Kind: "tool-84"}}
	executeControl(t, &s, c, d, "tool", nil, .4, 0)
	if s.Actors[0].MP != 0 || s.Actors[1].HP != 738 {
		t.Fatal(s)
	}
}
