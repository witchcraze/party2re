package skill

import (
	"errors"
	"math"
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func TestAdvancedMagicEveryPositionSealBoundary(t *testing.T) {
	for _, job := range []int{15, 16, 19} {
		for _, entry := range corebattle.AdvancedMagicEntries(job) {
			if entry.ID == "job-16-entry-1" || entry.ID == "job-16-entry-6" {
				continue
			}
			t.Run(entry.ID, func(t *testing.T) {
				s, c, d := advancedMagicFixture()
				s.Actors[0].Status = corebattle.StatusMagicSeal
				before := s
				before.Actors = append([]corebattle.LifetimeActor(nil), s.Actors...)
				if entry.ID == "job-16-entry-8" {
					f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"self"}}
					if _, err := d.InvocationAction(&s, &f, []string{entry.ID}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
						t.Fatal(err)
					}
				} else {
					draws := []float64{.99}
					if entry.ID == "job-15-entry-5" {
						draws = []float64{0, .99}
					}
					if entry.ID == "job-19-entry-8" {
						draws = []float64{0, 0, .99, 0, .99, 0, .99, 0, .99}
					}
					executeAdvancedMagic(t, &s, c, d, entry.ID, []string{"self"}, draws...)
				}
				if !reflect.DeepEqual(s, before) {
					t.Fatal(s, before)
				}
			})
		}
	}
}

func TestAdvancedMagicPoisonOrderedRecipientAndGuards(t *testing.T) {
	for _, row := range []struct {
		name            string
		hp              int
		tmp             corebattle.TemporaryEffect
		chance          float64
		selfHP, enemyHP int
		status          string
		calls           int
	}{
		{"cross999-success", 1000, "", .79999, 500, 973, corebattle.StatusDeadlyPoison, 1},
		{"strict80-failure", 999, "", .8, 500, 972, "", 1},
		{"one-hit-no-tuple", 900, corebattle.TemporaryOneHitDefense, 0, 500, 900, "", 0},
		{"reflection-resolves-again", 900, corebattle.TemporaryReflectMagic, 0, 473, 900, "", 1},
		{"death-before-status", 1, "", 0, 500, 0, "", 0},
	} {
		t.Run(row.name, func(t *testing.T) {
			s, c, d := advancedMagicFixture()
			s.Actors[1].HP, s.Actors[1].Temporary = row.hp, row.tmp
			draws := []float64{0, row.chance}
			if row.tmp == corebattle.TemporaryOneHitDefense {
				draws = nil
			}
			if row.tmp == corebattle.TemporaryReflectMagic {
				draws = []float64{0, 0, row.chance}
			}
			if row.hp == 1 {
				draws = []float64{0}
			}
			executeAdvancedMagic(t, &s, c, d, "job-15-entry-0", []string{"enemy"}, draws...)
			if s.Actors[0].HP != row.selfHP || s.Actors[1].HP != row.enemyHP || s.Actors[1].Status != row.status || c.Status.StatusCalls != row.calls {
				t.Fatal(s, c.Status)
			}
		})
	}
}

func TestAdvancedMagicDrainIntervalsAndNoReturnedValue(t *testing.T) {
	for _, row := range []struct {
		draw         float64
		damage, heal int
	}{
		{0, 45, 40}, {math.Nextafter(1, 0), 135, 121},
	} {
		s, c, d := advancedMagicFixture()
		executeAdvancedMagic(t, &s, c, d, "job-15-entry-5", []string{"enemy"}, row.draw, 0, 0)
		if s.Actors[1].HP != 900-row.damage || s.Actors[0].HP != 500+row.heal || c.DamageCalls != 1 || c.HealCalls != 1 {
			t.Fatal(s, c)
		}
	}
	for _, tmp := range []corebattle.TemporaryEffect{corebattle.TemporaryOneHitDefense, corebattle.TemporaryNullMagic} {
		s, c, d := advancedMagicFixture()
		s.Actors[1].Temporary = tmp
		executeAdvancedMagic(t, &s, c, d, "job-15-entry-5", []string{"enemy"}, 0)
		if s.Actors[0].HP != 500 || s.Actors[1].HP != 900 || c.HealCalls != 0 {
			t.Fatal(s, c)
		}
	}
}

