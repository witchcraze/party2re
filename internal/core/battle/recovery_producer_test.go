package battle

import (
	"errors"
	"reflect"
	"testing"
)

func TestFiniteRecoveryStatProducers(t *testing.T) {
	for _, tc := range []struct {
		id     int
		target string
		stat   BattleStat
		want   int
		party  bool
	}{
		{19, "ally", StatHP, 190, false}, {20, "self", StatHP, 370, false}, {21, "ally", StatHP, 370, false},
		{22, "self", StatHP, 280, true}, {23, "self", StatHP, 640, true},
		{28, "self", StatAttack, 250, false}, {29, "ally", StatAttack, 250, false},
		{30, "self", StatDefense, 250, false}, {31, "ally", StatDefense, 250, false},
		{32, "self", StatAgility, 250, false}, {33, "ally", StatAgility, 250, false},
		{93, "enemy", StatAttack, 113, true}, {94, "enemy", StatDefense, 113, true},
		{95, "enemy", StatAgility, 113, true}, {96, "enemy", StatHit, 76, true},
	} {
		s, c := recoveryFixture()
		rng := &targetRNG{t: t}
		if tc.id <= 23 {
			rng.draws = []float64{0}
			if tc.party {
				rng.draws = []float64{0, 0, 0}
			}
		}
		r, err := s.ApplyRecoveryStatGem(tc.id, &c, "ally", rng)
		if err != nil || *s.actor(tc.target).currentStat(tc.stat) != tc.want || len(r) == 0 {
			t.Fatal(tc, r, err, s)
		}
		if tc.id >= 93 && (len(r) != 2 || r[1].Returned) {
			t.Fatal(tc, r)
		}
		rng.done()
	}
	// Mixed damage/drain/random descendants cannot acquire support from a kind.
	for _, id := range []int{18, 24, 26, 27, 34, 86, 97, 101, 102} {
		s, c := recoveryFixture()
		before := reloadLifetime(t, s)
		_, err := s.ApplyRecoveryStatGem(id, &c, "ally", &targetRNG{t: t})
		if !errors.Is(err, ErrUnsupportedEffect) || !reflect.DeepEqual(before, s) {
			t.Fatal(id, err, s)
		}
	}
}

func TestDistributionCursorMaximumAndTails(t *testing.T) {
	for _, tc := range []struct {
		source, cursor string
		over           bool
		cap            int
	}{
		{"", "", false, 999}, {"item150", "enemy", false, 999}, {"item159", "enemy", true, 1998},
	} {
		s, c := recoveryFixture()
		s.actor("fallen").Attack = 2500
		s.actor("self").Attack = 2500 // First tied maximum is self, even with fallen tie.
		s.actor("ally").Attack = 100
		s.actor("self").OverLevel, s.actor("ally").OverLevel = true, true
		s.actor("enemy").OverLevel = tc.over
		rng := &targetRNG{t: t, draws: []float64{.25, .25, .25, .25}}
		r, err := s.ApplyDistributedAttack(&c, "ally", TargetCursor{Source: tc.source, ActorID: tc.cursor}, rng)
		if err != nil || r.HolderID != "self" || r.Cap != tc.cap || s.actor("ally").Attack != tc.cap || s.actor("self").Attack != 2500 || !reflect.DeepEqual(r.RaisedIDs, []string{"ally"}) {
			t.Fatal(tc, r, err, s)
		}
		rng.done()
	}
	// Fallen sole maximum; a living recipient already ABOVE cap is untouched.
	s, c := recoveryFixture()
	s.actor("fallen").Attack = 2500
	s.actor("ally").Attack = 1200
	r, err := s.ApplyDistributedAttack(&c, "ally", TargetCursor{}, &targetRNG{t: t, draws: []float64{.5, .5, .5, .5}})
	if err != nil || r.HolderID != "fallen" || s.actor("self").Attack != 999 || s.actor("ally").Attack != 1200 {
		t.Fatal(r, err, s)
	}
	// Four independent tail tests, with heal draws interleaved in party order.
	s, c = recoveryFixture()
	s.actor("self").Tension = 3
	rng := &targetRNG{t: t, draws: []float64{0, 0, 0, 0, 0, 0, 0}}
	r, err = s.ApplyDistributedAttack(&c, "ally", TargetCursor{}, rng)
	if err != nil || len(r.Tails) != 12 || c.PartyStatUpCalls != 3 || c.PartyHealCalls != 1 || s.actor("self").Attack != 250 || s.actor("ally").Attack != 200 || s.actor("self").Defense != 250 || s.actor("ally").Defense != 200 || s.actor("self").HP != 460 || s.actor("ally").HP != 280 {
		t.Fatal(r, err, s, c)
	}
	rng.done()
	// Independent skip/apply decisions; only DF and healing tails run.
	s, c = recoveryFixture()
	rng = &targetRNG{t: t, draws: []float64{.25, 0, .25, 0, 0, 0, 0}}
	r, err = s.ApplyDistributedAttack(&c, "ally", TargetCursor{}, rng)
	if err != nil || len(r.Tails) != 6 || c.PartyStatUpCalls != 1 || c.PartyHealCalls != 1 || s.actor("self").Attack != 150 || s.actor("self").Defense != 250 || s.actor("self").Agility != 150 {
		t.Fatal(r, err, s, c)
	}
	rng.done()
}

