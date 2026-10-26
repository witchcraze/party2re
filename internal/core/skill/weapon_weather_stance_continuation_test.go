package skill

import (
	"encoding/json"
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func TestWeaponWeatherStanceOrderedEnchantmentAndReflection(t *testing.T) {
	s, c, d := weaponWeatherStanceFixture()
	c.Command = "flame_slash"
	f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy"}}
	ops, err := d.InvocationAction(&s, &f, []string{"job-24-entry-5", "job-24-entry-0"}, noArithmeticDraws{t})
	if err != nil {
		t.Fatal(err)
	}
	rng := &damageGemDraws{t: t, draws: []float64{0}}
	if err := s.ApplyInvocations(&f, d.Invocations, ops, rng); err != nil {
		t.Fatal(err)
	}
	// The earlier enchantment converts physical120 to direct magic144.
	if s.Actors[1].HP != 771 || s.Actors[0].ExtendedTurns != 7 || rng.used != 1 {
		t.Fatal(s, c)
	}
	// Breaking a reflected physical target resets the caster's AT/tension;
	// the stat-down selects an enemy again, whose reflection returns to self.
	s, c, d = weaponWeatherStanceFixture()
	s.Actors[1].Temporary = corebattle.TemporaryReflectAttack
	s.Actors[0].Attack, s.Actors[0].Tension = 400, 4
	executeWeaponWeatherStance(t, &s, c, d, "job-27-entry-7", []string{"enemy"}, 0)
	if s.Actors[0].Attack != 80 || s.Actors[0].Tension != 1 || s.Actors[1].Attack != 200 || s.Actors[1].Temporary != corebattle.TemporaryReflectAttack {
		t.Fatal(s)
	}
}

func TestWeaponWeatherStanceHealCureChangesLaterEligibility(t *testing.T) {
	// Healing rage removes it before the next ally helper redirects to self.
	// Cure captures the loop member's state before its own target replacement.
	s, c, d := weaponWeatherStanceFixture()
	s.Actors[0].Temporary = corebattle.TemporaryRage
	s.Actors[0].Status, s.Actors[2].Status = corebattle.StatusPoison, corebattle.StatusSleep
	executeWeaponWeatherStance(t, &s, c, d, "job-27-entry-8", nil, 0, 0)
	if s.Actors[0].HP != 1000 || s.Actors[0].Temporary != "" || s.Actors[0].Status != "" || s.Actors[2].Status != corebattle.StatusSleep || c.PartyHealCalls != 1 || c.HealCalls != 2 || c.CureCalls != 2 {
		t.Fatal(s, c)
	}
	// A first cure of the caster's confusion changes later target selection.
	s, c, d = weaponWeatherStanceFixture()
	s.Actors[0].Status = corebattle.StatusConfusion
	executeWeaponWeatherStance(t, &s, c, d, "job-27-entry-8", nil, 0, 0, 0, 0)
	if s.Actors[0].HP != 1000 || s.Actors[0].Status != "" || c.HealCalls != 2 || c.CureCalls != 1 {
		t.Fatal(s, c)
	}
}

func TestWeaponWeatherStanceReloadContinuesWithoutReplay(t *testing.T) {
	s, c, d := weaponWeatherStanceFixture()
	c.Command, c.Custom = "frozen-command", true
	c.Cursor = corebattle.TargetCursor{ActorID: "self", Source: "item159"}
	level := 99
	c.RequesterLevel = &level
	s.Actors[0].Status = corebattle.StatusPoison
	executeWeaponWeatherStance(t, &s, c, d, "job-27-entry-8", nil, 0, 0)
	type checkpoint struct {
		State                                                corebattle.LifetimeState
		Target                                               corebattle.TargetContext
		Cursor                                               corebattle.TargetCursor
		HealCalls, PartyHealCalls, CureCalls, RequesterLevel int
		Arguments                                            []string
		EntryID                                              string
	}
	frozen := checkpoint{s, c.TargetContext, c.Cursor, c.HealCalls, c.PartyHealCalls, c.CureCalls, level, []string{"enemy", "tail"}, "job-27-entry-7"}
	data, err := json.Marshal(frozen)
	if err != nil {
		t.Fatal(err)
	}
	var loaded checkpoint
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatal(err)
	}
	resumed := &corebattle.WeaponWeatherStanceContext{DamageContext: corebattle.DamageContext{
		EffectContext: corebattle.EffectContext{TargetContext: loaded.Target, HealCalls: loaded.HealCalls, PartyHealCalls: loaded.PartyHealCalls, RequesterLevel: &loaded.RequesterLevel},
		Cursor:        loaded.Cursor,
	}, CureCalls: loaded.CureCalls}
	var rebuilt BattleDefinitions
	rebuilt.RegisterWeaponWeatherStance(resumed)
	for _, run := range []struct {
		s *corebattle.LifetimeState
		c *corebattle.WeaponWeatherStanceContext
		d BattleDefinitions
	}{{&s, c, d}, {&loaded.State, resumed, rebuilt}} {
		executeWeaponWeatherStance(t, run.s, run.c, run.d, loaded.EntryID, loaded.Arguments)
	}
	if !reflect.DeepEqual(s, loaded.State) || s.Actors[1].Attack != 40 || s.Actors[0].HP != 1000 || resumed.HealCalls != 2 || resumed.PartyHealCalls != 1 || resumed.CureCalls != 1 || resumed.StatDownCalls != 1 || !reflect.DeepEqual(c.Arguments, resumed.Arguments) || !reflect.DeepEqual(c.Cursor, resumed.Cursor) || resumed.Command != "frozen-command" || !resumed.Custom || *resumed.RequesterLevel != 99 {
		t.Fatal(s, loaded.State, c, resumed)
	}
}

