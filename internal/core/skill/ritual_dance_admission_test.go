package skill

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/core/random"
)

func TestRitualDanceRegistrationAndWholeActionRejection(t *testing.T) {
	s, c, d := ritualDanceFixture()
	f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy", "tail"}}
	if len(d.Invocations.Entries) != 11 || len(d.Invocations.JobTables[75].Variants[0]) != 7 || len(d.Invocations.JobTables[76].Variants[0]) != 6 {
		t.Fatal(d)
	}
	if _, err := (BattleDefinitions{}).InvocationAction(&s, &f, []string{"job-75-entry-1"}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
		t.Fatal(err)
	}
	// Tool84 must admit all positions, including each separately owned revival.
	for _, job := range []string{"job-75", "job-76"} {
		s.Actors[0].Jobs.JobID = job
		d.Invocations.Entries["tool"] = corebattle.CallbackDefinition{Invocation: corebattle.CallbackInvocation{Kind: "tool-84"}}
		if _, err := d.InvocationAction(&s, &f, []string{"tool"}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
			t.Fatal(job, err)
		}
	}
	for _, id := range []string{"job-76-entry-2", "job-76-entry-5"} {
		s, c, d := ritualDanceFixture()
		s.Fields.Default = "cosmic"
		f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy", "tail"}}
		c.Defeat = func(string) error { t.Fatal("preflight ran external hook"); return nil }
		before, err := json.Marshal(s)
		if err != nil {
			t.Fatal(err)
		}
		gear, args := s.Actors[1].Gear, f.Arguments
		ops := []corebattle.CallbackInvocation{{Kind: "entry", EntryID: "job-75-entry-4"}, {Kind: "entry", EntryID: id}}
		if err := s.ApplyInvocations(&f, d.Invocations, ops, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
			t.Fatal(id, err)
		}
		after, err := json.Marshal(s)
		if err != nil {
			t.Fatal(err)
		}
		if string(before) != string(after) || c.DamageCalls != 0 || c.MPHealCalls != 0 || c.ActorID != "self" || s.Actors[1].Gear != gear || !reflect.DeepEqual(args, []string{"enemy", "tail"}) {
			t.Fatal(id, s, c, f)
		}
	}
	// An opaque prefix cannot hide now-unreachable descendants.
	d.Invocations.Entries["opaque"] = corebattle.CallbackDefinition{
		Validate: func(*corebattle.LifetimeState, *corebattle.CallbackContext) error { return nil },
		Execute:  func(*corebattle.LifetimeState, *corebattle.CallbackContext, random.Generator) error { return nil },
	}
	if _, err := d.InvocationAction(&s, &f, []string{"opaque", "job-75-entry-2"}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
		t.Fatal(err)
	}
}

func TestRitualDanceAcceptedNoEffectAndGuardBoundaries(t *testing.T) {
	for _, id := range []string{"job-75-entry-0", "job-75-entry-1", "job-75-entry-2", "job-75-entry-3", "job-75-entry-6", "job-76-entry-0", "job-76-entry-1", "job-76-entry-2", "job-76-entry-5"} {
		s, c, d := ritualDanceFixture()
		s.Actors[1].HP, s.Actors[0].Attack = 0, 0
		c.Defeat, s.Actors[1].Gear = nil, nil
		draws := []float64{}
		switch id {
		case "job-75-entry-0", "job-76-entry-2":
			draws = []float64{0}
		case "job-76-entry-5":
			draws = []float64{0, 0}
		}
		target := "enemy"
		if id == "job-75-entry-2" || id == "job-75-entry-3" {
			target = "self"
		}
		executeRitualDance(t, &s, c, d, id, []string{target}, draws...)
		if s.Actors[1].HP != 0 || s.Actors[0].HP != 500 || c.DamageCalls != 0 {
			t.Fatal(id, s, c)
		}
	}
	// No temporary means no proportional HP loss, even when the target lives.
	s, c, d := ritualDanceFixture()
	executeRitualDance(t, &s, c, d, "job-75-entry-1", []string{"enemy"})
	if s.Actors[1].HP != 900 {
		t.Fatal(s)
	}
	// One-hit defense returns an empty target, so drain must omit healing.
	s, c, d = ritualDanceFixture()
	s.Actors[1].Temporary = corebattle.TemporaryOneHitDefense
	c.Defeat = nil
	executeRitualDance(t, &s, c, d, "job-76-entry-2", []string{"enemy"}, 0)
	if s.Actors[0].HP != 500 || c.HealCalls != 0 || s.Actors[1].Temporary != "" {
		t.Fatal(s, c)
	}
	// Guarded magic status/accuracy paths retain the original no-effect.
	for _, id := range []string{"job-75-entry-3", "job-75-entry-6", "job-76-entry-0"} {
		s, c, d := ritualDanceFixture()
		s.Actors[0].Status = corebattle.StatusMagicSeal
		executeRitualDance(t, &s, c, d, id, []string{"self"}, .25)
		if s.Actors[0].Status != corebattle.StatusMagicSeal || s.Actors[0].HP != 500 {
			t.Fatal(id, s)
		}
	}
	// Sleep-song does not share the magic seal category.
	s, c, d = ritualDanceFixture()
	s.Actors[0].Status = corebattle.StatusMagicSeal
	executeRitualDance(t, &s, c, d, "job-76-entry-1", nil, 0)
	if s.Actors[1].Status != corebattle.StatusSleep {
		t.Fatal(s)
	}
	// No enemy roster still retains each living ally's dance branch draw.
	s, c, d = ritualDanceFixture()
	s.Actors[1].TeamID = "party"
	c.EnemyIDs, c.PartyIDs = nil, []string{"self", "enemy", "fallen"}
	c.Defeat = nil
	executeRitualDance(t, &s, c, d, "job-76-entry-5", nil, 0, .25)
	if c.DamageCalls != 0 || s.Actors[0].HP != 500 || s.Actors[1].HP != 900 {
		t.Fatal(s, c)
	}
}

