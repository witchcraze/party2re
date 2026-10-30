package skill

import (
	"encoding/json"
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func TestRecoilDarkReloadWithoutReplay(t *testing.T) {
	for _, revived := range []bool{false, true} {
		s, c, d := recoilDarkFixture()
		c.Command = "frozen-command"
		level := 99
		c.RequesterLevel = &level
		draws := []float64{0, .75, 0}
		want := 563
		if revived {
			s.Actors[0].HP, s.Actors[0].Temporary = 1, corebattle.TemporaryRevival
			c.RawDefeat = rawDefeatContext()
			draws, want = append(draws, 0), 468
		}
		executeRecoilDark(t, &s, c, d, []string{"job-21-entry-5"}, nil, draws...)
		type checkpoint struct {
			State                                  corebattle.LifetimeState
			Target                                 corebattle.TargetContext
			End                                    corebattle.EndProcessing
			Cursor                                 corebattle.TargetCursor
			RequesterLevel, DamageCalls, HealCalls int
			Arguments                              []string
			EntryID                                string
		}
		frozen := checkpoint{s, c.TargetContext, c.End, c.Cursor, level, c.DamageCalls, c.HealCalls, []string{"enemy", "tail"}, "job-38-entry-0"}
		data, err := json.Marshal(frozen)
		if err != nil {
			t.Fatal(err)
		}
		var loaded checkpoint
		if err := json.Unmarshal(data, &loaded); err != nil {
			t.Fatal(err)
		}
		resumed := &corebattle.RecoilDarkStateContext{DamageContext: corebattle.DamageContext{
			EffectContext: corebattle.EffectContext{TargetContext: loaded.Target, RequesterLevel: &loaded.RequesterLevel, DamageCalls: loaded.DamageCalls, HealCalls: loaded.HealCalls},
			End:           loaded.End, Cursor: loaded.Cursor, GuaranteedHit: true, Defeat: func(string) error { return nil },
		}}
		rebuilt := BattleDefinitions{}
		rebuilt.RegisterRecoilDarkState(resumed)
		for _, run := range []struct {
			s *corebattle.LifetimeState
			c *corebattle.RecoilDarkStateContext
			d BattleDefinitions
		}{{&s, c, d}, {&loaded.State, resumed, rebuilt}} {
			executeRecoilDark(t, run.s, run.c, run.d, []string{loaded.EntryID}, loaded.Arguments, 0, 0)
		}
		if !reflect.DeepEqual(s, loaded.State) || s.Actors[0].HP != want || c.DamageCalls != 1 || resumed.DamageCalls != 1 || c.HealCalls != 1 || resumed.HealCalls != 1 || !reflect.DeepEqual(c.Arguments, resumed.Arguments) || resumed.Command != "frozen-command" || *resumed.RequesterLevel != 99 {
			t.Fatal(revived, s, loaded.State, c, resumed)
		}
	}
}

func TestRecoilDarkDamageDefeatBeforeTail(t *testing.T) {
	for _, revive := range []bool{false, true} {
		s, c, d := recoilDarkFixture()
		s.Actors[0].HP = 1
		s.Actors[1].Temporary = corebattle.TemporaryReflectMagic
		defeat := rawDefeatContext()
		if revive {
			s.Actors[0].Temporary = corebattle.TemporaryRevival
		}
		draws := []float64{0}
		if revive {
			draws = append(draws, 0, 0)
		}
		rng := &damageGemDraws{t: t, draws: draws}
		c.Defeat = func(id string) error {
			defeat.TargetContext = c.TargetContext
			_, err := s.ApplyDefeat(defeat, id, rng)
			return err
		}
		frame := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy"}}
		ops, err := d.InvocationAction(&s, &frame, []string{"job-38-entry-0"}, noArithmeticDraws{t})
		if err != nil {
			t.Fatal(err)
		}
		if err := s.ApplyInvocations(&frame, d.Invocations, ops, rng); err != nil {
			t.Fatal(err)
		}
		if rng.used != len(draws) {
			t.Fatal(rng.used)
		}
		want, heals := 0, 0
		if revive {
			want, heals = 508, 1
		}
		if s.Actors[0].HP != want || c.HealCalls != heals || s.Actors[1].HP != 900 {
			t.Fatal(revive, s, c)
		}
	}
	// Reckless strike's direct tail still writes after caster defeat; the outer
	// custom invocation then stops, so the later stance is never executed.
	s, c, d := recoilDarkFixture()
	s.Actors[0].HP, s.Actors[1].Temporary = 1, corebattle.TemporaryReflectAttack
	executeRecoilDark(t, &s, c, d, []string{"job-21-entry-3", "job-21-entry-1"}, []string{"enemy"}, 0)
	if s.Actors[0].HP != 0 || s.Actors[0].Temporary != corebattle.TemporaryDouble {
		t.Fatal(s)
	}
}

func TestRecoilDarkMPDrainKeepsCallbackArguments(t *testing.T) {
	s, c, d := recoilDarkFixture()
	c.Custom, s.Actors[0].Status = true, corebattle.StatusConfusion
	// An absent target remains absent for the job body: both enemy confusion
	// selections occur. A gem's custom fallback to self would skip both draws.
	executeRecoilDark(t, &s, c, d, []string{"job-38-entry-1"}, []string{"absent", "tail"}, .4, .4, 0, 0)
	if s.Actors[1].MP != 80 || s.Actors[0].MP != 99 || c.Arguments[0] != "tail" {
		t.Fatal(s, c)
	}
}