func TestAdvancedMagicTensionRawPowerAndSharedMultiplier(t *testing.T) {
	for _, row := range []struct {
		tension float64
		hp      int
	}{{1, 9550}, {6, 5050}, {10, 0}} {
		s, c, d := advancedMagicFixture()
		s.Actors[0].Tension, s.Actors[1].HP = row.tension, 10000
		executeAdvancedMagic(t, &s, c, d, "job-15-entry-7", []string{"enemy"}, 0)
		if s.Actors[1].HP != row.hp || s.Actors[0].Tension != 1 {
			t.Fatal(s)
		}
	}
}

func TestAdvancedMagicMPTheftFloorsImmunityAndCap(t *testing.T) {
	for _, id := range []string{"job-15-entry-4", "job-19-entry-6"} {
		for _, blocked := range []string{"floor", "curse", "base-defense"} {
			s, c, d := advancedMagicFixture()
			switch blocked {
			case "floor":
				s.Actors[1].MP = 20
			case "curse":
				s.Actors[1].Abilities = []string{"curse"}
			case "base-defense":
				s.Actors[1].BaseStats.Defense = 1000
			}
			mp := s.Actors[1].MP
			executeAdvancedMagic(t, &s, c, d, id, []string{"enemy"})
			if s.Actors[0].MP != 80 || s.Actors[1].MP != mp || c.MPHealCalls != 0 {
				t.Fatal(id, blocked, s, c)
			}
		}
		s, c, d := advancedMagicFixture()
		s.Actors[1].MP, s.Actors[1].MaxMP = 1000, 1000
		s.Actors[0].MP, s.Actors[0].MaxMP = 0, 1000
		executeAdvancedMagic(t, &s, c, d, id, []string{"enemy"}, .999, 0)
		if s.Actors[1].MP != 851 || s.Actors[0].MP != 135 || c.MPHealCalls != 1 {
			t.Fatal(id, s, c)
		}
	}
}

func TestAdvancedMagicTemporaryWritesAndRecoveryFallback(t *testing.T) {
	for _, row := range []struct {
		id  string
		tmp corebattle.TemporaryEffect
	}{
		{"job-15-entry-3", corebattle.TemporaryReflectMagic}, {"job-19-entry-1", corebattle.TemporaryReflectMagic},
		{"job-16-entry-4", corebattle.TemporaryRevival}, {"job-16-entry-5", corebattle.TemporaryReduceMagic},
	} {
		s, c, d := advancedMagicFixture()
		executeAdvancedMagic(t, &s, c, d, row.id, []string{"fallen"})
		if s.Actors[0].Temporary != row.tmp || s.Actors[2].Temporary != "" {
			t.Fatal(s)
		}
	}
	for _, id := range []string{"job-16-entry-0", "job-16-entry-3"} {
		s, c, d := advancedMagicFixture()
		s.Actors[0].HP = 1000
		executeAdvancedMagic(t, &s, c, d, id, []string{"fallen"}, 0)
		if s.Actors[0].HP != 1000 || s.Actors[2].HP != 0 || c.HealCalls != 1 {
			t.Fatal(s, c)
		}
	}
	// Payload cancellation does not suppress16.8's raw write or party heal.
	s, c, d := advancedMagicFixture()
	s.Actors[0].Temporary = corebattle.TemporaryNullMagic
	executeAdvancedMagic(t, &s, c, d, "job-16-entry-8", []string{"self"}, 0, 0)
	if s.Actors[0].Temporary != corebattle.TemporaryNullAttack || s.Actors[0].HP != 1000 || c.PartyHealCalls != 1 {
		t.Fatal(s, c)
	}
}
