package battle

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/witchcraze/party2re/internal/core/random"
)

// The injected draw tape stands for the consumer's frozen command randomness;
// SQL state/receipt ownership remains outside core. No RNG object is persisted.
type callbackTape struct {
	Draws    []float64
	Position int
	t        *testing.T
}

func (r *callbackTape) Float64() float64 {
	r.t.Helper()
	if r.Position >= len(r.Draws) {
		r.t.Fatal("unexpected draw")
	}
	v := r.Draws[r.Position]
	r.Position++
	return v
}
func (r *callbackTape) Intn(int) int   { r.t.Fatal("unexpected integer draw"); return 0 }
func (r *callbackTape) IntN(n int) int { return r.Intn(n) }

func TestInvocationContinuationAndRetryFromFrozenCommandInput(t *testing.T) {
	var final LifetimeState
	var finalTape callbackTape
	for _, reload := range []bool{false, true} {
		s, c, d := invocationFixture()
		addGeneratedCallback(&d)
		d.Entries["form"] = CallbackDefinition{Validate: d.Entries["noop"].Validate, Execute: func(s *LifetimeState, c *CallbackContext, _ random.Generator) error {
			a := s.actor(c.ActorID)
			a.Jobs.JobID, a.MP = "job-300", 1
			a.ConsumedItems = append(a.ConsumedItems, ConsumedItem{InstanceID: "owned-instance", ID: "item-test", Quantity: 1})
			s.actor("ally").MP = 10
			return nil
		}}
		d.JobTables[4] = invocationTable(CallbackEntry{ID: "form", MPCost: 20, Extended: true}, CallbackEntry{ID: "noop"})
		d.Entries["grow"] = CallbackDefinition{Validate: d.Entries["noop"].Validate, Execute: func(s *LifetimeState, c *CallbackContext, _ random.Generator) error {
			s.actor(c.ActorID).MP += 30
			s.Actors = append(s.Actors, LifetimeActor{Participant: Participant{ID: "helper", TeamID: "party", HP: 5}})
			c.PartyIDs = append(c.PartyIDs, "helper")
			return nil
		}}
		d.JobTables[300] = invocationTable(CallbackEntry{ID: "grow", MPCost: 999, RequiredSP: 999, Extended: true})
		rng := callbackTape{Draws: []float64{81.1 / 87, 0, 0}, t: t}
		for _, op := range []CallbackInvocation{{Kind: "random"}, {Kind: "tool-84"}} {
			input := struct {
				State LifetimeState
				Tape  callbackTape
			}{s, rng}
			data, err := json.Marshal(input)
			if err != nil {
				t.Fatal(err)
			}
			if reload {
				if err := json.Unmarshal(data, &input); err != nil {
					t.Fatal(err)
				}
				s, rng = input.State, input.Tape
				rng.t = t
			}
			// Repeated discovery must leave the frozen stream untouched.
			for range 2 {
				if err := s.ValidateInvocations(&c, d, []CallbackInvocation{op}, &rng); err != nil {
					t.Fatal(err)
				}
			}
			if err := s.ApplyInvocations(&c, d, []CallbackInvocation{op}, &rng); err != nil {
				t.Fatal(err)
			}
			// A rolled-back attempt reuses exactly the persisted pre-command input
			// and draws; a committed receipt must return facts without invoking core.
			var retry struct {
				State LifetimeState
				Tape  callbackTape
			}
			if err := json.Unmarshal(data, &retry); err != nil {
				t.Fatal(err)
			}
			retry.Tape.t = t
			retryContext := c
			retryContext.PartyIDs = []string{"self", "ally"}
			if err := retry.State.ApplyInvocations(&retryContext, d, []CallbackInvocation{op}, &retry.Tape); err != nil || !reflect.DeepEqual(retry.State, s) || retry.Tape.Position != rng.Position {
				t.Fatal("retry changed draws/effects", retry, s, err)
			}
			if reload {
				s = reloadLifetime(t, s)
			}
		}
		if s.actor("self").MP != 50 || s.actor("ally").MP != 10 || len(s.actor("self").ConsumedItems) != 1 || len(s.Actors) != 4 || rng.Position != 3 {
			t.Fatal(s, rng)
		}
		if reload {
			if !reflect.DeepEqual(s, final) || rng.Position != finalTape.Position {
				t.Fatal("reload changed continuation", s, final)
			}
		} else {
			final, finalTape = s, rng
		}
	}
}
