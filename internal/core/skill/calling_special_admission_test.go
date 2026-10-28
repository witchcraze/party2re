package skill

import (
	"errors"
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func callingFairyRegistry() (corebattle.LifetimeState, corebattle.RosterContext, BattleDefinitions) {
	s, c, d := callingManifestFixture()
	addCallingManifestHelper(&s, &c, 80)
	s.Actors[3].SummonForm = 1
	s.Actors[3].Icon = "assets/images/placeholders/battle/helpers.svg#calling-80-1"
	d.RegisterCallingJobs(&c)
	// Synthetic unrelated tables certify only the invocation boundary under
	// test. Job80 keeps its complete source table and all six native bodies.
	for job := 1; job <= 87; job++ {
		if job != 80 {
			d.Invocations.JobTables[job] = corebattle.CallbackTable{Variants: [][]corebattle.CallbackEntry{{}}}
		}
	}
	return s, c, d
}

func TestCallingFairyConditionalRandomClosureAndCycles(t *testing.T) {
	s, c, d := callingFairyRegistry()
	f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy", "tail"}}
	before := s
	if _, err := d.InvocationAction(&s, &f, []string{"job-80-special"}, noArithmeticDraws{t}); err != nil {
		t.Fatal(err)
	}
	// General random -> the real80.3 body -> general random -> the real80.2
	// helper attack. Listed costs/SP are waived; arguments and actor restore.
	rng := &damageGemDraws{t: t, draws: []float64{.5, 79.0 / 87, .5, .5, 79.0 / 87, 2.0 / 6, .5, 0}}
	if err := s.ApplyInvocations(&f, d.Invocations, []corebattle.CallbackInvocation{{Kind: "entry", EntryID: "job-80-special"}}, rng); err != nil {
		t.Fatal(err)
	}
	if rng.used != len(rng.draws) || s.Actors[1].HP != 9955 || s.Actors[0].MP != 100 || c.ActorID != "self" || f.ActorID != "self" || !reflect.DeepEqual(f.Arguments, []string{"enemy", "tail"}) {
		t.Fatal(s, c, f, rng.used)
	}
	// An unsupported reachable table rejects even the healing branch, before
	// costs, RNG, arguments, counters or any preceding callback can change.
	s, c, d = callingFairyRegistry()
	f.TargetContext = c.TargetContext
	d.Invocations.JobTables[1] = corebattle.CallbackTable{Variants: [][]corebattle.CallbackEntry{{{ID: "missing"}}}}
	before = s
	ops := []corebattle.CallbackInvocation{{Kind: "entry", EntryID: "job-80-defend"}, {Kind: "entry", EntryID: "job-80-special"}}
	if err := s.ApplyInvocations(&f, d.Invocations, ops, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) || !reflect.DeepEqual(before, s) || c.PartyHealCalls != 0 {
		t.Fatal(err, s)
	}
	// Archer and absent/fallen-helper guards never require general random.
	for _, guard := range []string{"archer", "fallen", "missing"} {
		s, c, d = callingFairyRegistry()
		switch guard {
		case "archer":
			s.Actors[3].Icon = "assets/images/placeholders/battle/helpers.svg#calling-80-2"
		case "fallen":
			s.Actors[3].HP = 0
		case "missing":
			s.Actors[3].RosterIdentity = "other"
		}
		d.Invocations.JobTables = nil
		if err := d.RosterAction(&s, &c, "job-80-special", "enemy", noArithmeticDraws{t}); err != nil {
			t.Fatal(guard, err)
		}
	}
}

func TestCallingSpecialRegistrationArgumentConsumptionAndDefaultRejection(t *testing.T) {
	for _, id := range []string{"job-61-special", "job-62-special", "job-62-drain-barrage", "job-63-special", "job-64-special", "job-66-special", "job-67-special", "job-71-special", "job-80-special", "job-87-special"} {
		s, c, d := callingManifestFixture()
		f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy", "tail"}}
		if _, err := (BattleDefinitions{}).InvocationAction(&s, &f, []string{id}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
			t.Fatal(id, err)
		}
		d.RegisterCallingJobs(&c)
		// Distinguish a bound callback's source no-effect from default rejection.
		if err := s.ApplyInvocations(&f, d.Invocations, []corebattle.CallbackInvocation{{Kind: "entry", EntryID: id}}, noArithmeticDraws{t}); err != nil {
			t.Fatal(id, err)
		}
	}
	for _, row := range []struct {
		job     int
		draws   []float64
		consume bool
	}{
		{62, []float64{0, 0, 0}, true}, {63, []float64{0}, false},
		{64, []float64{.5, 0}, true}, {71, []float64{0, 0}, true},
		{80, []float64{0, 0, 0}, false}, {87, []float64{0, 0, 1.0 / 3}, true},
	} {
		s, c, d := callingManifestFixture()
		addCallingManifestHelper(&s, &c, row.job)
		d.RegisterCallingJobs(&c)
		f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy", "tail"}}
		id := corebattle.RosterEntries(row.job)[3].ID
		rng := &damageGemDraws{t: t, draws: row.draws}
		if err := d.Invocations.Entries[id].Execute(&s, &f, rng); err != nil {
			t.Fatal(id, err)
		}
		want := []string{"enemy", "tail"}
		if row.consume {
			want = want[1:]
		}
		if !reflect.DeepEqual(f.Arguments, want) || rng.used != len(rng.draws) {
			t.Fatal(id, f, rng.used)
		}
	}
}
