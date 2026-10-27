package skill

import (
	"encoding/json"
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func TestBaseGuardReloadContinuesFrozenFactsWithoutReplay(t *testing.T) {
	s, c, d := baseGuardFixture()
	level := 99
	c.RequesterLevel, c.Command = &level, "frozen-base-guard"
	executeBaseGuard(t, &s, c, d, "job-1-entry-1", []string{"enemy"}, 0)
	executeBaseGuard(t, &s, c, d, "job-1-entry-2", []string{"fallen"})
	type checkpoint struct {
		State                             corebattle.LifetimeState
		Target                            corebattle.TargetContext
		Cursor                            corebattle.TargetCursor
		End                               corebattle.EndProcessing
		DamageCalls, StatDownCalls, Level int
		AdditionalEffect                  bool
		Arguments                         []string
		EntryID                           string
	}
	frozen := checkpoint{s, c.TargetContext, c.Cursor, c.End, c.DamageCalls, c.StatDownCalls, level, c.AdditionalEffect, []string{"enemy", "tail"}, "job-2-entry-6"}
	data, err := json.Marshal(frozen)
	if err != nil {
		t.Fatal(err)
	}
	var loaded checkpoint
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatal(err)
	}
	resumed := &corebattle.BaseGuardContext{DamageContext: corebattle.DamageContext{
		EffectContext: corebattle.EffectContext{TargetContext: loaded.Target, DamageCalls: loaded.DamageCalls, StatDownCalls: loaded.StatDownCalls, RequesterLevel: &loaded.Level, AdditionalEffect: loaded.AdditionalEffect},
		Cursor:        loaded.Cursor, End: loaded.End, GuaranteedHit: true, Defeat: func(string) error { return nil },
	}}
	var rebuilt BattleDefinitions
	rebuilt.RegisterBaseGuard(resumed)
	for _, run := range []struct {
		s *corebattle.LifetimeState
		c *corebattle.BaseGuardContext
		d BattleDefinitions
	}{{&s, c, d}, {&loaded.State, resumed, rebuilt}} {
		executeBaseGuard(t, run.s, run.c, run.d, loaded.EntryID, loaded.Arguments, 0, 0)
	}
	// First18 damage lowers DF80→68; the two later hits each floor((132/2−68×.3)×.9)=41.
	if !reflect.DeepEqual(s, loaded.State) || s.Actors[1].HP != 900 || c.DamageCalls != 3 || resumed.DamageCalls != 3 || resumed.StatDownCalls != 1 || resumed.Command != "frozen-base-guard" || *resumed.RequesterLevel != 99 || !reflect.DeepEqual(c.Arguments, []string{"tail"}) || !reflect.DeepEqual(c.Arguments, resumed.Arguments) || s.Actors[0].Temporary != corebattle.TemporaryCovering || s.Actors[2].Temporary != corebattle.TemporaryCover {
		t.Fatal(s, loaded, c, resumed)
	}
}
