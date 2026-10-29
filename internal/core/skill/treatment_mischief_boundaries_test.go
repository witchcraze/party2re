package skill

import (
	"errors"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func TestTreatmentMischiefArmorReturnBoundary(t *testing.T) {
	for _, bad := range []string{"seal", "darkness", "zero-base", "zero-helper", "additional-cap", "fallen-confusion"} {
		t.Run(bad, func(t *testing.T) {
			s, c, d := treatmentFixture()
			switch bad {
			case "seal":
				s.Actors[0].Status = corebattle.StatusMagicSeal
			case "darkness":
				s.Fields.Temporary = "darkness"
			case "zero-base":
				s.Actors[0].BaseStats.Defense, s.Actors[0].Defense = 0, -1
			case "zero-helper":
				s.Actors[0].Temporary = corebattle.TemporaryNullMagic
			case "additional-cap":
				s.Actors[0].Defense, c.AdditionalEffect = 200, true
			case "fallen-confusion":
				s.Actors[0].Status = corebattle.StatusConfusion
			}
			f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"self"}}
			if _, err := d.InvocationAction(&s, &f, []string{"job-50-entry-3"}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
				t.Fatal(err)
			}
			if c.StatUpCalls != 0 || c.TensionCalls != 0 {
				t.Fatal(c)
			}
		})
	}
	// Binding returns the recipient even though it prevents the defense buff.
	s, c, d := treatmentFixture()
	s.Actors[0].Status = corebattle.StatusSabaku
	executeTreatment(t, &s, c, d, "job-50-entry-3", []string{"self"})
	if s.Actors[0].Defense != 80 || s.Actors[0].Temporary != corebattle.TemporaryReduceBreath {
		t.Fatal(s.Actors[0])
	}
}

func TestTreatmentMischiefReturnedDamageAndMPDrain(t *testing.T) {
	for _, hp := range []int{1, 900} {
		s, c, d := treatmentFixture()
		s.Actors[1].HP = hp
		executeTreatment(t, &s, c, d, "job-51-entry-7", []string{"enemy"}, 0, 0)
		// Requested126 damage, not actual HP lost, yields floor(126*.05)=6,
		// then MP helper floor(6*.9)+1=6.
		if s.Actors[0].MP != 86 || c.MPHealCalls != 1 {
			t.Fatal(s.Actors[0], c)
		}
	}
	for _, id := range []string{"job-55-entry-1", "job-57-entry-5"} {
		for _, mp := range []int{0, 20, 40} {
			s, c, d := treatmentFixture()
			s.Actors[0].MP, s.Actors[0].Tension, s.Actors[1].MP = 0, 8, mp
			draws := []float64(nil)
			if mp == 40 {
				draws = []float64{0}
			}
			executeTreatment(t, &s, c, d, id, []string{"enemy"}, draws...)
			wantMP, wantEnemy := 0, mp
			if mp == 40 {
				wantMP, wantEnemy = 44, 20
			}
			if s.Actors[0].MP != wantMP || s.Actors[1].MP != wantEnemy {
				t.Fatal(id, s.Actors)
			}
		}
	}
}

func TestTreatmentMischiefRandomMembersAndTension(t *testing.T) {
	s, c, d := treatmentFixture()
	s.Actors[2].Tension = 5
	for range 2 {
		executeTreatment(t, &s, c, d, "job-55-entry-0", nil, .7)
	}
	if s.Actors[2].Hit != 24 || s.Actors[2].HP != 0 {
		t.Fatal(s.Actors[2])
	}
	executeTreatment(t, &s, c, d, "job-55-entry-2", nil, .7)
	if s.Actors[2].Status != corebattle.StatusMagicSeal || s.Actors[2].Tension != 1 {
		t.Fatal(s.Actors[2])
	}
	// An explicit fallen ally bypasses the enemy resolver in55.5.
	c.EnemyIDs = nil
	s.Actors = append(s.Actors[:1], s.Actors[2])
	executeTreatment(t, &s, c, d, "job-55-entry-5", []string{"fallen"})
	if s.Actors[1].Tension != 1.7 || s.Actors[1].HP != 0 {
		t.Fatal(s.Actors[1])
	}
	// Missing enemy resolution falls back to self for the tension operation.
	executeTreatment(t, &s, c, d, "job-55-entry-5", []string{"missing"})
	if s.Actors[0].Tension != 1.7 {
		t.Fatal(s.Actors[0])
	}
}

