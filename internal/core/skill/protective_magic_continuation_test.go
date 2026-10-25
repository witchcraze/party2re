package skill

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func TestProtectiveMagicReloadContinuesWithoutReplay(t *testing.T) {
	s, c, d := protectiveMagicFixture()
	level := 99
	c.RequesterLevel, c.Command, c.Custom = &level, "frozen-protective-command", true
	c.Cursor = corebattle.TargetCursor{ActorID: "self", Source: "item159"}
	s.Actors[0].Status = corebattle.StatusPoison
	executeProtectiveMagic(t, &s, c, d, "job-33-entry-7", nil)
	executeProtectiveMagic(t, &s, c, d, "job-34-entry-6", []string{"enemy", "tail"}, 0)
	for range 4 {
		if s.Actors[0].FinishExtended(corebattle.LifetimeTail{SecondSubmitted: true}) {
			t.Fatal("expired before fifth submission")
		}
	}
	type checkpoint struct {
		State                                             corebattle.LifetimeState
		Target                                            corebattle.TargetContext
		DamageCalls, HealCalls, PartyHealCalls, CureCalls int
		RequesterLevel                                    int
		Cursor                                            corebattle.TargetCursor
		Arguments                                         []string
		EntryID                                           string
	}
	frozen := checkpoint{s, c.TargetContext, c.DamageCalls, c.HealCalls, c.PartyHealCalls, c.CureCalls, level, c.Cursor, []string{"enemy", "tail"}, "job-32-entry-1"}
	data, err := json.Marshal(frozen)
	if err != nil {
		t.Fatal(err)
	}
	var loaded checkpoint
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatal(err)
	}
	resumed := &corebattle.ProtectiveMagicContext{DamageContext: corebattle.DamageContext{
		EffectContext: corebattle.EffectContext{TargetContext: loaded.Target, DamageCalls: loaded.DamageCalls, HealCalls: loaded.HealCalls, PartyHealCalls: loaded.PartyHealCalls, RequesterLevel: &loaded.RequesterLevel},
		Cursor:        loaded.Cursor, GuaranteedHit: true, Defeat: func(string) error { return nil },
	}, CureCalls: loaded.CureCalls}
	var rebuilt BattleDefinitions
	rebuilt.RegisterProtectiveMagic(resumed)
	for _, run := range []struct {
		s *corebattle.LifetimeState
		c *corebattle.ProtectiveMagicContext
		d BattleDefinitions
	}{{&s, c, d}, {&loaded.State, resumed, rebuilt}} {
		executeProtectiveMagic(t, run.s, run.c, run.d, loaded.EntryID, loaded.Arguments, 0, 0)
		if !run.s.Actors[0].FinishExtended(corebattle.LifetimeTail{SecondSubmitted: true}) {
			t.Fatal("did not expire at fifth submission")
		}
	}
	if !reflect.DeepEqual(s, loaded.State) || s.Actors[1].HP != 734 || s.Actors[2].MP != 0 || s.Actors[0].HP != 634 || s.Actors[0].Extended != "" || resumed.DamageCalls != 1 || resumed.CureCalls != 1 || resumed.HealCalls != 2 || resumed.PartyHealCalls != 1 || !reflect.DeepEqual(c.Arguments, resumed.Arguments) || !reflect.DeepEqual(c.Cursor, resumed.Cursor) || resumed.Command != "frozen-protective-command" || !resumed.Custom || *resumed.RequesterLevel != 99 {
		t.Fatal(s, loaded.State, c, resumed)
	}
}

func TestProtectiveMagicEarlierBarrierChangesLaterEligibility(t *testing.T) {
	for _, supported := range []bool{false, true} {
		s, c, d := protectiveMagicFixture()
		s.Actors[0].Status, s.Actors[0].Temporary = corebattle.StatusConfusion, corebattle.TemporaryNullMagic
		if !supported {
			s.Actors[0].Gear, c.Defeat = nil, nil
		}
		f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"self", "tail"}}
		// Before the barrier, immunity makes the damage descendant unreachable.
		if _, err := d.InvocationAction(&s, &f, []string{"job-34-entry-1"}, noArithmeticDraws{t}); err != nil {
			t.Fatal(err)
		}
		ids := []string{"job-32-entry-3", "job-34-entry-1"}
		ops, err := d.InvocationAction(&s, &f, ids, noArithmeticDraws{t})
		if !supported {
			if !errors.Is(err, corebattle.ErrUnsupportedEffect) || s.Actors[0].Temporary != corebattle.TemporaryNullMagic || c.Arguments != nil {
				t.Fatal(err, s, c)
			}
			ops = []corebattle.CallbackInvocation{{Kind: "entry", EntryID: ids[0]}, {Kind: "entry", EntryID: ids[1]}}
			if err := s.ApplyInvocations(&f, d.Invocations, ops, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) || s.Actors[0].Temporary != corebattle.TemporaryNullMagic {
				t.Fatal(err, s)
			}
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		if s.Actors[0].Temporary != corebattle.TemporaryNullMagic {
			t.Fatal("projection mutated caller")
		}
		rng := &damageGemDraws{t: t, draws: []float64{0}}
		if err := s.ApplyInvocations(&f, d.Invocations, ops, rng); err != nil || rng.used != 1 {
			t.Fatal(err, rng.used)
		}
		if s.Actors[0].HP != 401 || s.Actors[0].Temporary != corebattle.TemporaryReflectMagic || s.Actors[1].HP != 900 || c.DamageCalls != 1 {
			t.Fatal(s, c)
		}
	}
}

func TestProtectiveMagicGroupOrderedResourcesAndSecondResolution(t *testing.T) {
	s, c, d := protectiveMagicFixture()
	s.Actors[0].Status = corebattle.StatusConfusion
	s.Actors[2].MP, s.Actors[2].Temporary = 14, corebattle.TemporaryAbsorbMagic
	// First resolution selects the fallen ally and restores30MP, making it
	// eligible to pay15. The second resolution independently selects the enemy.
	executeProtectiveMagic(t, &s, c, d, "job-34-entry-6", []string{"enemy", "tail"}, .8, .8, 0, .4, .4, 0)
	if s.Actors[2].MP != 29 || s.Actors[2].HP != 0 || s.Actors[1].HP != 734 || c.DamageCalls != 1 {
		t.Fatal(s, c)
	}
	s, c, d = protectiveMagicFixture()
	s.Actors[1].Temporary = corebattle.TemporaryNullMagic
	c.Defeat = nil
	// A resolved symbolic helper with zeroed payload still debits allies;
	// the numeric damage helper then independently observes magic immunity.
	executeProtectiveMagic(t, &s, c, d, "job-34-entry-6", []string{"enemy"})
	if s.Actors[2].MP != 0 || s.Actors[1].HP != 900 || c.DamageCalls != 0 {
		t.Fatal(s, c)
	}
	s, c, d = protectiveMagicFixture()
	s.Actors[1].Temporary = corebattle.TemporaryReflectMagic
	executeProtectiveMagic(t, &s, c, d, "job-34-entry-6", []string{"enemy"}, 0, 0)
	if s.Actors[2].MP != 0 || s.Actors[0].HP != 334 || s.Actors[1].HP != 900 || c.DamageCalls != 1 {
		t.Fatal(s, c)
	}
}
