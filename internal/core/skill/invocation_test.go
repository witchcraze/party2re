package skill

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/core/random"
)

func TestInvocationRegistrySharesWholeActionAdmission(t *testing.T) {
	s := corebattle.LifetimeState{Actors: []corebattle.LifetimeActor{{Participant: corebattle.Participant{ID: "self", TeamID: "party", HP: 100, MP: 20, CMP: 20}, Jobs: corebattle.BattleJobs{JobID: "job-73"}}}}
	c := corebattle.CallbackContext{TargetContext: corebattle.TargetContext{ActorID: "self", PartyIDs: []string{"self"}}}
	d := BattleDefinitions{Invocations: corebattle.CallbackCatalog{Entries: map[string]corebattle.CallbackDefinition{
		"extended": {Invocation: corebattle.CallbackInvocation{Kind: "gem-86"}},
		"book": {Validate: func(*corebattle.LifetimeState, *corebattle.CallbackContext) error { return nil },
			Execute: func(*corebattle.LifetimeState, *corebattle.CallbackContext, random.Generator) error {
				t.Fatal("preflight executed book")
				return nil
			},
			Children: []corebattle.CallbackInvocation{{Kind: "entry", EntryID: "unsupported-tool"}}},
	}}}
	before, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	for _, registry := range []BattleDefinitions{{}, d} {
		for _, ids := range [][]string{{"unregistered"}, {"extended", "book"}} {
			if _, err := registry.InvocationAction(&s, &c, ids, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
				t.Fatal(err)
			}
			ops := []corebattle.CallbackInvocation{}
			for _, id := range ids {
				ops = append(ops, corebattle.CallbackInvocation{Kind: "entry", EntryID: id})
			}
			if err := s.ApplyInvocations(&c, registry.Invocations, ops, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
				t.Fatal(err)
			}
		}
	}
	after, err := json.Marshal(s)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("unsupported later slot changed CMP/MP/state", err)
	}
	ops, err := d.InvocationAction(&s, &c, []string{"extended"}, noArithmeticDraws{t})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ApplyInvocations(&c, d.Invocations, ops, noArithmeticDraws{t}); err != nil || s.Actors[0].MP != 19 || s.Actors[0].Attack != 1 {
		t.Fatal(s, err)
	}
	if _, err := d.InvocationAction(nil, &c, []string{"extended"}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrInvalidRequest) {
		t.Fatal(err)
	}
}
