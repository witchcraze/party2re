package skill

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/core/random"
)

// Real tables and invoking bodies are under test. Other leaves are explicit
// test ports; their intrinsic certification belongs to the individual owners.
func playerDispatchFixture() (corebattle.LifetimeState, *corebattle.BaseGuardContext, BattleDefinitions) {
	s, c, _ := baseGuardFixture()
	var d BattleDefinitions
	d.RegisterPlayerCallbacks(PlayerCallbackContexts{BaseGuard: c, Transformations: &corebattle.TransformationContext{}}, 3)
	for _, table := range d.Invocations.JobTables {
		for _, entries := range table.Variants {
			for _, entry := range entries {
				if _, ok := d.Invocations.Entries[entry.ID]; !ok {
					d.Invocations.Entries[entry.ID] = corebattle.CallbackDefinition{}
				}
			}
		}
	}
	for id, def := range d.Invocations.Entries {
		if def.Invocation.Kind != "" {
			continue
		}
		d.Invocations.Entries[id] = corebattle.CallbackDefinition{
			Validate: func(*corebattle.LifetimeState, *corebattle.CallbackContext) error { return nil },
			Execute:  func(*corebattle.LifetimeState, *corebattle.CallbackContext, random.Generator) error { return nil },
		}
	}
	return s, c, d
}

// ff97fc5:_skill.cgi:3860,3886,9773–9787. Unlearned MP5 child runs at MP1.
func TestPlayerRandomBindingsUseFullTablesAndIntrinsicChild(t *testing.T) {
	for _, id := range []string{"job-55-entry-6", "job-56-entry-5"} {
		s, c, d := playerDispatchFixture()
		s.Actors[0].MP, s.Actors[0].Jobs.SP = 1, 0
		d.Invocations.Entries["job-2-entry-3"] = baseGuardInvocation(c, "job-2-entry-3")
		f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"ally", "tail"}}
		ops, err := d.InvocationAction(&s, &f, []string{id}, noArithmeticDraws{t})
		if err != nil {
			t.Fatal(err)
		}
		rng := &damageGemDraws{t: t, draws: []float64{1.1 / 87, 3.1 / 9}}
		if err := s.ApplyInvocations(&f, d.Invocations, ops, rng); err != nil {
			t.Fatal(err)
		}
		if rng.used != 2 || s.Actors[0].Temporary != corebattle.TemporaryDeflect || s.Actors[0].MP != 1 {
			t.Fatal(s.Actors[0], rng.used)
		}
	}
}

// S=612 selects source job3 and position4 (612%87=3,612%8=4).
// ff97fc5:_skill.cgi:6537–6550: capture MP3, check internally, do not debit it.
func TestPlayerGeneratedBindingFreezesChildAcrossReload(t *testing.T) {
	for _, mp := range []int{2, 3} {
		var finals []corebattle.LifetimeState
		for _, reload := range []bool{false, true} {
			s, c, d := playerDispatchFixture()
			a := &s.Actors[0]
			a.Attack, a.Defense, a.Agility = 612-len(s.Actors), 0, 0
			a.MP = mp
			f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"ally", "tail"}}
			entries, err := d.PlayerCallbackTable(&s, &f, "job-82")
			if err != nil {
				t.Fatal(err)
			}
			op := entries[0].Invocation
			if op.GeneratedChildID != "job-3-entry-4" || op.GeneratedChildMP != 3 || entries[0].MPCost != 3 {
				t.Fatal(entries[0])
			}
			// A submitted child must survive a later change in actor stats.
			s.Actors[0].Attack = 1
			if reload {
				input := struct {
					State     corebattle.LifetimeState
					Target    corebattle.TargetContext
					Arguments []string
					Op        corebattle.CallbackInvocation
				}{s, f.TargetContext, f.Arguments, op}
				data, err := json.Marshal(input)
				if err != nil {
					t.Fatal(err)
				}
				if err := json.Unmarshal(data, &input); err != nil {
					t.Fatal(err)
				}
				s, op = input.State, input.Op
				f = corebattle.CallbackContext{TargetContext: input.Target, Arguments: input.Arguments}
				c.TargetContext = f.TargetContext
				var rebuilt BattleDefinitions
				rebuilt.RegisterPlayerCallbacks(PlayerCallbackContexts{BaseGuard: c, Transformations: &corebattle.TransformationContext{}}, 3)
				d = rebuilt
			}
			d.Invocations.Entries["job-3-entry-4"] = baseGuardInvocation(c, "job-3-entry-4")
			if err := s.ApplyInvocations(&f, d.Invocations, []corebattle.CallbackInvocation{op}, noArithmeticDraws{t}); err != nil {
				t.Fatal(err)
			}
			if s.Actors[0].MP != mp || (s.Actors[0].Temporary == corebattle.TemporaryStrongDefense) != (mp == 3) {
				t.Fatal(s.Actors[0])
			}
			finals = append(finals, s)
		}
		if !reflect.DeepEqual(finals[0], finals[1]) {
			t.Fatal(finals)
		}
	}
}

