package skill

import (
	"math"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func TestBaseGuardProbabilityBoundaries(t *testing.T) {
	for _, row := range []struct {
		id        string
		threshold float64
		low       int
	}{
		{"job-1-entry-5", .5, 1}, {"job-1-entry-6", 2.0 / 3, 68},
	} {
		for _, draw := range []float64{math.Nextafter(row.threshold, 0), row.threshold, math.Nextafter(row.threshold, 1)} {
			s, c, d := baseGuardFixture()
			want := row.low
			draws := []float64{draw, 0}
			if draw < row.threshold {
				want = 113
			} else if row.low == 1 {
				draws = append(draws, 0)
			} // Minimum damage has its own draw.
			executeBaseGuard(t, &s, c, d, row.id, []string{"enemy"}, draws...)
			if 1000-s.Actors[1].HP != want {
				t.Fatal(row, draw, s.Actors[1])
			}
		}
	}
}

func TestBaseGuardRatioStrictOrderedBoundaries(t *testing.T) {
	for _, row := range []struct{ threshold, atOrBelow, above int }{
		{15000, 126, 135}, {10000, 117, 126}, {8000, 108, 117}, {5000, 99, 108}, {3000, 90, 99},
	} {
		for _, offset := range []int{-1, 0, 1} {
			s, c, d := baseGuardFixture()
			s.Actors[0].HP, s.Actors[0].MaxHP = 1000, 1000
			s.Actors[1].HP = row.threshold + offset
			before := s.Actors[1].HP
			executeBaseGuard(t, &s, c, d, "job-2-entry-8", []string{"enemy"}, 0)
			want := row.atOrBelow
			if offset > 0 {
				want = row.above
			}
			if before-s.Actors[1].HP != want {
				t.Fatal(row, offset, s)
			}
		}
	}
	// The first MaxHP comparison wins even when later current-HP ratios are larger.
	s, c, d := baseGuardFixture()
	s.Actors[0].HP, s.Actors[0].MaxHP = 1000, 10
	s.Actors[1].HP = 151
	executeBaseGuard(t, &s, c, d, "job-2-entry-8", []string{"enemy"}, 0)
	if 151-s.Actors[1].HP != 135 {
		t.Fatal(s)
	}
	for _, target := range []string{"", "missing", "fallen", "self"} {
		s, c, d := baseGuardFixture()
		executeBaseGuard(t, &s, c, d, "job-2-entry-8", []string{target}, 0)
		if s.Actors[1].HP != 928 || c.AreaDamageCalls != 1 {
			t.Fatal(target, s, c)
		}
	}
	s, c, d = baseGuardFixture()
	s.Actors[1].HP = 0
	executeBaseGuard(t, &s, c, d, "job-2-entry-8", []string{"enemy"})
	if c.AreaDamageCalls != 0 || c.DamageCalls != 0 {
		t.Fatal(c)
	}
}

func TestBaseGuardCoverBindingsAndNoEffect(t *testing.T) {
	for _, id := range []string{"job-1-entry-2", "job-2-entry-4", "job-3-entry-1"} {
		for _, target := range []string{"self", "fallen", "missing", "enemy", ""} {
			for _, fallen := range []bool{false, true} {
				s, c, d := baseGuardFixture()
				s.Actors[0].Temporary, s.Actors[2].Temporary = corebattle.TemporaryStrongDefense, corebattle.TemporaryReflectMagic
				if fallen {
					s.Actors[2].HP = 0
				}
				draws := []float64(nil)
				if target == "missing" || target == "enemy" || target == "" {
					draws = []float64{0}
				}
				executeBaseGuard(t, &s, c, d, id, []string{target}, draws...)
				wantSelf, wantAlly := corebattle.TemporaryStrongDefense, corebattle.TemporaryReflectMagic
				if target == "fallen" && !fallen {
					wantSelf, wantAlly = corebattle.TemporaryCovering, corebattle.TemporaryCover
				}
				if s.Actors[0].Temporary != wantSelf || s.Actors[2].Temporary != wantAlly {
					t.Fatal(id, target, fallen, s)
				}
			}
		}
		// An unsuccessful physical-seal cure consumes its draw without rebinding.
		s, c, d := baseGuardFixture()
		s.Actors[0].Status = corebattle.StatusPhysicalSeal
		executeBaseGuard(t, &s, c, d, id, []string{"fallen"}, .9)
		if s.Actors[0].Temporary != "" || s.Actors[2].Temporary != "" {
			t.Fatal(s)
		}
	}
}

func TestBaseGuardDamageTailGuards(t *testing.T) {
	for _, tmp := range []corebattle.TemporaryEffect{corebattle.TemporaryOneHitDefense, corebattle.TemporaryNullAttack, corebattle.TemporaryReflectAttack, ""} {
		for _, hp := range []int{1, 1000} {
			s, c, d := baseGuardFixture()
			s.Actors[1].HP, s.Actors[1].Temporary = hp, tmp
			draws := []float64{0}
			if tmp == corebattle.TemporaryOneHitDefense || tmp == corebattle.TemporaryNullAttack {
				draws = nil
			}
			if tmp == corebattle.TemporaryReflectAttack {
				draws = append(draws, 0)
			}
			executeBaseGuard(t, &s, c, d, "job-1-entry-1", []string{"enemy"}, draws...)
			wantEnemy, wantSelf := 80, 80
			if tmp == "" && hp > 1 {
				wantEnemy = 68
			}
			if tmp == corebattle.TemporaryReflectAttack {
				wantSelf = 68
			}
			if s.Actors[0].Defense != wantSelf || s.Actors[1].Defense != wantEnemy {
				t.Fatal(tmp, hp, s, c)
			}
		}
	}
	for _, tmp := range []corebattle.TemporaryEffect{corebattle.TemporaryNullAttack, corebattle.TemporaryOneHitDefense} {
		s, c, d := baseGuardFixture()
		s.Actors[1].Temporary = tmp
		executeBaseGuard(t, &s, c, d, "job-3-entry-3", []string{"enemy"})
		if s.Actors[0].Temporary != corebattle.TemporaryDouble || s.Actors[1].HP != 1000 {
			t.Fatal(s)
		}
	}
	// A reflected first hit defeats the caster, so the captured second hit stops.
	s, c, d := baseGuardFixture()
	s.Actors[0].HP = 1
	s.Actors[1].Temporary = corebattle.TemporaryReflectAttack
	c.Defeat = func(id string) error {
		if id != "self" {
			t.Fatal(id)
		}
		return nil
	}
	executeBaseGuard(t, &s, c, d, "job-2-entry-6", []string{"enemy", "fallen"}, 0)
	if s.Actors[0].HP != 0 || c.DamageCalls != 1 || s.Actors[2].HP != 100 || len(c.Arguments) != 1 || c.Arguments[0] != "fallen" {
		t.Fatal(s, c)
	}
	// A one-use defense is consumed by the first hit; the second uses the same target.
	s, c, d = baseGuardFixture()
	s.Actors[1].Temporary = corebattle.TemporaryOneHitDefense
	executeBaseGuard(t, &s, c, d, "job-2-entry-6", []string{"enemy", "fallen"}, 0)
	if s.Actors[1].HP != 963 || s.Actors[2].HP != 100 || c.DamageCalls != 2 {
		t.Fatal(s, c)
	}
}

func TestBaseGuardEveryEntryNoEffectBoundary(t *testing.T) {
	for job := 0; job <= 3; job++ {
		for _, entry := range corebattle.BaseGuardEntries(job) {
			if entry.ID == "job-3-entry-6" {
				continue
			}
			t.Run(entry.ID, func(t *testing.T) {
				s, c, d := baseGuardFixture()
				// No enemies: attacks/status return without needing defeat/reset.
				c.EnemyIDs = nil
				c.Defeat = nil
				s.Actors[1].TeamID = "party"
				c.PartyIDs = append(c.PartyIDs, "enemy")
				s.Actors[0].Status = corebattle.StatusSabaku
				s.Actors[0].HP, s.Actors[2].HP, s.Actors[1].HP = 0, 0, 0
				draws := []float64(nil)
				if entry.ID == "job-1-entry-5" || entry.ID == "job-1-entry-6" {
					draws = []float64{0}
				}
				if entry.ID == "job-1-entry-2" || entry.ID == "job-2-entry-4" || entry.ID == "job-3-entry-1" {
					draws = []float64{0}
				}
				executeBaseGuard(t, &s, c, d, entry.ID, []string{"missing"}, draws...)
				if c.DamageCalls != 0 || s.Actors[0].Attack != 100 || s.Actors[0].Defense != 80 || s.Actors[0].Tension != 1 || s.Actors[0].MP != 80 {
					t.Fatal(s, c)
				}
			})
		}
	}
}

func TestBaseGuardJobZeroUsesOneOrdinaryHelper(t *testing.T) {
	for _, id := range []string{"job-0-entry-0", "job-0-entry-1", "job-0-entry-2"} {
		s, c, d := baseGuardFixture()
		s.Actors[0].Abilities = []string{"seal_shinsoku"}
		executeBaseGuard(t, &s, c, d, id, []string{"enemy"}, .5, 0)
		if c.DamageCalls != 1 || s.Actors[1].HP != 977 {
			t.Fatal(s, c)
		}
	}
}