func TestRitualDanceGroupOrderedAdmissionAndActorRestoration(t *testing.T) {
	// The first ally consumes defense; a later ally needs the complete tail.
	s, c, d := ritualDanceFixture()
	s.Actors[2].HP, s.Actors[2].Attack = 100, 100
	s.Actors[1].Temporary, s.Fields.Default = corebattle.TemporaryOneHitDefense, "cosmic"
	f := corebattle.CallbackContext{TargetContext: c.TargetContext}
	if _, err := d.InvocationAction(&s, &f, []string{"job-76-entry-5"}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) || s.Actors[1].Temporary != corebattle.TemporaryOneHitDefense || c.ActorID != "self" {
		t.Fatal(err, s, c)
	}
	// The shield-only single living actor remains a supported no-effect.
	s.Actors[2].HP = 0
	executeRitualDance(t, &s, c, d, "job-76-entry-5", nil, 0, 0)
	if s.Actors[1].Temporary != "" || s.Actors[1].HP != 900 {
		t.Fatal(s)
	}
	// A confused first hit kills a later party member before its loop guard.
	s, c, d = ritualDanceFixture()
	s.Actors[2].HP, s.Actors[2].Attack = 1, 100
	s.Actors[0].Status = corebattle.StatusConfusion
	executeRitualDance(t, &s, c, d, "job-76-entry-5", nil, 0, .9, .9, 0)
	if c.DamageCalls != 1 || s.Actors[2].HP != 0 {
		t.Fatal(s, c)
	}
	// Restore temporary actor/tension even when an admitted external hook fails.
	s, c, d = ritualDanceFixture()
	s.Actors[0].Attack = 0
	s.Actors[2].HP, s.Actors[2].Attack, s.Actors[2].Tension, s.Actors[2].NPC = 100, 100, 5, true
	s.Actors[1].HP = 1
	failure := errors.New("defeat unavailable")
	c.Defeat = func(string) error { return failure }
	f = corebattle.CallbackContext{TargetContext: c.TargetContext}
	err := s.ApplyInvocations(&f, d.Invocations, []corebattle.CallbackInvocation{{Kind: "entry", EntryID: "job-76-entry-5"}}, &damageGemDraws{t: t, draws: []float64{0, 0, 0, 0, 0}})
	if !errors.Is(err, failure) || c.ActorID != "self" || c.ActorNPC || s.Actors[2].Tension != 5 || f.ActorID != "self" {
		t.Fatal(err, s, c, f)
	}
}

func TestRitualDanceGroupAdmitsTargetsAfterCoverDefeat(t *testing.T) {
	for _, missing := range []string{"protected", "next-coverer", "next-coverer-supported", ""} {
		s, c, d := ritualDanceFixture()
		s.Actors[2].HP, s.Actors[2].Attack = 100, 100
		s.Actors[1].Temporary = corebattle.TemporaryCover
		cover := s.Actors[1]
		cover.ID, cover.HP, cover.Temporary = "cover", 1, corebattle.TemporaryCovering
		s.Actors = append(s.Actors, cover)
		c.EnemyIDs = append(c.EnemyIDs, cover.ID)
		if missing == "protected" {
			s.Actors[1].Gear = nil
		}
		if missing == "next-coverer" || missing == "next-coverer-supported" {
			cover.ID = "next-cover"
			if missing == "next-coverer" {
				cover.Gear = nil
			} else {
				cover.HP = 900
				s.Actors[1].Gear = nil
			}
			s.Actors = append(s.Actors, cover)
			c.EnemyIDs = append(c.EnemyIDs, cover.ID)
		}
		calls := 0
		c.Defeat = func(id string) error {
			calls++
			for i := range s.Actors {
				if s.Actors[i].ID == id {
					s.Actors[i].Temporary = ""
				}
			}
			return nil
		}
		f := corebattle.CallbackContext{TargetContext: c.TargetContext}
		_, err := d.InvocationAction(&s, &f, []string{"job-76-entry-5"}, noArithmeticDraws{t})
		if missing == "protected" || missing == "next-coverer" {
			if !errors.Is(err, corebattle.ErrUnsupportedEffect) || calls != 0 || s.Actors[3].HP != 1 || s.Actors[3].Temporary != corebattle.TemporaryCovering || c.ActorID != "self" {
				t.Fatal(missing, err, calls, s, c)
			}
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		executeRitualDance(t, &s, c, d, "job-76-entry-5", nil, 0, 0, 0, 0, 0, 0)
		wantHP := 810
		if missing == "next-coverer-supported" {
			wantHP = 900
			if s.Actors[4].HP != 810 {
				t.Fatal(s)
			}
		}
		if s.Actors[1].HP != wantHP || s.Actors[3].HP != 0 || c.DamageCalls != 2 || calls != 1 {
			t.Fatal(s, c, calls)
		}
	}
}
