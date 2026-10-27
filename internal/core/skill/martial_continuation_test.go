package skill

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func TestMartialReloadContinuesFrozenInputWithoutReplay(t *testing.T) {
	s, c, d := martialFixture()
	executeMartial(t, &s, c, d, "job-4-entry-8", nil)
	executeMartial(t, &s, c, d, "job-25-entry-1", nil, 0)
	executeMartial(t, &s, c, d, "job-4-entry-3", []string{"enemy"}, 0)
	s.Actors[0].FinishExtended(corebattle.LifetimeTail{DamageCalls: 1})
	level := 99
	c.RequesterLevel, c.Command = &level, "frozen-martial-command"
	type checkpoint struct {
		State                                      corebattle.LifetimeState
		Target                                     corebattle.TargetContext
		Cursor                                     corebattle.TargetCursor
		End                                        corebattle.EndProcessing
		DamageCalls, HealCalls, StatUpCalls, Level int
		Arguments                                  []string
		EntryID                                    string
	}
	frozen := checkpoint{s, c.TargetContext, c.Cursor, c.End, c.DamageCalls, c.HealCalls, c.StatUpCalls, level, []string{"enemy", "enemy", "tail"}, "job-23-entry-3"}
	data, err := json.Marshal(frozen)
	if err != nil {
		t.Fatal(err)
	}
	var loaded checkpoint
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatal(err)
	}
	resumed := &corebattle.MartialContext{DamageContext: corebattle.DamageContext{EffectContext: corebattle.EffectContext{TargetContext: loaded.Target, DamageCalls: loaded.DamageCalls, HealCalls: loaded.HealCalls, StatUpCalls: loaded.StatUpCalls, RequesterLevel: &loaded.Level}, Cursor: loaded.Cursor, End: loaded.End, GuaranteedHit: true, Defeat: func(string) error { return nil }}}
	var rebuilt BattleDefinitions
	rebuilt.RegisterMartial(resumed)
	for _, run := range []struct {
		s *corebattle.LifetimeState
		c *corebattle.MartialContext
		d BattleDefinitions
	}{{&s, c, d}, {&loaded.State, resumed, rebuilt}} {
		executeMartial(t, run.s, run.c, run.d, loaded.EntryID, loaded.Arguments, 0, 0)
		// Six qualifying operations expire the flame stance, including the
		// operation already committed before this checkpoint.
		for range 5 {
			run.s.Actors[0].FinishExtended(corebattle.LifetimeTail{DamageCalls: 1})
		}
	}
	// Flame reduces physical power to90%; only the two AG-based hits qualify
	// for direct damage: floor151.2×.45 + 2×floor79.2×.9 = 210.
	if !reflect.DeepEqual(s, loaded.State) || s.Actors[0].HP != 590 || s.Actors[1].HP != 790 || s.Actors[0].Extended != "" || s.Actors[0].ExtendedTurns != 0 || resumed.DamageCalls != 3 || resumed.HealCalls != 1 || resumed.StatUpCalls != 2 || c.DamageCalls != 3 || !reflect.DeepEqual(resumed.Arguments, []string{"tail"}) || resumed.Command != "frozen-martial-command" || *resumed.RequesterLevel != 99 {
		t.Fatal(s, loaded.State, c, resumed)
	}
}

func TestMartialIndirectTablesAndExtendedCharge(t *testing.T) {
	for _, job := range []int{4, 23, 25} {
		s, c, d := martialFixture()
		s.Actors[0].MP, s.Actors[0].Jobs.SP = 0, 0
		s.Actors[0].Jobs.JobID = map[int]string{4: "job-04", 23: "job-23", 25: "job-25"}[job]
		s.Actors[1].HP = 1001 // Death immunity admits that position without defeat ports.
		d.Invocations.Entries["tool"] = corebattle.CallbackDefinition{Invocation: corebattle.CallbackInvocation{Kind: "tool-84"}}
		draws := []float64{.999}
		if job == 23 {
			draws = []float64{.999, 0}
		}
		executeMartial(t, &s, c, d, "tool", []string{"enemy", "tail"}, draws...)
		if s.Actors[0].MP != 0 || s.Actors[0].CMP != 90 {
			t.Fatal(s)
		}
		if job == 4 && s.Actors[0].Extended != corebattle.ExtendedFlame || job == 23 && s.Actors[1].HP != 875 || job == 25 && s.Actors[0].Temporary != corebattle.TemporaryRevival {
			t.Fatal(job, s)
		}
		last := d.Invocations.JobTables[job].Variants[0]
		delete(d.Invocations.Entries, last[len(last)-1].ID)
		frame := corebattle.CallbackContext{TargetContext: c.TargetContext}
		if _, err := d.InvocationAction(&s, &frame, []string{"tool"}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
			t.Fatal(err)
		}
		d.Invocations.Entries["random"] = corebattle.CallbackDefinition{Invocation: corebattle.CallbackInvocation{Kind: "random"}}
		if _, err := d.InvocationAction(&s, &frame, []string{"random"}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
			t.Fatal(err)
		}
	}
	for _, mp := range []int{9, 10} {
		s, c, d := martialFixture()
		s.Actors[0].Jobs = corebattle.BattleJobs{JobID: "job-04", SP: 100}
		s.Actors[0].MP = mp
		d.Invocations.Entries["extension"] = corebattle.CallbackDefinition{Invocation: corebattle.CallbackInvocation{Kind: "gem-86"}}
		executeMartial(t, &s, c, d, "extension", nil)
		wantMP := 9
		if mp == 10 {
			wantMP = 0
		}
		if s.Actors[0].MP != wantMP || (s.Actors[0].Extended == corebattle.ExtendedFlame) != (mp == 10) {
			t.Fatal(mp, s)
		}
	}
}