func TestDistributionGuardsAndWholePreflight(t *testing.T) {
	for _, helper := range []string{"", "fallen"} {
		s, c := recoveryFixture()
		before := reloadLifetime(t, s)
		r, err := s.ApplyDistributedAttack(&c, helper, TargetCursor{}, &targetRNG{t: t})
		if err != nil || r.HolderID != "" || !reflect.DeepEqual(s, before) {
			t.Fatal(r, err, s)
		}
	}
	for _, cursor := range []TargetCursor{{ActorID: "ally"}, {Source: "submitted", ActorID: "ally"}, {Source: "item159", ActorID: "missing"}} {
		s, c := recoveryFixture()
		before := reloadLifetime(t, s)
		_, err := s.ApplyDistributedAttack(&c, "ally", cursor, &targetRNG{t: t})
		if !errors.Is(err, ErrUnsupportedEffect) || !reflect.DeepEqual(s, before) {
			t.Fatal(cursor, err, s)
		}
	}
	s, c := recoveryFixture()
	s.actor("self").Status = StatusDanceSeal
	r, err := s.ApplyDistributedAttack(&c, "ally", TargetCursor{}, &targetRNG{t: t, draws: []float64{.25}})
	if err != nil || r.HolderID != "" || s.actor("self").Status != StatusDanceSeal {
		t.Fatal(r, err, s)
	}
	// Unsupported AG child prevents even the outer dance-cure draw and AT write.
	s.actor("ally").ItemDefinitionIDs = []string{"item-201"}
	before := reloadLifetime(t, s)
	_, err = s.ApplyDistributedAttack(&c, "ally", TargetCursor{}, &targetRNG{t: t})
	if !errors.Is(err, ErrUnsupportedEffect) || !reflect.DeepEqual(s, before) {
		t.Fatal(err, s)
	}
	// No positive maximum is not a failure: tails still draw.
	s, c = recoveryFixture()
	for _, id := range c.PartyIDs {
		s.actor(id).Attack = 0
	}
	r, err = s.ApplyDistributedAttack(&c, "ally", TargetCursor{}, &targetRNG{t: t, draws: []float64{.5, .5, .5, .5}})
	if err != nil || r.HolderID != "" || len(r.RaisedIDs) != 0 {
		t.Fatal(r, err, s)
	}
}

func TestRecoverySerializedContinuation(t *testing.T) {
	s, c := recoveryFixture()
	s.actor("self").ItemDefinitionIDs = []string{"item-256"}
	s.actor("ally").ItemDefinitionIDs = []string{"item-205"}
	level := 99
	c.RequesterLevel = &level
	var completed []LifetimeState
	var results [][]EffectApplication
	for _, reload := range []bool{false, true} {
		state, op := reloadLifetime(t, s), c
		rng := &targetRNG{t: t, draws: []float64{.5, 0, 0, 0}}
		var all []EffectApplication
		for _, id := range []int{19, 31, 22} {
			op = c // Each submitted operation starts fresh helper counters.
			r, err := state.ApplyRecoveryStatGem(id, &op, "ally", rng)
			if err != nil {
				t.Fatal(err)
			}
			all = append(all, r...)
			if reload {
				state = reloadLifetime(t, state)
			}
		}
		rng.done()
		completed = append(completed, state)
		results = append(results, all)
	}
	if !reflect.DeepEqual(completed[0], completed[1]) || !reflect.DeepEqual(results[0], results[1]) {
		t.Fatal(completed, results)
	}
}