func TestTreatmentMischiefFreshStatusChoiceAndProbabilityBoundary(t *testing.T) {
	for _, roll := range []float64{.499, .5} {
		s, c, d := treatmentFixture()
		other := s.Actors[1]
		other.ID = "other"
		s.Actors = append(s.Actors, other)
		c.EnemyIDs = append(c.EnemyIDs, other.ID)
		executeTreatment(t, &s, c, d, "job-51-entry-3", nil, 0, roll, .5, roll)
		first, second := "", ""
		if roll < .5 {
			first, second = corebattle.StatusConfusion, corebattle.StatusSleep
		}
		if s.Actors[1].Status != first || s.Actors[3].Status != second || c.Status.StatusCalls != 2 {
			t.Fatal(s.Actors, c.Status)
		}
	}
}

func TestTreatmentMischiefDeathPrecedesAreaAndArrowLivingGuard(t *testing.T) {
	s, c, d := treatmentFixture()
	executeTreatment(t, &s, c, d, "job-51-entry-6", nil, .1)
	if s.Actors[1].HP != 0 || c.Status.DeathCalls != 1 || c.AreaDamageCalls != 1 || c.DamageCalls != 0 {
		t.Fatal(s.Actors, c)
	}
	for _, id := range []string{"job-57-entry-1", "job-57-entry-3"} {
		s, c, d := treatmentFixture()
		s.Actors[1].HP = 1
		executeTreatment(t, &s, c, d, id, []string{"enemy"}, 0)
		if s.Actors[1].HP != 0 || s.Actors[1].Hit != 95 || s.Actors[1].Status != "" || c.StatDownCalls != 0 || c.Status.StatusCalls != 0 {
			t.Fatal(s.Actors[1], c)
		}
	}
}

func TestTreatmentMischiefHalfRevivalChanceAndRawTails(t *testing.T) {
	for _, roll := range []float64{.499, .5} {
		s, c, d := treatmentFixture()
		s.Actors[2].Status, s.Actors[2].Temporary = corebattle.StatusSleep, corebattle.TemporaryDefense
		executeTreatment(t, &s, c, d, "job-57-entry-4", []string{"fallen"}, roll)
		want := 0
		if roll < .5 {
			want = 50
		}
		if s.Actors[2].HP != want || s.Actors[2].Status != corebattle.StatusSleep || s.Actors[2].Temporary != corebattle.TemporaryDefense {
			t.Fatal(s.Actors[2])
		}
	}
}

func TestTreatmentMischiefArrowCarriesHitCountersIntoStatusAdmission(t *testing.T) {
	s, c, d := treatmentFixture()
	s.Actors[0].ItemDefinitionIDs = []string{"item-239"}
	s.Actors[1].ItemDefinitionIDs = []string{"item-195"}
	s.Actors[2].HP, s.Actors[2].Gear = 100, nil
	// First hit activates confusion. The subsequent helper can select the ally,
	// but item239 zeros status after that hit, so ally gear is unreachable.
	executeTreatment(t, &s, c, d, "job-57-entry-3", []string{"enemy"}, 0, 0, .7, .7, .5)
	if s.Actors[0].Status != corebattle.StatusConfusion || s.Actors[2].Status != "" || c.Status.StatusCalls != 1 || c.DamageCalls != 1 {
		t.Fatal(s.Actors, c)
	}
}

func TestTreatmentMischiefBlockedArrowNeedsNoStatusDescendants(t *testing.T) {
	s, c, d := treatmentFixture()
	s.Actors[1].Temporary, s.Actors[1].Gear, c.Defeat = corebattle.TemporaryOneHitDefense, nil, nil
	executeTreatment(t, &s, c, d, "job-57-entry-3", []string{"enemy"})
	if c.Status.StatusCalls != 0 || s.Actors[1].HP != 900 || s.Actors[1].Temporary != "" {
		t.Fatal(s.Actors, c)
	}
}
