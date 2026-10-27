package skill

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func TestGuardianReloadContinuesWithoutReplay(t *testing.T) {
	s, c, d := guardianFixture()
	level := 99
	c.RequesterLevel, c.Command, c.Custom = &level, "frozen-guardian-command", true
	c.Cursor = corebattle.TargetCursor{ActorID: "self", Source: "item159"}
	s.Actors[0].Tension = 5
	executeGuardian(t, &s, c, d, "job-18-entry-7", []string{"enemy", "first-tail"}, 0, 0, 0)
	s.Actors[0].HP = 0
	type checkpoint struct {
		State                                                                                           corebattle.LifetimeState
		Target                                                                                          corebattle.TargetContext
		DamageCalls, HealCalls, PartyHealCalls, CureCalls, PartyCureCalls, StatusCalls, AreaStatusCalls int
		RequesterLevel                                                                                  int
		Cursor                                                                                          corebattle.TargetCursor
		EntryID                                                                                         string
		Arguments                                                                                       []string
	}
	frozen := checkpoint{s, c.TargetContext, c.DamageCalls, c.HealCalls, c.PartyHealCalls, c.CureCalls, c.PartyCureCalls, c.StatusCalls, c.AreaStatusCalls, level, c.Cursor, "job-18-entry-7", []string{"enemy", "repeat-tail"}}
	data, err := json.Marshal(frozen)
	if err != nil {
		t.Fatal(err)
	}
	var loaded checkpoint
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatal(err)
	}
	resumed := &corebattle.GuardianHealingContext{DamageContext: corebattle.DamageContext{
		EffectContext: corebattle.EffectContext{TargetContext: loaded.Target, DamageCalls: loaded.DamageCalls, HealCalls: loaded.HealCalls, PartyHealCalls: loaded.PartyHealCalls, RequesterLevel: &loaded.RequesterLevel},
		Cursor:        loaded.Cursor, GuaranteedHit: true,
	}, CureCalls: loaded.CureCalls, PartyCureCalls: loaded.PartyCureCalls, StatusCalls: loaded.StatusCalls, AreaStatusCalls: loaded.AreaStatusCalls}
	var rebuilt BattleDefinitions
	rebuilt.RegisterGuardianHealing(resumed)
	// The repeated color needs no defeat port, including after reconstruction.
	c.Defeat = nil
	for _, run := range []struct {
		s *corebattle.LifetimeState
		c *corebattle.GuardianHealingContext
		d BattleDefinitions
	}{{&s, c, d}, {&loaded.State, resumed, rebuilt}} {
		executeGuardian(t, run.s, run.c, run.d, loaded.EntryID, loaded.Arguments, 0, 0)
	}
	if !reflect.DeepEqual(s, loaded.State) || s.Actors[0].HP != 271 || s.Actors[2].HP != 541 || s.Actors[1].HP != 414 || s.Actors[0].Tension != 5 || resumed.DamageCalls != 1 || resumed.HealCalls != 4 || resumed.PartyHealCalls != 0 || !reflect.DeepEqual(c.Arguments, resumed.Arguments) || !reflect.DeepEqual(c.Cursor, resumed.Cursor) || resumed.Command != "frozen-guardian-command" || !resumed.Custom || *resumed.RequesterLevel != 99 {
		t.Fatal(s, loaded.State, c, resumed)
	}
}

func TestGuardianBarrierMakesLaterDamageReachable(t *testing.T) {
	s, c, d := guardianFixture()
	for i := range s.Actors {
		s.Actors[i].Temporary = corebattle.TemporaryNullMagic
	}
	c.Defeat = nil
	f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy"}}
	// With all heals blocked, zero damage has no defeat descendant.
	if _, err := d.InvocationAction(&s, &f, []string{"job-18-entry-7"}, noArithmeticDraws{t}); err != nil {
		t.Fatal(err)
	}
	// The exact barrier transition replaces immunity on living party members.
	// Reflection now reaches that caster, requiring its defeat owner up front.
	s.Actors[1].Temporary = corebattle.TemporaryReflectMagic
	ops := []corebattle.CallbackInvocation{{Kind: "entry", EntryID: "job-17-entry-2"}, {Kind: "entry", EntryID: "job-18-entry-7"}}
	if err := s.ApplyInvocations(&f, d.Invocations, ops, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
		t.Fatal(err)
	}
	if s.Actors[0].Temporary != corebattle.TemporaryNullMagic || s.Actors[2].HP != 0 || c.HealCalls != 0 {
		t.Fatal(s, c)
	}
}

func TestGuardianRestorationOrderedFallenCasterAndAbsorption(t *testing.T) {
	s, c, d := guardianFixture()
	s.Actors[0].HP, s.Actors[0].Tension = 0, 7
	s.Actors[2].Temporary = corebattle.TemporaryReflectMagic
	c.PartyIDs = []string{"fallen", "self"}
	// First ally revives, but its reflected heal cannot heal the still-fallen
	// caster. The later caster revival changes eligibility for the next heal.
	executeGuardian(t, &s, c, d, "job-18-entry-7", []string{"enemy"}, 0, 0)
	if s.Actors[2].HP != 1 || s.Actors[0].HP != 271 || s.Actors[0].Tension != 7 || s.Actors[1].HP != 657 || c.HealCalls != 1 {
		t.Fatal(s, c)
	}
	s, c, d = guardianFixture()
	s.Actors[2].Temporary, s.Actors[2].MaxMP = corebattle.TemporaryAbsorbMagic, 100
	executeGuardian(t, &s, c, d, "job-18-entry-7", []string{"enemy"}, 0, 0, 0, 0)
	if s.Actors[2].MP != 60 || s.Actors[2].HP != 271 || s.Actors[1].HP != 414 {
		t.Fatal(s, c)
	}
}

func TestGuardianRestorationSanctuaryUsesReturnedHealing(t *testing.T) {
	s, c, d := guardianFixture()
	s.Fields.Default = "sanctuary"
	s.Actors[0].HP, s.Actors[2].HP = 1000, 1000
	executeGuardian(t, &s, c, d, "job-18-entry-7", []string{"enemy"}, 0, 0, 0)
	if s.Actors[0].HP != 676 || s.Actors[2].HP != 676 || s.Actors[1].HP != 201 {
		t.Fatal(s)
	}
}
