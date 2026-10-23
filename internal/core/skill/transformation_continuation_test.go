package skill

import (
	"encoding/json"
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/core/random"
)

func TestTransformationContinuationRebuildsFromFrozenFactsWithoutReplay(t *testing.T) {
	var finals []corebattle.LifetimeState
	for _, reload := range []bool{false, true} {
		s, c, _ := transformationRegistryFixture()
		// The operation owner stores only facts and the next callback cursor.
		input := struct {
			Target       corebattle.TargetContext
			Icon, Gender string
			Next         int
		}{c.TargetContext, c.RequesterIcon, "f", 0}
		steps := []struct {
			id, target string
			draws      []float64
		}{
			{"job-82-form", "", nil},
			{"job-300-tune", "", []float64{0, .99}},
			{"job-300-form", "", nil},
			{"job-36-copy", "ally", nil},
			{"job-79-speed", "", nil},
		}
		for input.Next < len(steps) {
			step := steps[input.Next]
			// Closures are rebuilt against the loaded state and frozen requester.
			c = corebattle.TransformationContext{EffectContext: corebattle.EffectContext{TargetContext: input.Target}, RequesterIcon: input.Icon, RequesterGender: input.Gender}
			var d BattleDefinitions
			d.RegisterTransformations(&c)
			frame := corebattle.CallbackContext{TargetContext: input.Target, Arguments: []string{step.target}}
			rng := &damageGemDraws{t: t, draws: step.draws}
			if err := s.ApplyInvocations(&frame, d.Invocations, []corebattle.CallbackInvocation{{Kind: "entry", EntryID: step.id}}, rng); err != nil || rng.used != len(step.draws) {
				t.Fatal(err, rng)
			}
			input.Target, input.Next = frame.TargetContext, input.Next+1
			if reload {
				data, err := json.Marshal(struct {
					State corebattle.LifetimeState
					Input any
				}{s, input})
				if err != nil {
					t.Fatal(err)
				}
				var loaded struct {
					State corebattle.LifetimeState
					Input json.RawMessage
				}
				if err := json.Unmarshal(data, &loaded); err != nil {
					t.Fatal(err)
				}
				if err := json.Unmarshal(loaded.Input, &input); err != nil {
					t.Fatal(err)
				}
				s = loaded.State
			}
		}
		if s.Actors[0].MP != 50 || s.Actors[0].Jobs.JobID != "job-79" || s.Actors[0].Agility != 150 || s.Actors[0].Icon != input.Icon {
			t.Fatal(s)
		}
		finals = append(finals, s)
	}
	if !reflect.DeepEqual(finals[0], finals[1]) {
		t.Fatal(finals)
	}
}

func TestTransformationCapturedAppearanceAndArgumentFrames(t *testing.T) {
	for _, gender := range []string{"m", "f"} {
		s, c, d := transformationRegistryFixture()
		c.RequesterGender = gender
		s.Actors[0].NPC = true
		d.RegisterTransformations(&c)
		frame := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"ally", "trailing"}}
		table := d.Invocations.JobTables[79]
		selected := table.Variants[table.Select(&s, &frame)][3]
		if selected.ID != "job-79-breath" {
			t.Fatal(selected)
		}
		s.Actors[0].Icon = "assets/images/placeholders/battle/forms.svg#winged-" + gender
		if table.Select(&s, &frame) != 1 {
			t.Fatal("table did not rebuild from current appearance")
		}
		calls := 0
		c.AreaDamage = func(corebattle.TargetContext, float64, corebattle.CallbackCategory, bool, random.Generator) error {
			calls++
			return nil
		}
		if err := s.ApplyInvocations(&frame, d.Invocations, []corebattle.CallbackInvocation{{Kind: "entry", EntryID: selected.ID}}, noArithmeticDraws{t}); err != nil || calls != 1 || c.StatUpCalls != 0 {
			t.Fatal("captured body changed with appearance", err, c)
		}
		// Naked callback consumes one argument; the outer gem boundary gives each
		// slot the original target instead of carrying this shifted child slice.
		if err := d.Invocations.Entries["job-36-copy"].Execute(&s, &frame, noArithmeticDraws{t}); err != nil || !reflect.DeepEqual(frame.Arguments, []string{"trailing"}) || s.Actors[0].Jobs != s.Actors[1].Jobs {
			t.Fatal(err, frame, s)
		}
	}
}

func TestTransformationAdmissionDoesNotTouchSourceReferences(t *testing.T) {
	s, c, d := transformationRegistryFixture()
	d.RegisterTransformations(&c)
	sp := 19
	s.Actors[0].JobHistory = map[string]*int{"job-1": &sp}
	s.Actors[0].Abilities = []string{"source"}
	s.Actors[0].ItemDefinitionIDs = []string{"item-201"}
	frame := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"ally", "trailing"}}
	before, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	contextBefore := c
	for _, last := range []string{"job-82-form", "job-79-speed"} {
		ops := []corebattle.CallbackInvocation{{Kind: "entry", EntryID: "job-36-copy"}, {Kind: "entry", EntryID: last}}
		err := s.ValidateInvocations(&frame, d.Invocations, ops, noArithmeticDraws{t})
		if (err == nil) != (last == "job-82-form") {
			t.Fatal(last, err)
		}
		after, err := json.Marshal(s)
		if err != nil || string(before) != string(after) || !reflect.DeepEqual(c, contextBefore) || sp != 19 || s.Actors[0].Abilities[0] != "source" || s.Actors[0].ItemDefinitionIDs[0] != "item-201" || !reflect.DeepEqual(frame.Arguments, []string{"ally", "trailing"}) {
			t.Fatal("admission changed independently observed references", err, s, c, frame)
		}
	}
}