func TestPlayerDispatchMissingDescendantRejectsBeforeMutation(t *testing.T) {
	for _, id := range []string{"job-55-entry-6", "job-56-entry-5", "job-82-entry-0"} {
		s, c, d := playerDispatchFixture()
		delete(d.Invocations.Entries, "job-3-entry-4")
		f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"ally", "tail"}}
		before, err := json.Marshal(s)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := d.InvocationAction(&s, &f, []string{id}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
			t.Fatal(id, err)
		}
		after, err := json.Marshal(s)
		if err != nil {
			t.Fatal(err)
		}
		if string(before) != string(after) || !reflect.DeepEqual(f.Arguments, []string{"ally", "tail"}) {
			t.Fatal(s, f)
		}
	}
}

func TestPlayerGeneratedExcludedExtendedPositionDoesNotBlock(t *testing.T) {
	s, c, d := playerDispatchFixture()
	// Job2 has N9: p0 cannot share residue2 modulo gcd(87,9)=3.
	// General random also redraws this extended position; tool84 includes it.
	delete(d.Invocations.Entries, "job-2-entry-0")
	f := corebattle.CallbackContext{TargetContext: c.TargetContext}
	if _, err := d.InvocationAction(&s, &f, []string{"job-82-entry-0"}, noArithmeticDraws{t}); err != nil {
		t.Fatal(err)
	}
	s.Actors[0].Jobs.JobID = "job-02"
	if err := s.ValidateInvocations(&f, d.Invocations, []corebattle.CallbackInvocation{{Kind: "tool-84"}}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
		t.Fatal(err)
	}
}

func TestPlayerGeneratedProjectionCarriesEarlierEligibility(t *testing.T) {
	s, c, d := playerDispatchFixture()
	s.Actors[0].MP = 3
	d.Invocations.Entries["job-3-entry-4"] = baseGuardInvocation(c, "job-3-entry-4")
	d.Invocations.Entries["later"] = corebattle.CallbackDefinition{
		Validate: func(s *corebattle.LifetimeState, f *corebattle.CallbackContext) error {
			if f.AfterEffects || s.Actors[0].Temporary != corebattle.TemporaryStrongDefense {
				return corebattle.ErrUnsupportedEffect
			}
			return nil
		},
		Execute: func(s *corebattle.LifetimeState, _ *corebattle.CallbackContext, _ random.Generator) error {
			s.Actors[0].Hit = 100
			return nil
		},
	}
	f := corebattle.CallbackContext{TargetContext: c.TargetContext}
	ops := []corebattle.CallbackInvocation{{Kind: "entry", EntryID: "job-82-entry-0", GeneratedChildID: "job-3-entry-4", GeneratedChildMP: 3}, {Kind: "entry", EntryID: "later"}}
	if err := s.ApplyInvocations(&f, d.Invocations, ops, noArithmeticDraws{t}); err != nil || s.Actors[0].Hit != 100 {
		t.Fatal(err, s.Actors[0])
	}
}

func TestPlayerCapturedGeneratedMPGuardSkipsUnavailableChild(t *testing.T) {
	s, c, d := playerDispatchFixture()
	delete(d.Invocations.Entries, "job-3-entry-4")
	f := corebattle.CallbackContext{TargetContext: c.TargetContext}
	op := corebattle.CallbackInvocation{Kind: "entry", EntryID: "job-82-entry-0", GeneratedChildID: "job-3-entry-4", GeneratedChildMP: 3}
	s.Actors[0].MP = 2
	if err := s.ApplyInvocations(&f, d.Invocations, []corebattle.CallbackInvocation{op}, noArithmeticDraws{t}); err != nil || s.Actors[0].MP != 2 {
		t.Fatal(err, s.Actors[0])
	}
	s.Actors[0].MP = 3
	if err := s.ApplyInvocations(&f, d.Invocations, []corebattle.CallbackInvocation{op}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
		t.Fatal(err)
	}
}
