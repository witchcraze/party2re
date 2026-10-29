package skill

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func TestRitualDanceFrozenCostAndReloadWithoutReplay(t *testing.T) {
	s, c, d := ritualDanceFixture()
	frozenSize := len(c.PartyIDs)
	executeRitualDance(t, &s, c, d, "job-75-entry-4", nil, 0, 0)
	// The party changes after table construction; the listed cost stays24.
	late := s.Actors[2]
	late.ID, late.HP, late.MaxHP, late.Attack = "late", 100, 100, 200
	s.Actors = append(s.Actors, late)
	c.PartyIDs = append(c.PartyIDs, late.ID)
	type checkpoint struct {
		State                               corebattle.LifetimeState
		Target                              corebattle.TargetContext
		Arguments                           []string
		EntryID                             string
		PartySize, DamageCalls, MPHealCalls int
	}
	frozen := checkpoint{s, c.TargetContext, []string{"enemy", "tail"}, "job-76-entry-5", frozenSize, c.DamageCalls, c.MPHealCalls}
	data, err := json.Marshal(frozen)
	if err != nil {
		t.Fatal(err)
	}
	var loaded checkpoint
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatal(err)
	}
	_, resumed, _ := ritualDanceFixture()
	resumed.TargetContext, resumed.DamageCalls, resumed.MPHealCalls = loaded.Target, loaded.DamageCalls, loaded.MPHealCalls
	rebuilt := BattleDefinitions{}
	rebuilt.RegisterRitualDance(resumed, loaded.PartySize)
	for _, run := range []struct {
		s *corebattle.LifetimeState
		c *corebattle.RitualDanceContext
		d BattleDefinitions
	}{{&s, c, d}, {&loaded.State, resumed, rebuilt}} {
		entry := run.d.Invocations.JobTables[76].Variants[0][5]
		if entry.MPCost != 24 || len(run.c.PartyIDs) != 3 {
			t.Fatal(entry, run.c)
		}
		executeRitualDance(t, run.s, run.c, run.d, loaded.EntryID, loaded.Arguments, 0, 0, 0, .25, 0, 0)
	}
	if !reflect.DeepEqual(s, loaded.State) || s.Actors[0].HP != 230 || c.DamageCalls != 2 || resumed.DamageCalls != 2 || c.MPHealCalls != 1 || resumed.MPHealCalls != 1 || !reflect.DeepEqual(loaded.Arguments, []string{"enemy", "tail"}) {
		t.Fatal(s, loaded.State, c, resumed)
	}
}

func TestRitualDanceCompleteIndirectTablesAndResourceWrappers(t *testing.T) {
	for _, reverse := range []bool{false, true} {
		s, c, d := ritualDanceFixture()
		revival := &corebattle.RevivalContext{TargetContext: c.TargetContext, Defeat: c.Defeat}
		if reverse {
			d = BattleDefinitions{}
			d.RegisterRevivals(revival)
			d.RegisterRitualDance(c, 2)
		} else {
			d.RegisterRevivals(revival)
		}
		for _, job := range []string{"job-75", "job-76"} {
			s.Actors[0].Jobs.JobID, s.Actors[0].MP, s.Actors[0].Jobs.SP = job, 0, 0
			d.Invocations.Entries["tool"] = corebattle.CallbackDefinition{Invocation: corebattle.CallbackInvocation{Kind: "tool-84"}}
			f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"self"}}
			ops, err := d.InvocationAction(&s, &f, []string{"tool"}, noArithmeticDraws{t})
			if err != nil {
				t.Fatal(reverse, job, err)
			}
			draws := []float64{.01, 0, 0}
			if job == "job-76" {
				draws = []float64{.01}
			}
			if err := s.ApplyInvocations(&f, d.Invocations, ops, &damageGemDraws{t: t, draws: draws}); err != nil {
				t.Fatal(job, err)
			}
			if s.Actors[0].MP != 0 {
				t.Fatal(s)
			}
		}
	}
	// The real random wrapper waives listed costs and retains internal MP gains.
	// Other jobs are deliberately empty fixtures, not catalog certification.
	for _, slot := range []int{3, 4} {
		s, c, d := ritualDanceFixture()
		for job := 1; job <= 87; job++ {
			if job != 75 && job != 76 {
				d.Invocations.JobTables[job] = corebattle.CallbackTable{Variants: [][]corebattle.CallbackEntry{{}}}
			}
		}
		d.RegisterRevivals(&corebattle.RevivalContext{TargetContext: c.TargetContext, Defeat: c.Defeat})
		d.Invocations.Entries["random"] = corebattle.CallbackDefinition{Invocation: corebattle.CallbackInvocation{Kind: "random"}}
		s.Actors[0].MP, s.Actors[0].Jobs.SP = 0, 0
		args, draws, wantMP := []string{"self", "tail"}, []float64{74.1 / 87, (float64(slot) + .1) / 7}, 0
		if slot == 4 {
			draws = append(draws, 0, 0)
			wantMP = 102
		}
		executeRitualDance(t, &s, c, d, "random", args, draws...)
		if s.Actors[0].MP != wantMP || c.ActorID != "self" {
			t.Fatal(slot, s, c)
		}
		delete(d.Invocations.Entries, "job-76-entry-5")
		f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: args}
		if _, err := d.InvocationAction(&s, &f, []string{"random"}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
			t.Fatal(err)
		}
	}
}

func TestRitualDanceIndirectRestoresInternalMPDecrease(t *testing.T) {
	s, c, d := ritualDanceFixture()
	s.Actors[2].MaxHP = 101
	revival := &corebattle.RevivalContext{TargetContext: c.TargetContext, Defeat: func(string) error { s.Actors[0].HP = 0; return nil }}
	d.RegisterRevivals(revival)
	for job := 1; job <= 87; job++ {
		if job != 75 && job != 76 {
			d.Invocations.JobTables[job] = corebattle.CallbackTable{Variants: [][]corebattle.CallbackEntry{{}}}
		}
	}
	d.Invocations.Entries["random"] = corebattle.CallbackDefinition{Invocation: corebattle.CallbackInvocation{Kind: "random"}}
	executeRitualDance(t, &s, c, d, "random", nil, 75.1/87, 3.1/6)
	if s.Actors[0].HP != 0 || s.Actors[0].MP != 80 || s.Actors[2].HP != 101 {
		t.Fatal(s)
	}
}
