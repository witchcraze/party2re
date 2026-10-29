package skill

import (
	"fmt"
	"math"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func TestRitualDanceStripAndSacrificeBoundaries(t *testing.T) {
	for _, row := range []struct{ hp, want int }{{1, 1}, {24995, 19996}, {25000, 20000}, {25001, 20001}} {
		s, c, d := ritualDanceFixture()
		s.Actors[1].HP, s.Actors[1].Temporary = row.hp, corebattle.TemporaryDefense
		executeRitualDance(t, &s, c, d, "job-75-entry-1", []string{"enemy"})
		if s.Actors[1].HP != row.want || s.Actors[1].Temporary != "" || c.DamageCalls != 0 {
			t.Fatal(row, s, c)
		}
	}
	for _, row := range []struct {
		hp, wantHP, wantMP int
		draw               float64
	}{
		{1, 1, 85, 0}, {2, 1, 85, 0}, {200, 1, 157, 0},
		{500, 230, 182, 0}, {500, 140, 215, math.Nextafter(1, 0)},
	} {
		s, c, d := ritualDanceFixture()
		s.Actors[0].HP = row.hp
		executeRitualDance(t, &s, c, d, "job-75-entry-4", nil, row.draw, 0)
		if s.Actors[0].HP != row.wantHP || s.Actors[0].MP != row.wantMP || c.MPHealCalls != 1 {
			t.Fatal(row, s.Actors[0], c)
		}
	}
	// Exclusions occur after the sacrifice, and still consume tension/rage.
	s, c, d := ritualDanceFixture()
	s.Actors[0].ItemDefinitionIDs, s.Actors[0].Temporary, s.Actors[0].Tension = []string{"item-189"}, corebattle.TemporaryRage, 3
	executeRitualDance(t, &s, c, d, "job-75-entry-4", nil, 0, 0)
	if s.Actors[0].HP != 230 || s.Actors[0].MP != 80 || s.Actors[0].Tension != 1 || s.Actors[0].Temporary != "" {
		t.Fatal(s)
	}
}

func TestRitualDanceTransferAccuracyAndProtection(t *testing.T) {
	// All members, including the original target and fallen allies, are eligible.
	for _, row := range []struct {
		draw  float64
		index int
	}{{0, 0}, {.4, 1}, {.9, 2}} {
		s, c, d := ritualDanceFixture()
		s.Actors[0].Status, s.Actors[0].Tension = corebattle.StatusPoison, 5
		executeRitualDance(t, &s, c, d, "job-75-entry-3", []string{"self", "tail"}, row.draw)
		if s.Actors[row.index].Status != corebattle.StatusPoison || row.index != 0 && s.Actors[0].Status != "" || s.Actors[0].Tension != 5 || len(c.Arguments) != 1 || c.Arguments[0] != "tail" {
			t.Fatal(row, s, c)
		}
	}
	s, c, d := ritualDanceFixture()
	s.Actors[0].Hit = 99
	executeRitualDance(t, &s, c, d, "job-75-entry-2", []string{"fallen"})
	if s.Actors[0].Hit != 95 || s.Actors[2].Hit != 99 {
		t.Fatal(s)
	}
	for _, row := range []struct {
		draw float64
		tmp  corebattle.TemporaryEffect
	}{{0, corebattle.TemporaryNullAttack}, {.5, corebattle.TemporaryReduceAttack}} {
		s, c, d := ritualDanceFixture()
		executeRitualDance(t, &s, c, d, "job-76-entry-4", []string{"self"}, row.draw)
		if s.Actors[0].Temporary != row.tmp || c.CureCalls != 0 {
			t.Fatal(s, c)
		}
	}
}

func TestRitualDanceCurseBranchesAndGuards(t *testing.T) {
	for _, row := range []struct {
		status, weak float64
		want         string
	}{{0, .5, corebattle.StatusKinju}, {.9, 0, corebattle.StatusDofuu}} {
		s, c, d := ritualDanceFixture()
		s.Actors[1].HP, s.Actors[1].BaseStats.Defense, s.Actors[1].Tension = 5000, 5000, 5
		executeRitualDance(t, &s, c, d, "job-75-entry-6", []string{"enemy"}, 0, row.status, row.weak)
		if s.Actors[1].Status != row.want || s.Actors[1].Tension != 1 || (row.weak == 0) != (s.Actors[1].Temporary == corebattle.TemporaryDouble) {
			t.Fatal(row, s)
		}
	}
	s, c, d := ritualDanceFixture()
	s.Actors[1].Tension = 5
	executeRitualDance(t, &s, c, d, "job-75-entry-6", []string{"enemy"}, .5, .5)
	if s.Actors[1].Status != "" || s.Actors[1].Temporary != "" || s.Actors[1].Tension != 5 {
		t.Fatal(s)
	}
	// The outer category guard runs before shifting the supplied target.
	s.Actors[0].Status = corebattle.StatusMagicSeal
	executeRitualDance(t, &s, c, d, "job-75-entry-6", []string{"enemy", "tail"}, .25)
	if len(c.Arguments) != 2 || s.Actors[0].Status != corebattle.StatusMagicSeal {
		t.Fatal(c, s)
	}
	// A successful outer cure suppresses a second category-cure draw.
	executeRitualDance(t, &s, c, d, "job-75-entry-6", []string{"enemy"}, 0, .5, .5)
	if s.Actors[0].Status != "" {
		t.Fatal(s)
	}
	for i, status := range []string{corebattle.StatusMagicSeal, corebattle.StatusPhysicalSeal, corebattle.StatusKinju} {
		s, c, d := ritualDanceFixture()
		executeRitualDance(t, &s, c, d, "job-75-entry-0", nil, (float64(i)+.1)/3, .749)
		if s.Actors[1].Status != status || c.AreaStatusCalls != 0 {
			t.Fatal(s, c)
		}
	}
	for _, row := range []struct {
		id        string
		threshold float64
	}{{"job-75-entry-0", .75}, {"job-76-entry-1", .5}} {
		s, c, d := ritualDanceFixture()
		draws := []float64{row.threshold}
		if row.id == "job-75-entry-0" {
			draws = append([]float64{0}, draws...)
		}
		executeRitualDance(t, &s, c, d, row.id, nil, draws...)
		if s.Actors[1].Status != "" {
			t.Fatal(row, s)
		}
	}
}

func TestRitualDanceDrainAndHitBoundaries(t *testing.T) {
	for _, row := range []struct {
		draw     float64
		hp, self int
	}{{0, 765, 645}, {math.Nextafter(1, 0), 585, 840}} {
		s, c, d := ritualDanceFixture()
		executeRitualDance(t, &s, c, d, "job-76-entry-2", []string{"enemy"}, row.draw, 0, 0)
		if s.Actors[1].HP != row.hp || s.Actors[0].HP != row.self {
			t.Fatal(row, s)
		}
	}
	// Healing uses requested damage, including overkill, and follows reflection.
	s, c, d := ritualDanceFixture()
	s.Actors[1].HP = 1
	executeRitualDance(t, &s, c, d, "job-76-entry-2", []string{"enemy"}, 0, 0, 0)
	if s.Actors[1].HP != 0 || s.Actors[0].HP != 645 {
		t.Fatal(s)
	}
	s, c, d = ritualDanceFixture()
	s.Actors[1].Temporary = corebattle.TemporaryReflectMagic
	executeRitualDance(t, &s, c, d, "job-76-entry-2", []string{"enemy"}, 0, 0, 0)
	if s.Actors[1].HP != 900 || s.Actors[0].HP != 510 {
		t.Fatal(s)
	}
	for _, hit := range []int{49, 50, 51, 95} {
		t.Run(fmt.Sprint(hit), func(t *testing.T) {
			s, c, d := ritualDanceFixture()
			s.Actors[1].Hit = hit
			s.Actors[0].Tension = 3
			executeRitualDance(t, &s, c, d, "job-76-entry-0", nil)
			want := hit
			if hit > 50 {
				want = max(50, hit-int(float64(hit)*.2*3))
			}
			if s.Actors[1].Hit != want || (hit > 50 && s.Actors[0].Tension != 1) {
				t.Fatal(hit, s)
			}
		})
	}
}

func TestRitualDanceGroupBranches(t *testing.T) {
	for _, row := range []struct {
		supplied, direct bool
		damage           int
		tension          float64
	}{
		{false, true, 90, 3}, {false, false, 54, 3}, {true, true, 216, 1}, {true, false, 135, 1},
	} {
		s, c, d := ritualDanceFixture()
		s.Actors[0].Tension = 3
		args, branch := []string{"missing"}, .25
		if row.supplied {
			args = []string{"fallen"}
		}
		if row.direct {
			branch = math.Nextafter(.25, 0)
		}
		draws := []float64{branch, 0, 0}
		executeRitualDance(t, &s, c, d, "job-76-entry-5", args, draws...)
		if 900-s.Actors[1].HP != row.damage || s.Actors[0].Tension != row.tension || c.DamageCalls != 1 || c.ActorID != "self" {
			t.Fatal(row, s, c)
		}
	}
	// In the supplied branch the loop's ally shadows the original target.
	s, c, d := ritualDanceFixture()
	s.Actors[0].Status, s.Actors[0].Tension = corebattle.StatusConfusion, 3
	executeRitualDance(t, &s, c, d, "job-76-entry-5", []string{"enemy"}, 0, 0)
	if s.Actors[0].HP != 284 || s.Actors[1].HP != 900 || s.Actors[0].Tension != 1 {
		t.Fatal(s)
	}
	// A living ally acts with its own stats/role and source party order.
	s, c, d = ritualDanceFixture()
	s.Actors[2].HP, s.Actors[2].Attack, s.Actors[2].Tension, s.Actors[2].NPC = 100, 200, 5, true
	executeRitualDance(t, &s, c, d, "job-76-entry-5", nil, 0, 0, 0, .25, 0, 0)
	if s.Actors[1].HP != 702 || s.Actors[2].Tension != 5 || c.ActorNPC || c.DamageCalls != 2 {
		t.Fatal(s, c)
	}
}
