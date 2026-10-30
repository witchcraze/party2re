package skill

import (
	"math"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func TestAdvancedMagicLoopHPAndBaseDefense999Boundary(t *testing.T) {
	for _, row := range []struct{ hp, baseDF, damage, deaths int }{
		{999, 999, 0, 4}, {1000, 999, 360, 3},
		{999, 1000, 632, 0}, {1000, 1000, 632, 0},
	} {
		s, c, d := advancedMagicFixture()
		s.Actors[1].HP, s.Actors[1].BaseStats.Defense = row.hp, row.baseDF
		// Current defense deliberately disagrees with base defense.
		s.Actors[1].Defense = 80
		draws := []float64{0}
		for i := range 4 {
			draws = append(draws, 0) // First, untyped target resolution.
			if row.damage == 0 || row.deaths > 0 && i > 0 {
				draws = append(draws, .99)
			} else {
				draws = append(draws, 0, 0)
			}
		}
		executeAdvancedMagic(t, &s, c, d, "job-19-entry-8", []string{"self", "tail"}, draws...)
		if s.Actors[1].HP != row.hp-row.damage || c.Status.DeathCalls != row.deaths || c.DamageCalls != 4-row.deaths {
			t.Fatal(row, s, c)
		}
	}
}

func TestAdvancedMagicLoopUpperHitAndPowerBounds(t *testing.T) {
	s, c, d := advancedMagicFixture()
	s.Actors[1].HP = 10000
	draws := []float64{math.Nextafter(1, 0)}
	for range 6 {
		draws = append(draws, 0, .999, 0)
	}
	executeAdvancedMagic(t, &s, c, d, "job-19-entry-8", nil, draws...)
	if s.Actors[1].HP != 6766 || c.DamageCalls != 6 || c.Status.DeathCalls != 0 || len(c.Arguments) != 0 {
		t.Fatal(s, c)
	}
}

func TestAdvancedMagicLoopStopsAfterReflectedCasterDeath(t *testing.T) {
	s, c, d := advancedMagicFixture()
	s.Actors[1].Temporary = corebattle.TemporaryReflectMagic
	executeAdvancedMagic(t, &s, c, d, "job-19-entry-8", nil, 0, 0, 0)
	if s.Actors[0].HP != 0 || s.Actors[1].HP != 900 || c.Status.DeathCalls != 1 {
		t.Fatal(s, c)
	}
}

func TestAdvancedMagicLoopRetainsFailedSelectionAndEmptyRoster(t *testing.T) {
	s, c, d := advancedMagicFixture()
	// First death succeeds. Later outer and inner helpers both select the
	// fallen entry, so no further death probability draw is reached.
	executeAdvancedMagic(t, &s, c, d, "job-19-entry-8", nil, 0, 0, 0, 0, 0, 0, 0, 0, 0)
	if s.Actors[1].HP != 0 || c.Status.DeathCalls != 1 {
		t.Fatal(s, c)
	}
	s, c, d = advancedMagicFixture()
	s.Actors = append(s.Actors[:1], s.Actors[2])
	c.EnemyIDs = nil
	executeAdvancedMagic(t, &s, c, d, "job-19-entry-8", nil, 0)
	if c.Status.DeathCalls != 0 || c.DamageCalls != 0 || s.Actors[0].HP != 500 {
		t.Fatal(s, c)
	}
}

func TestAdvancedMagicAreaStatusChanceAndImmunityBoundaries(t *testing.T) {
	for _, row := range []struct {
		id, status string
		chance     float64
	}{
		{"job-15-entry-2", corebattle.StatusSleep, .55}, {"job-16-entry-2", corebattle.StatusMagicSeal, .9},
		{"job-19-entry-2", corebattle.StatusConfusion, .5}, {"job-19-entry-4", corebattle.StatusMagicSeal, .7},
		{"job-19-entry-3", "death", .2}, {"job-19-entry-7", "death", .2},
	} {
		for _, upper := range []bool{false, true} {
			s, c, d := advancedMagicFixture()
			draw := row.chance
			if !upper {
				draw -= .00001
			}
			executeAdvancedMagic(t, &s, c, d, row.id, []string{"enemy"}, draw)
			if row.status == "death" {
				if s.Actors[1].HP != 0 {
					t.Fatal(row.id, s)
				} // Inclusive death threshold.
			} else if (s.Actors[1].Status == row.status) == upper {
				t.Fatal(row.id, upper, s)
			}
		}
		s, c, d := advancedMagicFixture()
		s.Actors[1].BaseStats.Defense = 1000
		executeAdvancedMagic(t, &s, c, d, row.id, []string{"enemy"})
		if s.Actors[1].HP != 900 || s.Actors[1].Status != "" {
			t.Fatal(row.id, s)
		}
	}
}
