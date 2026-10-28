package skill

import (
	"math"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func TestLateSongBreathAndDanceGuards(t *testing.T) {
	for _, id := range []string{"job-84-entry-0", "job-84-entry-1", "job-84-entry-2"} {
		s, c, d := lateSongFixture()
		s.Actors[0].HP, s.Actors[0].Hit, s.Actors[0].Status = 0, 1, corebattle.StatusPhysicalSeal
		s.Actors[1].Temporary = corebattle.TemporaryOneHitDefense
		var draws []float64
		if id == "job-84-entry-2" {
			draws = []float64{.9}
		}
		executeLateSong(t, &s, c, d, id, []string{"enemy"}, draws...)
		if s.Actors[1].HP != 1000 || s.Actors[0].Status != corebattle.StatusPhysicalSeal {
			t.Fatal(id, s)
		}
		if id == "job-84-entry-0" && (s.Actors[0].Hit != 95 || s.Actors[0].Temporary != corebattle.TemporaryReduceBreath) {
			t.Fatal(s)
		}
	}
	for _, row := range []struct {
		target           string
		draw             float64
		calls, remaining int
		status           string
	}{
		{"self", .25, 0, 1, corebattle.StatusDanceSeal},
		{"self", math.Nextafter(.25, 0), 1, 0, ""},
		{"fallen", math.Nextafter(.25, 0), 1, 0, ""},
		{"ally", math.Nextafter(.25, 0), 2, 0, ""},
	} {
		s, c, d := lateSongFixture()
		s.Actors[0].Status = corebattle.StatusDanceSeal
		if row.target == "ally" {
			s.Actors[2].ID, s.Actors[2].HP, c.PartyIDs[1] = "ally", 10, "ally"
		}
		executeLateSong(t, &s, c, d, "job-84-entry-3", []string{row.target}, row.draw)
		if c.TensionCalls != row.calls || len(c.Arguments) != row.remaining || s.Actors[0].Status != row.status {
			t.Fatal(row, s, c)
		}
		if row.target == "ally" && s.Actors[2].Tension != 1.7 {
			t.Fatal(s)
		}
	}
}

func TestLateSongRitualAndRawCure(t *testing.T) {
	for _, row := range []struct{ hp, wantHP, wantMP int }{{0, 1, 85}, {1, 1, 85}, {10, 1, 88}, {270, 1, 182}, {271, 1, 182}} {
		s, c, d := lateSongFixture()
		s.Actors[0].HP = row.hp
		executeLateSong(t, &s, c, d, "job-85-entry-0", []string{"ignored"}, 0, 0)
		if s.Actors[0].HP != row.wantHP || s.Actors[0].MP != row.wantMP || len(c.Arguments) != 1 {
			t.Fatal(row, s)
		}
	}
	for _, target := range []string{"self", "enemy", "fallen", "missing", ""} {
		s, c, d := lateSongFixture()
		for i := range s.Actors {
			s.Actors[i].Status, s.Actors[i].Temporary = corebattle.StatusSleep, corebattle.TemporaryDouble
		}
		executeLateSong(t, &s, c, d, "job-85-entry-2", []string{target})
		want := target
		if target == "" || target == "missing" {
			want = "self"
		}
		for _, a := range s.Actors {
			if (a.Status == "" && a.Temporary == "") != (a.ID == want) {
				t.Fatal(target, s)
			}
		}
	}
}

func TestLateSongChainProbabilityAndDamage(t *testing.T) {
	for _, draw := range []float64{math.Nextafter(.6, 0), .6} {
		s, c, d := lateSongFixture()
		s.Actors[1].Tension = 8 // Raw chaining ignores status immunity at HP1000.
		executeLateSong(t, &s, c, d, "job-85-entry-3", []string{"enemy"}, draw)
		chained := draw < .6
		if (s.Actors[1].Status == corebattle.StatusSabaku) != chained || chained && s.Actors[1].Tension != 1 {
			t.Fatal(draw, s)
		}
		executeLateSong(t, &s, c, d, "job-85-entry-4", []string{"enemy"}, 0)
		want := 910
		if chained {
			want = 370
		}
		if s.Actors[1].HP != want || c.DamageCalls != 1 {
			t.Fatal(draw, s, c)
		}
	}
	for _, id := range []string{"job-85-entry-3", "job-85-entry-4"} {
		s, c, d := lateSongFixture()
		s.Actors[1].HP = 0
		var draws []float64
		if id == "job-85-entry-4" {
			draws = []float64{0}
		} // Second helper selects again.
		executeLateSong(t, &s, c, d, id, []string{"enemy"}, draws...)
		if s.Actors[1].HP != 0 || s.Actors[1].Status != "" {
			t.Fatal(s)
		}
	}
}

func TestLateSongForbiddenRecoilAfterNoEffect(t *testing.T) {
	for _, guard := range []string{"one-hit", "null", "fallen", "sealed", "no-enemies"} {
		s, c, d := lateSongFixture()
		c.HeldBooks["self"] = "forbidden"
		if guard != "sealed" {
			c.Defeat = nil
		}
		var draws []float64
		switch guard {
		case "one-hit":
			s.Actors[1].Temporary = corebattle.TemporaryOneHitDefense
		case "null":
			s.Actors[1].Temporary = corebattle.TemporaryNullMagic
		case "fallen":
			s.Actors[1].HP = 0
		case "sealed":
			s.Actors[0].Status = corebattle.StatusMagicSeal
			draws = []float64{.9}
		case "no-enemies":
			c.EnemyIDs = nil
			s.Actors[1].TeamID = "party"
			c.PartyIDs = append(c.PartyIDs, "enemy")
		}
		before := s.Actors[1].HP
		executeLateSong(t, &s, c, d, "job-86-entry-4", []string{"enemy"}, draws...)
		if s.Actors[1].HP != before || s.Actors[0].HP != 349 || c.End.ImmediateRiskCalls != 1 {
			t.Fatal(guard, s, c)
		}
	}
}

func TestLateSongDeathImmunityAndChance(t *testing.T) {
	for _, row := range []struct {
		hp    int
		want  int
		draws []float64
	}{
		{1000, 1000, nil}, {999, 999, []float64{.180001}}, {999, 0, []float64{.18}},
	} {
		s, c, d := lateSongFixture()
		s.Actors[1].HP = row.hp
		// Immune death needs no defeat implementation; a possible successful
		// death must still be admitted even when this execution roll will miss.
		if row.hp == 1000 {
			c.Death.Defeat = nil
		}
		executeLateSong(t, &s, c, d, "job-85-entry-1", nil, row.draws...)
		if s.Actors[1].HP != row.want || c.Death.DeathCalls != 1 {
			t.Fatal(row, s, c)
		}
	}
}
