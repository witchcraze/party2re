package battle

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/witchcraze/party2re/internal/core/random"
)

func TestDoubleTailUsesIndependentDrawAndRejectsAbsentTuple(t *testing.T) {
	s, c := rosterFixture()
	addRosterHelper(t, &s, &c, 67)
	admitRosterDamage(&s, &c)
	rng := &targetRNG{t: t, draws: []float64{0, .5}}
	r, err := s.ApplyRoster(&c, "job-67-double-tail", "", rng)
	if err != nil || r.Effects[0].Amount != 135 || s.actor("enemy").Temporary != TemporaryDouble || c.AreaDamageCalls != 0 || !c.AdditionalEffect {
		t.Fatal(r, err, c)
	}
	rng.done()
	s.actor("enemy").Temporary = TemporaryOneHitDefense
	before, _ := json.Marshal(s)
	if _, err := s.ApplyRoster(&c, "job-67-double-tail", "", &targetRNG{t: t}); !errors.Is(err, ErrUnsupportedEffect) {
		t.Fatal(err)
	}
	after, _ := json.Marshal(s)
	if string(before) != string(after) {
		t.Fatal("absent tuple rejection mutated state")
	}
}

func TestVenomComposesDamageStatusAndGrowthInSourceOrder(t *testing.T) {
	s, c := rosterFixture()
	id := addRosterHelper(t, &s, &c, 87)
	admitRosterDamage(&s, &c)
	s.actor(id).SummonForm = 1
	s.actor("enemy").MaxHP = 999
	statuses := []string{}
	c.Status = &RosterStatusCallbacks{
		Validate: func(s *LifetimeState, frame TargetContext, e TargetEffect, chance float64) error {
			if frame.ActorID != "player" || e.TargetID != "enemy" || e.Category != CategoryBreath || chance != .75 {
				return ErrUnsupportedEffect
			}
			return nil
		},
		Execute: func(s *LifetimeState, frame TargetContext, e TargetEffect, chance float64, rng random.Generator) error {
			statuses = append(statuses, e.Payload.Symbol)
			// The separately certified owner is invoked after each successful
			// matching damage return. Its own status probability belongs there.
			wantHP := 892
			if len(statuses) == 2 {
				wantHP = 784
			}
			if s.actor("enemy").HP != wantHP || s.actor(id).SummonForm != 1 {
				t.Fatal("composition order", s.Actors)
			}
			return nil
		},
	}
	rng := &targetRNG{t: t, draws: []float64{0, 0, 0, 0, .9, 0, 0, 0, 0, 0, 0}}
	r, err := s.ApplyRoster(&c, "job-87-venom", "", rng)
	if err != nil || len(r.Effects) != 2 || len(statuses) != 2 || statuses[0] != StatusDeadlyPoison || statuses[1] != StatusParalyze || s.actor(id).SummonForm != 2 || c.AreaDamageCalls != 0 {
		t.Fatal(r, err, statuses, s.Actors)
	}
	rng.done()
}

func TestVenomBossThresholdSkipsStatusPortAndRedirectSkipsTail(t *testing.T) {
	for _, boss := range []bool{false, true} {
		s, c := rosterFixture()
		id := addRosterHelper(t, &s, &c, 87)
		s.actor(id).SummonForm = 2
		admitRosterDamage(&s, &c)
		if boss {
			s.actor("enemy").MaxHP = 1000
		} else {
			s.actor("enemy").MaxHP = 999
			s.actor("enemy").BaseStats.Defense = 1000
		}
		rng := &targetRNG{t: t, draws: []float64{.5, 0, 0}}
		r, err := s.ApplyRoster(&c, "job-87-venom", "", rng)
		if err != nil || len(r.Effects) != 1 || s.actor("enemy").Status != StatusPoison {
			t.Fatal(r, err, s.Actors)
		}
		rng.done()
	}
	s, c := rosterFixture()
	id := addRosterHelper(t, &s, &c, 87)
	s.actor(id).SummonForm = 2
	admitRosterDamage(&s, &c)
	s.actor("enemy").Temporary = TemporaryReflectBreath
	// Redirected hits never run either target-status tail, even at the threshold.
	rng := &targetRNG{t: t, draws: []float64{.5, 0}}
	r, err := s.ApplyRoster(&c, "job-87-venom", "", rng)
	if err != nil || len(r.Effects) != 1 || r.Effects[0].TargetID != "player" || s.actor("enemy").Status != "" {
		t.Fatal(r, err)
	}
	rng.done()
}

func TestVenomStatusPortRejectionPrecedesAllDamage(t *testing.T) {
	s, c := rosterFixture()
	addRosterHelper(t, &s, &c, 87)
	admitRosterDamage(&s, &c)
	s.actor("enemy").MaxHP = 999
	c.Status = &RosterStatusCallbacks{Validate: func(*LifetimeState, TargetContext, TargetEffect, float64) error { return ErrUnsupportedEffect }, Execute: func(*LifetimeState, TargetContext, TargetEffect, float64, random.Generator) error {
		t.Fatal("executed rejected status")
		return nil
	}}
	before, _ := json.Marshal(s)
	if _, err := s.ApplyRoster(&c, "job-87-venom", "", &targetRNG{t: t}); !errors.Is(err, ErrUnsupportedEffect) {
		t.Fatal(err)
	}
	after, _ := json.Marshal(s)
	if string(before) != string(after) || c.DamageCalls != 0 {
		t.Fatal("partial venom")
	}
}
