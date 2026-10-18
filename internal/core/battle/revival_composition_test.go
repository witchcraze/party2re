package battle

import (
	"errors"
	"reflect"
	"testing"

	"github.com/witchcraze/party2re/internal/core/random"
)

func TestRevivalTargetHooksAndAbsorptionBeforeMPFormula(t *testing.T) {
	for _, tc := range []struct {
		name, id, status string
		tmp              TemporaryEffect
		target           string
		draws            []float64
		wantTarget       string
		wantHP, wantMP   int
	}{
		{"absorption before MP revival", "job-75-entry-5", "", TemporaryAbsorbMagic, "fallen", []float64{0}, "fallen", 30, 15},
		{"den absorption", "job-75-entry-5", "", TemporaryAbsorbMagic, "fallen", []float64{0}, "fallen", 1, 0},
		{"tool bypasses magic seal", "tool-6", StatusMagicSeal, "", "fallen", nil, "fallen", 101, 0},
		{"seal failed cure", "gem-64", StatusMagicSeal, "", "fallen", []float64{.25}, "fallen", 0, 0},
		{"seal successful cure", "gem-64", StatusMagicSeal, "", "fallen", []float64{.249}, "fallen", 101, 0},
		{"reflection to living caster", "gem-64", "", TemporaryReflectMagic, "fallen", nil, "fallen", 0, 0},
		{"confusion revives fallen enemy", "gem-64", StatusConfusion, "", "fallen", []float64{.9}, "dead_enemy", 101, 0},
		{"nonrevival last fallback", "job-50-entry-6", "", "", "fallen", nil, "fallen", 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, c := revivalFixture()
			s.actor("self").Status = tc.status
			s.actor("fallen").Temporary = tc.tmp
			if tc.name == "den absorption" {
				s.Fields.Default = "den"
			}
			rng := &targetRNG{t: t, draws: tc.draws}
			if _, err := s.ApplyRevival(&c, tc.id, tc.target, rng); err != nil || s.actor(tc.wantTarget).HP != tc.wantHP || s.actor(tc.wantTarget).MP != tc.wantMP {
				t.Fatal(s, err)
			}
			rng.done()
		})
	}
}

func TestRevivalNamedHelperPrecedenceAndRosterBridge(t *testing.T) {
	s, c := revivalFixture()
	s.actor("self").HP = 0
	s.actor("ally").Icon = nativeFormIcon("job-66", "")
	s.actor("enemy").RosterIdentity = "calling-66"
	s.actor("enemy").HP = 0
	if r, err := s.ApplyRevival(&c, "job-66-revive", "", &targetRNG{t: t}); err != nil || r.Applied {
		t.Fatal(r, err)
	}
	s.actor("enemy").RosterIdentity = ""
	rc := RosterContext{DamageContext: DamageContext{EffectContext: EffectContext{TargetContext: c.TargetContext}}}
	if r, err := s.ApplyRoster(&rc, "job-66-revive", "", &targetRNG{t: t}); err != nil || !r.Applied || r.HelperID != "ally" || s.actor("self").HP != 50 || s.actor("ally").HP != 50 {
		t.Fatal(s, r, err)
	}
}

func TestSacrificeFirstAndSecondDefeatShareAutomaticRevival(t *testing.T) {
	for _, ability := range []string{"pharaoh", "amulet"} {
		s, c := revivalFixture()
		a := s.actor("self")
		a.MP = 100
		if ability == "pharaoh" {
			a.Abilities = []string{"pharaoh"}
		} else {
			a.ItemDefinitionIDs = []string{"item-193"}
		}
		var order []string
		reset := func(id string) error { s.actor(id).ResetEffects(LifetimeReset{}); return nil }
		c.Defeat = func(id string) error {
			order = append(order, "defeat")
			r, err := s.ApplyAutomaticRevival(&AutomaticRevivalContext{TargetContext: c.TargetContext, Reset: reset}, id, &targetRNG{t: t})
			if err != nil || r.Prevented {
				return err
			}
			s.actor(id).HP = 0
			return reset(id)
		}
		c.Deaths = &RevivalDeathCallbacks{
			Validate: func(*LifetimeState, TargetContext, CallbackCategory, float64) error { return nil },
			Execute: func(_ *LifetimeState, _ TargetContext, _ CallbackCategory, _ float64, _ random.Generator) error {
				return c.Defeat("self")
			},
		}
		if _, err := s.ApplyRevival(&c, "job-7-entry-8", "", &targetRNG{t: t}); err != nil {
			t.Fatal(err)
		}
		wantHP := 101
		if ability == "amulet" {
			wantHP = 0
		}
		if !reflect.DeepEqual(order, []string{"defeat", "defeat"}) || a.HP != wantHP || a.MP != 1 {
			t.Fatal(s, order)
		}
		if ability == "amulet" && len(a.ConsumedItems) != 1 {
			t.Fatal(s)
		}
	}
}

func TestRevivalClosureErrorsStopBeforeFinalMPWrite(t *testing.T) {
	failure := errors.New("defeat failed")
	s, c := revivalFixture()
	c.Defeat = func(string) error { return failure }
	if _, err := s.ApplyRevival(&c, "gem-66", "", &targetRNG{t: t}); !errors.Is(err, failure) || s.actor("self").MP != 3 {
		t.Fatal(s, err)
	}
	c.Deaths = &RevivalDeathCallbacks{
		Validate: func(*LifetimeState, TargetContext, CallbackCategory, float64) error { return failure },
		Execute: func(*LifetimeState, TargetContext, CallbackCategory, float64, random.Generator) error {
			t.Fatal("executed rejected closure")
			return nil
		},
	}
	before := reloadLifetime(t, s)
	if _, err := s.ApplyRevival(&c, "job-7-entry-8", "", &targetRNG{t: t}); !errors.Is(err, failure) || !reflect.DeepEqual(before, s) {
		t.Fatal(s, err)
	}
}
