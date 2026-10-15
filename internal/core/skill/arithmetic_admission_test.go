package skill

import (
	"errors"
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func TestArithmeticRegistryAdmitsWholeDiscoveredCallback(t *testing.T) {
	s := corebattle.LifetimeState{Actors: []corebattle.LifetimeActor{{Participant: corebattle.Participant{ID: "self", TeamID: "party", HP: 1000, MP: 4, MaxHP: 1000}}}}
	c := corebattle.ArithmeticContext{TargetContext: corebattle.TargetContext{ActorID: "self", PartyIDs: []string{"self"}}, SubmittedText: "＠ＭＰ４＠×反射 ", Discovered: corebattle.ArithmeticEntries("job-72")}
	rng := noArithmeticDraws{t}
	definitions := BattleDefinitions{ArithmeticSkills: map[string]bool{"job-72-divisor-4": true}}
	before := s
	before.Actors = append([]corebattle.LifetimeActor(nil), s.Actors...)
	for _, d := range []BattleDefinitions{{}, definitions} {
		if _, err := d.ArithmeticAction(&s, &c, "job-72-divisor-4", "", rng); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
			t.Fatal(err)
		}
	}
	definitions.ArithmeticSkills["job-72-reflect"] = true
	entry, err := definitions.ArithmeticAction(&s, &c, "job-72-divisor-4", "", rng)
	if err != nil || entry.ID != "job-72-divisor-4" || entry.MPCost != 14 || entry.RequiredSP != 100 || !reflect.DeepEqual(s, before) {
		t.Fatal(entry, err, s)
	}
	if _, err := s.ApplyArithmetic(&c, entry.ID, "", rng); err != nil || s.Actors[0].Temporary != corebattle.TemporaryReflectMagic {
		t.Fatal(s, err)
	}
	// Registration cannot launder an unknown callback, or a missing defeat owner.
	for _, id := range []string{"unknown", "job-31-death"} {
		s.Actors[0].HP, s.Actors[0].MP = 999, 5
		definitions.ArithmeticSkills[id] = true
		if _, err := definitions.ArithmeticAction(&s, &c, id, "", rng); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
			t.Fatal(id, err)
		}
	}
}

func TestArithmeticRegistrationKeepsBatchAndIndirectClosuresGated(t *testing.T) {
	definitions := BattleDefinitions{ArithmeticSkills: make(map[string]bool)}
	s := corebattle.LifetimeState{Actors: []corebattle.LifetimeActor{{Participant: corebattle.Participant{ID: "self", TeamID: "party", HP: 1000, MP: 0}}}}
	c := corebattle.ArithmeticContext{TargetContext: corebattle.TargetContext{ActorID: "self", PartyIDs: []string{"self"}}}
	for _, job := range []string{"job-31", "job-72"} {
		for _, entry := range corebattle.ArithmeticEntries(job) {
			definitions.ArithmeticSkills[entry.ID] = true
			got, err := definitions.ArithmeticAction(&s, &c, entry.ID, "self", noArithmeticDraws{t})
			if err != nil || got != entry {
				t.Fatal(entry, got, err)
			}
			if _, err := definitions.JobAction(Definition{ID: entry.ID}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
				t.Fatal(entry, err)
			}
		}
	}
	if _, err := definitions.JobAction(Definition{ID: "random"}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
		t.Fatal(err)
	}
	if _, err := definitions.ArithmeticAction(nil, &c, "job-31-gravity", "", noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrInvalidRequest) {
		t.Fatal(err)
	}
}

type noArithmeticDraws struct{ t *testing.T }

func (r noArithmeticDraws) Float64() float64 { r.t.Fatal("admission consumed RNG"); return 0 }
func (r noArithmeticDraws) Intn(int) int     { r.t.Fatal("admission consumed RNG"); return 0 }
func (r noArithmeticDraws) IntN(n int) int   { return r.Intn(n) }
