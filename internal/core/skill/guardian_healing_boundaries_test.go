package skill

import (
	"fmt"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func TestGuardianHealingEveryPositionGuard(t *testing.T) {
	_, _, catalog := guardianFixture()
	for id := range catalog.Invocations.Entries {
		t.Run(id, func(t *testing.T) {
			s, c, d := guardianFixture()
			s.Actors[0].Status = corebattle.StatusMagicSeal
			draws := []float64{.9}
			switch id {
			case "job-17-entry-0", "job-17-entry-5":
				s.Actors[0].Status = corebattle.StatusPhysicalSeal
			case "job-18-entry-1":
				s.Actors[0].Status = corebattle.StatusDanceSeal
			case "job-18-entry-2":
				// Song cures sleep even while magic is sealed.
				s.Actors[0].Status, draws = corebattle.StatusSleep, nil
			case "job-18-entry-7":
				// Each single heal has its own seal check, then the zero hit
				// checks once more and still records use after normal return.
				draws = []float64{.9, .9, .9}
			}
			executeGuardian(t, &s, c, d, id, []string{"self", "tail"}, draws...)
			if s.Actors[0].HP != 500 || s.Actors[1].HP != 900 || s.Actors[0].Temporary != "" || c.HealCalls != 0 || c.DamageCalls != 0 || c.StatusCalls != 0 || s.Actors[0].MP != 80 {
				t.Fatal(s, c)
			}
			if id == "job-18-entry-7" && (s.Actors[2].HP != 1 || !s.SkillHistory["party"][id]) {
				t.Fatal(s)
			}
			if id == "job-18-entry-2" && (s.Actors[0].Status != "" || c.PartyCureCalls != 1 || c.CureCalls != 2 || !c.AdditionalEffect) {
				t.Fatal(s, c)
			}
		})
	}
}

func TestGuardianCoverBarrierReflectionAndCureBoundaries(t *testing.T) {
	for _, target := range []string{"self", "fallen", "living"} {
		s, c, d := guardianFixture()
		if target == "living" {
			s.Actors[2].HP, target = 100, "fallen"
		}
		executeGuardian(t, &s, c, d, "job-17-entry-0", []string{target, "tail"})
		wantSelf, wantTarget := corebattle.TemporaryEffect(""), corebattle.TemporaryEffect("")
		if s.Actors[2].HP > 0 {
			wantSelf, wantTarget = corebattle.TemporaryCovering, corebattle.TemporaryCover
		}
		if s.Actors[0].Temporary != wantSelf || s.Actors[2].Temporary != wantTarget {
			t.Fatal(s)
		}
	}
	for _, hp := range []int{0, 1} {
		s, c, d := guardianFixture()
		s.Actors[0].Status = corebattle.StatusMagicSeal
		s.Actors[2].HP, s.Actors[2].Temporary = hp, corebattle.TemporaryDefense
		executeGuardian(t, &s, c, d, "job-17-entry-2", nil, .249999)
		want := corebattle.TemporaryDefense
		if hp > 0 {
			want = corebattle.TemporaryReduceMagic
		}
		if s.Actors[0].Status != "" || s.Actors[0].Temporary != corebattle.TemporaryReduceMagic || s.Actors[2].Temporary != want {
			t.Fatal(s)
		}
	}
	for _, status := range []string{corebattle.StatusPoison, corebattle.StatusDeadlyPoison, corebattle.StatusVirulentPoison, corebattle.StatusSleep} {
		s, c, d := guardianFixture()
		s.Actors[2].HP, s.Actors[2].Status = 100, status
		executeGuardian(t, &s, c, d, "job-18-entry-0", []string{"fallen"})
		want := ""
		if status == corebattle.StatusSleep {
			want = status
		}
		if s.Actors[2].Status != want || c.CureCalls != 1 {
			t.Fatal(s, c)
		}
	}
	// Symbolic reflection redirects the assignment, including from a fallen
	// intended ally which falls back to the caster before its temporary hook.
	for _, hp := range []int{0, 1} {
		s, c, d := guardianFixture()
		s.Actors[2].HP, s.Actors[2].Temporary = hp, corebattle.TemporaryReflectMagic
		executeGuardian(t, &s, c, d, "job-18-entry-3", []string{"fallen"})
		if s.Actors[0].Temporary != corebattle.TemporaryReflectMagic || c.Arguments == nil {
			t.Fatal(s, c)
		}
	}
	for _, row := range []struct {
		draw    float64
		success bool
	}{{.699999, true}, {.7, false}} {
		s, c, d := guardianFixture()
		executeGuardian(t, &s, c, d, "job-18-entry-1", nil, row.draw)
		if (s.Actors[1].Status == corebattle.StatusDanceSeal) != row.success {
			t.Fatal(row, s)
		}
	}
}

func TestGuardianRestorationReturnedAmountCapsAndTension(t *testing.T) {
	for _, maxHP := range []int{999, 1000, 1001, 2000} {
		t.Run(fmt.Sprint(maxHP), func(t *testing.T) {
			s, c, d := guardianFixture()
			s.Actors[0].HP, s.Actors[0].MaxHP, s.Actors[0].Tension = maxHP, maxHP, 5
			s.Actors[2].HP, s.Actors[2].MaxHP = maxHP, maxHP
			power := min(300, int(float64(maxHP)*.3))
			heal := int(float64(power) * .9)
			executeGuardian(t, &s, c, d, "job-18-entry-7", []string{"enemy", "tail"}, 0, 0, 0)
			// Full health still returns the rolled amount, so damage uses
			// twice that amount, never the actual HP delta (which is zero).
			if s.Actors[0].HP != maxHP || s.Actors[2].HP != maxHP || s.Actors[1].HP != 900-int(float64(heal*2)*.9) || s.Actors[0].Tension != 5 || c.HealCalls != 2 || c.PartyHealCalls != 0 {
				t.Fatal(s, c)
			}
		})
	}
	// The single heal30 and party heal120 keep their separate helper counters.
	for _, id := range []string{"job-17-entry-1", "job-18-entry-5"} {
		s, c, d := guardianFixture()
		s.Actors[0].HP, s.Actors[0].Tension = 995, 3
		draws := []float64{0}
		if id == "job-18-entry-5" {
			draws = append(draws, 0)
		}
		executeGuardian(t, &s, c, d, id, []string{"self"}, draws...)
		if s.Actors[0].HP != 1000 || s.Actors[0].Tension != 1 {
			t.Fatal(s)
		}
	}
}

func TestGuardianRestorationRepeatAndDifferentActor(t *testing.T) {
	s, c, d := guardianFixture()
	s.Actors[0].Tension = 4
	executeGuardian(t, &s, c, d, "job-18-entry-7", []string{"enemy", "tail"}, 0, 0, 0)
	// A different actor of the same color still revives/heals, but does not
	// consume the supplied target or execute damage on the second use.
	s.Actors[0].HP, s.Actors[2].HP, s.Actors[2].Tension = 0, 100, 5
	c.ActorID = "fallen"
	executeGuardian(t, &s, c, d, "job-18-entry-7", []string{"enemy", "tail"}, 0, 0)
	if s.Actors[0].HP != 271 || s.Actors[2].HP != 370 || s.Actors[2].Tension != 5 || s.Actors[1].HP != 414 || c.DamageCalls != 1 || len(c.Arguments) != 2 {
		t.Fatal(s, c)
	}
	// Another color has an independent use. Its actor may itself be fallen.
	c.ActorID, c.PartyIDs, c.EnemyIDs = "enemy", []string{"enemy"}, []string{"self", "fallen"}
	s.Actors[1].HP, s.Actors[1].Tension = 0, 3
	executeGuardian(t, &s, c, d, "job-18-entry-7", []string{"self"}, 0, 0)
	if s.Actors[1].HP != 271 || s.Actors[0].HP != 28 || s.Actors[1].Tension != 3 || !s.SkillHistory["enemy"]["job-18-entry-7"] {
		t.Fatal(s, c)
	}
}

func TestGuardianRestorationReflectionKillAndNormalReturn(t *testing.T) {
	s, c, d := guardianFixture()
	s.Actors[0].HP, s.Actors[0].Tension = 1, 6
	s.Actors[1].Temporary = corebattle.TemporaryReflectMagic
	defeated := ""
	c.Defeat = func(id string) error { defeated = id; return nil }
	executeGuardian(t, &s, c, d, "job-18-entry-7", []string{"enemy"}, 0, 0, 0)
	if defeated != "self" || s.Actors[0].HP != 0 || s.Actors[0].Tension != 6 || s.Actors[1].HP != 900 || s.Actors[2].HP != 271 || !s.SkillHistory["party"]["job-18-entry-7"] {
		t.Fatal(defeated, s)
	}
}