func TestWeaponWeatherStanceSixthAndSeventhDamageTailReload(t *testing.T) {
	s, c, d := weaponWeatherStanceFixture()
	executeWeaponWeatherStance(t, &s, c, d, "job-24-entry-5", nil)
	a := &s.Actors[0]
	if a.FinishExtended(corebattle.LifetimeTail{}) || a.ExtendedTurns != 7 {
		t.Fatal(a)
	}
	for range 6 {
		if a.FinishExtended(corebattle.LifetimeTail{DamageCalls: 1}) {
			t.Fatal("expired before seventh damage tail")
		}
	}
	if a.Extended != corebattle.ExtendedMagicSword || a.ExtendedTurns != 1 {
		t.Fatal(a)
	}
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	var loaded corebattle.LifetimeState
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatal(err)
	}
	if !a.FinishExtended(corebattle.LifetimeTail{DamageCalls: 1}) || !loaded.Actors[0].FinishExtended(corebattle.LifetimeTail{DamageCalls: 1}) || a.Extended != "" || !reflect.DeepEqual(s, loaded) {
		t.Fatal(s, loaded)
	}
}

func TestWeaponWeatherStanceCurrentPreviousCostAndNameCollisions(t *testing.T) {
	for _, current := range []string{"job-24", "job-28"} {
		s, c, d := weaponWeatherStanceFixture()
		s.Actors[0].Jobs = corebattle.BattleJobs{JobID: current, SP: 130, OldJobID: "job-27", OldSP: 110}
		for _, job := range []int{24, 27} {
			for i := range d.Invocations.JobTables[job].Variants[0] {
				d.Invocations.JobTables[job].Variants[0][i].Name = "shared-display-alias"
			}
		}
		d.Invocations.Entries["extended"] = corebattle.CallbackDefinition{Invocation: corebattle.CallbackInvocation{Kind: "gem-86"}}
		draws := []float64(nil)
		wantMP, wantHP := 65, 500
		if current == "job-28" { // No current extended entry: use old job27.
			wantMP, wantHP, draws = 35, 1000, []float64{0, 0}
		}
		executeWeaponWeatherStance(t, &s, c, d, "extended", []string{"enemy"}, draws...)
		if s.Actors[0].MP != wantMP || s.Actors[0].HP != wantHP || current == "job-24" && s.Actors[0].ExtendedTurns != 7 {
			t.Fatal(current, s)
		}
	}
	// Indirect positional selection ignores learning, listed cost and aliases.
	s, c, d := weaponWeatherStanceFixture()
	s.Actors[0].Jobs.JobID, s.Actors[0].MP = "job-24", 0
	d.Invocations.Entries["tool"] = corebattle.CallbackDefinition{Invocation: corebattle.CallbackInvocation{Kind: "tool-84"}}
	executeWeaponWeatherStance(t, &s, c, d, "tool", []string{"ignored"}, 1.5/6, 0, 0)
	if s.Actors[0].MP != 0 || s.Actors[1].HP != 864 {
		t.Fatal(s)
	}
}
