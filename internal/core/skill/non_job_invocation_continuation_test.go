package skill

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

// Full production tables and wrapper bodies; unrelated leaves use explicit
// test ports from playerDispatchFixture, never production certification.
func TestNonJobInvocationGeneralRandomFullTableBindings(t *testing.T) {
	for _, id := range []string{"tool-85", "gem-83"} {
		for _, row := range []struct{ job, size, position int }{{1, 7, 2}, {2, 9, 4}} {
			t.Run(fmt.Sprintf("%s/job%d", id, row.job), func(t *testing.T) {
				s, c, d := playerDispatchFixture()
				s.Actors[0].MP, s.Actors[0].Jobs.SP = 1, 0
				child := fmt.Sprintf("job-%d-entry-%d", row.job, row.position)
				d.Invocations.Entries[child] = baseGuardInvocation(c, child)
				d.RegisterNonJobInvocations()
				f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"fallen", "tail"}}
				// First position is extended: redraw in the same job, not a new job.
				rng := &damageGemDraws{t: t, draws: []float64{(float64(row.job) - .5) / 87, 0, (float64(row.position) + .5) / float64(row.size)}}
				runNonJobInvocation(t, &s, &f, d, []string{id}, rng)
				if rng.used != 3 || s.Actors[0].Temporary != corebattle.TemporaryCovering || s.Actors[2].Temporary != corebattle.TemporaryCover || s.Actors[0].MP != 1 || !reflect.DeepEqual(c.Arguments, []string{"tail"}) {
					t.Fatal(s, c, rng.used)
				}
			})
		}
	}
}

func TestNonJobInvocationTool84StarterDuplicatePositions(t *testing.T) {
	for pos := range 3 {
		s, c, d := baseGuardFixture()
		s.Actors[0].Jobs.JobID = "starter"
		d.RegisterNonJobInvocations()
		f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy", "tail"}}
		runNonJobInvocation(t, &s, &f, d, []string{"tool-84"}, &damageGemDraws{t: t, draws: []float64{(float64(pos) + .5) / 5, .5, 0}})
		if s.Actors[1].HP != 977 || !reflect.DeepEqual(c.Arguments, []string{"tail"}) {
			t.Fatal(s, c)
		}
	}
}

func TestNonJobInvocationFrozenInputAndFactsReload(t *testing.T) {
	s, c, d := callingManifestFixture()
	addCallingManifestHelper(&s, &c, 59)
	d.RegisterCallingJobs(&c)
	d.RegisterNonJobInvocations()
	f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy", "tail"}}
	input := struct {
		State     corebattle.LifetimeState
		Target    corebattle.TargetContext
		Arguments []string
		IDs       []string
		Draws     []float64
	}{s, f.TargetContext, f.Arguments, []string{"gem-89"}, []float64{0, 0, 0, 0, 0}}
	data, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	beforeContext := f.TargetContext
	beforeGear := *s.Actors[0].Gear
	if _, err := d.InvocationAction(&s, &f, input.IDs, noArithmeticDraws{t}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(f.TargetContext, beforeContext) || !reflect.DeepEqual(beforeGear, *s.Actors[0].Gear) || c.DamageCalls != 0 {
		t.Fatal("admission mutated input")
	}
	runNonJobInvocation(t, &s, &f, d, input.IDs, &damageGemDraws{t: t, draws: input.Draws})
	if s.Actors[1].HP != 9933 || s.Actors[0].MP != 100 || f.ActorID != "self" {
		t.Fatal(s, f)
	}
	if err := json.Unmarshal(data, &input); err != nil {
		t.Fatal(err)
	}
	_, loadedContext, loadedDefinitions := callingManifestFixture()
	loadedContext.TargetContext = input.Target
	loadedDefinitions.RegisterCallingJobs(&loadedContext)
	loadedDefinitions.RegisterNonJobInvocations()
	loadedFrame := corebattle.CallbackContext{TargetContext: input.Target, Arguments: input.Arguments}
	runNonJobInvocation(t, &input.State, &loadedFrame, loadedDefinitions, input.IDs, &damageGemDraws{t: t, draws: input.Draws})
	if !reflect.DeepEqual(s, input.State) || !reflect.DeepEqual(f.TargetContext, loadedFrame.TargetContext) {
		t.Fatal("reload changed result")
	}
	// A committed consumer returns these facts, without another callback call.
	result, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	var loaded corebattle.LifetimeState
	if err := json.Unmarshal(result, &loaded); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(s, loaded) {
		t.Fatal("result facts lost on reload")
	}
}

func TestNonJobInvocationCallingNoEffectAndHiddenLaterChild(t *testing.T) {
	for gem := 87; gem <= 92; gem++ {
		s, c, d := callingManifestFixture()
		d.RegisterCallingJobs(&c)
		d.RegisterNonJobInvocations()
		f := corebattle.CallbackContext{TargetContext: c.TargetContext}
		if gem == 87 {
			// Existing fallen preferred helper prevents summoning/fallback to60.
			addCallingManifestHelper(&s, &c, 59)
			s.Actors[3].HP = 0
			f.TargetContext = c.TargetContext
		}
		before, err := json.Marshal(s)
		if err != nil {
			t.Fatal(err)
		}
		runNonJobInvocation(t, &s, &f, d, []string{fmt.Sprintf("gem-%d", gem)}, noArithmeticDraws{t})
		after, err := json.Marshal(s)
		if err != nil || string(before) != string(after) {
			t.Fatal(gem, "no effect changed state", err)
		}
	}
	s, c, d := npc402Fixture()
	d.RegisterNonJobInvocations()
	f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy"}}
	delete(d.Invocations.Entries, "npc-job-402-position-5")
	if _, err := d.InvocationAction(&s, &f, []string{"tool-84"}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
		t.Fatal(err)
	}
	if s.Actors[1].HP != 1000 || s.Actors[0].MP != 80 || c.DamageCalls != 0 {
		t.Fatal("hidden later child partially executed")
	}
}

func TestNonJobInvocationDirectNPCStartsWithoutArguments(t *testing.T) {
	s, c, d := npc402Fixture()
	s.Actors[0].NPC, c.ActorNPC = true, true
	d.RegisterNonJobInvocations()
	f := corebattle.CallbackContext{TargetContext: c.TargetContext}
	runNonJobInvocation(t, &s, &f, d, []string{"tool-84"}, callingZeroDraw{})
	if s.Actors[1].HP != 703 || len(c.Arguments) != 0 || f.ActorID != "self" || c.DamageCalls != 3 {
		t.Fatal(s, c, f)
	}
}
