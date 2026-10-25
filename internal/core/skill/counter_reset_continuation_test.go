package skill

import (
	"encoding/json"
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func TestCounterResetReloadContinuesWithoutReplay(t *testing.T) {
	s, c, d := counterResetFixture()
	level := 99
	c.RequesterLevel, c.Command, c.Custom = &level, "frozen-counter-command", true
	c.Cursor = corebattle.TargetCursor{ActorID: "self", Source: "item159"}
	executeCounterReset(t, &s, c, d, "job-35-entry-6", []string{"ignored"}, 0, 0)
	type checkpoint struct {
		State                                                                      corebattle.LifetimeState
		Target                                                                     corebattle.TargetContext
		DamageCalls, AreaDamageCalls, StatusCalls, AreaStatusCalls, RequesterLevel int
		Cursor                                                                     corebattle.TargetCursor
		Arguments                                                                  []string
		EntryID                                                                    string
	}
	frozen := checkpoint{s, c.TargetContext, c.DamageCalls, c.AreaDamageCalls, c.StatusCalls, c.AreaStatusCalls, level, c.Cursor, []string{"enemy", "tail"}, "job-37-entry-7"}
	data, err := json.Marshal(frozen)
	if err != nil {
		t.Fatal(err)
	}
	var loaded checkpoint
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatal(err)
	}
	resumed := &corebattle.CounterResetContext{DamageContext: corebattle.DamageContext{
		EffectContext: corebattle.EffectContext{TargetContext: loaded.Target, DamageCalls: loaded.DamageCalls, AreaDamageCalls: loaded.AreaDamageCalls, RequesterLevel: &loaded.RequesterLevel, AdditionalEffect: true},
		Cursor:        loaded.Cursor, GuaranteedHit: true, Defeat: func(string) error { return nil },
	}, StatusCalls: loaded.StatusCalls, AreaStatusCalls: loaded.AreaStatusCalls}
	var rebuilt BattleDefinitions
	rebuilt.RegisterCounterReset(resumed)
	for _, run := range []struct {
		s *corebattle.LifetimeState
		c *corebattle.CounterResetContext
		d BattleDefinitions
	}{{&s, c, d}, {&loaded.State, resumed, rebuilt}} {
		executeCounterReset(t, run.s, run.c, run.d, loaded.EntryID, loaded.Arguments)
	}
	if !reflect.DeepEqual(s, loaded.State) || s.Actors[1].HP != 765 || s.Actors[1].Status != corebattle.StatusParalyze || s.Actors[1].Attack != 50 || s.Actors[1].Defense != 40 || resumed.DamageCalls != 1 || resumed.StatusCalls != 1 || resumed.AreaStatusCalls != 1 || resumed.AreaDamageCalls != 1 || !reflect.DeepEqual(c.Arguments, resumed.Arguments) || !reflect.DeepEqual(c.Cursor, resumed.Cursor) || resumed.Command != "frozen-counter-command" || !resumed.Custom || *resumed.RequesterLevel != 99 {
		t.Fatal(s, loaded.State, c, resumed)
	}
}

func TestCounterResetOrderedDamageChangesLaterEligibility(t *testing.T) {
	// Current-HP immunity ends at999 after the damage; the status must apply.
	s, c, d := counterResetFixture()
	s.Actors[1].HP, s.Actors[1].MaxHP = 1000, 1000
	executeCounterReset(t, &s, c, d, "job-35-entry-6", nil, 0, 0)
	if s.Actors[1].HP != 865 || s.Actors[1].Status != corebattle.StatusParalyze {
		t.Fatal(s)
	}
	// Reflection can defeat the caster: damage stops but the status loop still
	// reaches the other enemy. No broad living guard belongs around the body.
	s, c, d = counterResetFixture()
	other := s.Actors[1]
	other.ID = "other"
	s.Actors = append(s.Actors, other)
	c.EnemyIDs = append(c.EnemyIDs, "other")
	s.Actors[0].HP = 1
	s.Actors[1].Temporary = corebattle.TemporaryReflectMagic
	executeCounterReset(t, &s, c, d, "job-35-entry-6", nil, 0, 0)
	if s.Actors[0].HP != 0 || s.Actors[3].HP != 900 || s.Actors[3].Status != corebattle.StatusParalyze || c.DamageCalls != 1 || c.StatusCalls != 1 {
		t.Fatal(s, c)
	}
	// A supplied defeat owner may revive the caster and let damage continue.
	// The real reset clears its consumed temporary before the later status.
	s, c, d = counterResetFixture()
	other = s.Actors[1]
	other.ID = "other"
	s.Actors = append(s.Actors, other)
	c.EnemyIDs = append(c.EnemyIDs, "other")
	s.Actors[0].HP, s.Actors[0].Temporary = 1, corebattle.TemporaryRevival
	s.Actors[1].Temporary = corebattle.TemporaryReflectMagic
	rng := &damageGemDraws{t: t, draws: []float64{0, 0, 0, 0, 0}}
	c.Defeat = func(id string) error {
		c.StatusDefeat.TargetContext = c.TargetContext
		_, err := s.ApplyDefeat(c.StatusDefeat, id, rng)
		return err
	}
	f := corebattle.CallbackContext{TargetContext: c.TargetContext}
	ops, err := d.InvocationAction(&s, &f, []string{"job-35-entry-6"}, noArithmeticDraws{t})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ApplyInvocations(&f, d.Invocations, ops, rng); err != nil {
		t.Fatal(err)
	}
	if rng.used != 5 || s.Actors[0].HP != 100 || s.Actors[0].Temporary != "" || s.Actors[0].Status != corebattle.StatusParalyze || s.Actors[3].HP != 786 || s.Actors[3].Status != corebattle.StatusParalyze || c.DamageCalls != 2 || c.StatusCalls != 2 {
		t.Fatal(s, c, rng.used)
	}
}
