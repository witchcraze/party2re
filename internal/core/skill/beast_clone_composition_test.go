package skill

import (
	"fmt"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/core/random"
)

func TestBeastRepeatedCountAndCapturedTarget(t *testing.T) {
	for _, row := range []struct {
		draw  float64
		count int
	}{{0, 2}, {1.0 / 3, 3}, {2.0 / 3, 4}, {.999, 4}} {
		s, c, d := beastCloneFixture()
		draws := []float64{row.draw}
		for range row.count {
			draws = append(draws, 0, 0)
		} // absent target selection, damage
		executeBeastClone(t, &s, c, d, "job-10-entry-8", []string{"tail"}, draws...)
		if c.DamageCalls != row.count || s.Actors[1].HP != 900-28*row.count || c.Arguments[0] != "tail" {
			t.Fatal(s, c)
		}
	}
	s, c, d := beastCloneFixture()
	s.Actors[1].HP = 10
	other := s.Actors[1]
	other.ID, other.HP = "other", 900
	s.Actors = append(s.Actors, other)
	c.EnemyIDs = append(c.EnemyIDs, "other")
	executeBeastClone(t, &s, c, d, "job-12-entry-7", []string{"enemy", "other"}, 0)
	if s.Actors[1].HP != 0 || s.Actors[3].HP != 900 || c.DamageCalls != 1 || c.Arguments[0] != "other" {
		t.Fatal(s, c)
	}
}

func TestBeastEarlierDamageChangesEligibility(t *testing.T) {
	for _, id := range []string{"job-10-entry-8", "job-12-entry-7"} {
		s, c, d := beastCloneFixture()
		s.Actors[0].HP = 1
		s.Actors[1].Temporary = corebattle.TemporaryReflectAttack
		draws := []float64{0}
		if id == "job-10-entry-8" {
			draws = append([]float64{.99, 0}, draws...)
		}
		executeBeastClone(t, &s, c, d, id, []string{"enemy"}, draws...)
		if s.Actors[0].HP != 0 || c.DamageCalls != 1 {
			t.Fatal(id, s, c)
		}
	}
	for _, hp := range []int{1, 1000} {
		s, c, d := beastCloneFixture()
		s.Actors[1].HP = hp
		draws := []float64{0}
		if hp == 1000 {
			draws = append(draws, 0)
		}
		executeBeastClone(t, &s, c, d, "job-12-entry-4", []string{"enemy"}, draws...)
		if (s.Actors[1].Status == corebattle.StatusParalyze) != (hp == 1000) || c.StatusCalls != map[bool]int{false: 0, true: 1}[hp == 1000] {
			t.Fatal(s, c)
		}
	}
}

func TestBeastIndependentSealsAndArmorReset(t *testing.T) {
	for _, cured := range []bool{false, true} {
		s, c, d := beastCloneFixture()
		s.Actors[0].Status = corebattle.StatusMagicSeal
		draw := .25
		if cured {
			draw = 0
		}
		executeBeastClone(t, &s, c, d, "job-10-entry-6", nil, draw)
		if (s.Actors[0].Temporary == corebattle.TemporaryAbsorbMagic) != cured || (s.Actors[0].Status == "") != cured {
			t.Fatal(s)
		}
	}
	// A failed damage seal returns no target, so the status tail is unreachable.
	s, c, d := beastCloneFixture()
	s.Actors[0].Status = corebattle.StatusPhysicalSeal
	executeBeastClone(t, &s, c, d, "job-12-entry-4", []string{"enemy"}, .25)
	if !c.AdditionalEffect || c.StatusCalls != 0 || s.Actors[1].HP != 900 {
		t.Fatal(s, c)
	}
	for _, tmp := range []corebattle.TemporaryEffect{corebattle.TemporaryOneHitDefense, corebattle.TemporaryReflectAttack} {
		s, c, d := beastCloneFixture()
		s.Actors[1].Defense, s.Actors[1].BaseStats.Defense = 300, 80
		s.Actors[1].Tension = 5
		s.Actors[1].Temporary = tmp
		draws := []float64(nil)
		target := 1
		if tmp == corebattle.TemporaryReflectAttack {
			target = 0
			draws = []float64{0}
		}
		executeBeastClone(t, &s, c, d, "job-26-entry-7", []string{"enemy", "tail"}, draws...)
		if s.Actors[target].Defense != 72 || s.Actors[target].Temporary != "" || s.Actors[target].Tension != 1 || c.StatDownCalls != 1 {
			t.Fatal(s, c)
		}
	}
}

func TestCoverCloneRoleAliasing(t *testing.T) {
	for _, row := range []struct{ hp, want int }{{250, 1}, {251, 500}, {500, 500}, {1, 1}} {
		s, c, d := beastCloneFixture()
		existingCoverClone(&s, c, row.hp)
		c.ActorID = "clone"
		executeBeastClone(t, &s, c, d, "job-26-entry-8", nil)
		if s.Actors[3].HP != row.want || s.Actors[3].Temporary != corebattle.TemporaryCover || len(s.Actors) != 4 {
			t.Fatal(row, s)
		}
	}
}

func TestBeastToolKeepsEveryPositionAndWaivesListedCosts(t *testing.T) {
	for _, job := range []int{10, 12, 26} {
		for pos := range corebattle.BeastCloneEntries(job) {
			t.Run(fmt.Sprintf("%d/%d", job, pos), func(t *testing.T) {
				s, c, d := beastCloneFixture()
				s.Actors[0].Jobs.JobID = fmt.Sprintf("job-%d", job)
				s.Actors[0].Jobs.SP = 0
				s.Actors[0].MP = 0
				entries := d.Invocations.JobTables[job].Variants[0]
				for i := range entries {
					entries[i].Name = "shared-display-alias"
				}
				d.Invocations.Entries["tool"] = corebattle.CallbackDefinition{Invocation: corebattle.CallbackInvocation{Kind: "tool-84"}}
				// Select the concrete source position, with independent entry binding.
				selected := entries[pos].ID
				executed := false
				body := d.Invocations.Entries[selected]
				original := body.Execute
				body.Execute = func(s *corebattle.LifetimeState, f *corebattle.CallbackContext, rng random.Generator) error {
					executed = true
					return original(s, f, rng)
				}
				d.Invocations.Entries[selected] = body
				f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy", "tail"}}
				ops, err := d.InvocationAction(&s, &f, []string{"tool"}, noArithmeticDraws{t})
				if err != nil {
					t.Fatal(err)
				}
				draws := append([]float64{(float64(pos) + .5) / float64(len(entries))}, make([]float64, 20)...)
				if err := s.ApplyInvocations(&f, d.Invocations, ops, &damageGemDraws{t: t, draws: draws}); err != nil {
					t.Fatal(err)
				}
				if !executed || s.Actors[0].MP != 0 || s.Actors[0].CMP != 90 {
					t.Fatal(selected, executed, s)
				}
			})
		}
	}
}
