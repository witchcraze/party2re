package skill

import (
	"fmt"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func TestSongDanceDrainEntryGuards(t *testing.T) {
	for _, row := range []struct {
		job, pos int
		draws    []float64
	}{
		{13, 0, nil}, {13, 1, nil}, {13, 2, nil}, {13, 3, nil}, {13, 4, nil}, {13, 5, nil},
		{14, 0, []float64{.5}}, {14, 1, []float64{.5}}, {14, 2, nil}, {14, 3, []float64{.5}},
		{14, 5, []float64{0, .5, .5}}, {14, 6, []float64{.5}},
		{20, 0, []float64{.5}}, {20, 1, []float64{.5}}, {20, 2, nil}, {20, 3, []float64{.35}},
		{20, 4, []float64{.5}}, {20, 5, nil}, {20, 6, []float64{.5}},
	} {
		id := fmt.Sprintf("job-%d-entry-%d", row.job, row.pos)
		t.Run(id, func(t *testing.T) {
			s, c, d := songDanceFixture()
			s.Actors[0].Status = corebattle.StatusDanceSeal
			switch id {
			case "job-13-entry-0":
				s.Actors[1].MP = 20 // Native MP floor is20% of MaxMP.
			case "job-13-entry-1":
				s.Actors[0].HP = 0
			case "job-13-entry-2":
				s.Actors[0].Status = corebattle.StatusPoison
			case "job-13-entry-3":
				s.Actors[0].Defense = 125 // BaseDF×2.5 cap.
			case "job-13-entry-4", "job-20-entry-5":
				s.Actors[0].Status = corebattle.StatusMagicSeal
				s.Actors[1].HP = 1000 // Immune; no defeat context is needed.
				c.Status.Defeat = nil
			case "job-13-entry-5":
				s.Actors[0].Attack = 250
			case "job-14-entry-3", "job-20-entry-1", "job-20-entry-6":
				s.Actors[0].Status = corebattle.StatusPhysicalSeal
			case "job-20-entry-2", "job-20-entry-3":
				s.Actors[0].Status = corebattle.StatusMagicSeal
			}
			a, e := s.Actors[0], s.Actors[1]
			executeSongDance(t, &s, c, d, id, []string{"enemy"}, row.draws...)
			wantTemporary := a.Temporary
			if id == "job-14-entry-2" {
				wantTemporary = corebattle.TemporaryDeflect
			}
			if id == "job-20-entry-2" {
				wantTemporary = corebattle.TemporaryReduceMagic
			}
			if s.Actors[0].HP != a.HP || s.Actors[0].MP != a.MP || s.Actors[0].Attack != a.Attack || s.Actors[0].Defense != a.Defense || s.Actors[0].Agility != a.Agility || s.Actors[0].Status != a.Status || s.Actors[0].Temporary != wantTemporary || s.Actors[1].HP != e.HP || s.Actors[1].MP != e.MP || s.Actors[1].Status != "" {
				t.Fatal(s.Actors, a, e)
			}
		})
	}
}

func TestSongDanceDrainCategoriesAndProbabilities(t *testing.T) {
	for _, seal := range []string{corebattle.StatusMagicSeal, corebattle.StatusDanceSeal, corebattle.StatusPhysicalSeal} {
		s, c, d := songDanceFixture()
		s.Actors[0].Status = seal
		executeSongDance(t, &s, c, d, "job-13-entry-1", nil, 0, 0)
		if s.Actors[0].HP != 554 || s.Actors[0].Status != seal {
			t.Fatal(seal, s)
		}
	}
	for _, row := range []struct {
		id     string
		chance float64
		death  bool
	}{
		{"job-13-entry-4", .5, false}, {"job-20-entry-0", .85, false},
		{"job-20-entry-3", .35, false}, {"job-20-entry-4", .6, false}, {"job-20-entry-5", .17, true},
	} {
		for _, draw := range []float64{row.chance - .000001, row.chance, row.chance + .000001} {
			s, c, d := songDanceFixture()
			executeSongDance(t, &s, c, d, row.id, []string{"enemy"}, draw)
			applied := s.Actors[1].Status != ""
			if row.death {
				applied = s.Actors[1].HP == 0
			}
			if applied != (draw < row.chance || row.death && draw == row.chance) {
				t.Fatal(row, draw, s)
			}
		}
	}
	// Song's30-power heal retains the existing below50 instrument boost.
	s, c, d := songDanceFixture()
	s.Actors[0].ItemDefinitionIDs = []string{"item-148"}
	executeSongDance(t, &s, c, d, "job-13-entry-1", nil, 0, 0)
	if s.Actors[0].HP != 1000 {
		t.Fatal(s)
	}
}

func TestSongDanceDrainReturnedDamageAndFloors(t *testing.T) {
	for _, row := range []struct {
		id       string
		hp, mp   int
		excluded bool
	}{
		{"job-20-entry-1", 500, 84, false}, {"job-20-entry-6", 534, 80, false},
		{"job-20-entry-1", 500, 80, true},
	} {
		s, c, d := songDanceFixture()
		s.Actors[1].HP = 1 // Returned overkill40/76 exceeds actual HP loss1.
		if row.excluded {
			s.Actors[0].ItemDefinitionIDs = []string{"item-189"}
		}
		executeSongDance(t, &s, c, d, row.id, []string{"enemy"}, 0, 0)
		if s.Actors[0].HP != row.hp || s.Actors[0].MP != row.mp || s.Actors[1].HP != 0 {
			t.Fatal(row, s)
		}
	}
	for _, id := range []string{"job-20-entry-1", "job-20-entry-6"} {
		s, c, d := songDanceFixture()
		s.Actors[1].Temporary = corebattle.TemporaryOneHitDefense
		executeSongDance(t, &s, c, d, id, []string{"enemy"})
		if c.HealCalls != 0 || c.MPHealCalls != 0 || s.Actors[1].Temporary != "" {
			t.Fatal(c, s)
		}
	}
	for _, id := range []string{"job-13-entry-0", "job-14-entry-1"} {
		s, c, d := songDanceFixture()
		s.Actors[1].MP, s.Actors[1].MaxMP = 201, 1000
		executeSongDance(t, &s, c, d, id, []string{"enemy"})
		if s.Actors[1].MP != 200 || s.Actors[0].Tension != 1 {
			t.Fatal(s)
		}
		s, c, d = songDanceFixture()
		s.Actors[1].MP, s.Actors[1].MaxMP = 1000, 1000
		executeSongDance(t, &s, c, d, id, []string{"enemy"}, .5)
		if s.Actors[1].MP != 875 { // Random capped requested debit125.
			t.Fatal(s)
		}
	}
}

func TestSongDanceDrainDanceOrderAndDeath(t *testing.T) {
	for _, count := range []int{2, 3, 4} {
		s, c, d := songDanceFixture()
		s.Actors[0].Tension = 2
		draws := []float64{float64(count-2) / 3}
		for range count {
			draws = append(draws, 0, 0) // Fresh absent-target selection, then damage.
		}
		executeSongDance(t, &s, c, d, "job-14-entry-5", []string{"fallen", "tail"}, draws...)
		if s.Actors[1].HP != 900-36*(count+1) || c.DamageCalls != count || s.Actors[0].Tension != 1 || len(c.Arguments) != 2 {
			t.Fatal(count, s, c)
		}
	}
	s, c, d := songDanceFixture()
	s.Actors[0].HP, s.Actors[0].Status = 1, corebattle.StatusConfusion
	// Confusion selects the caster; first damage kills them before later hits.
	c.Defeat = func(id string) error {
		if id != "self" {
			t.Fatal(id)
		}
		return nil
	}
	executeSongDance(t, &s, c, d, "job-14-entry-5", nil, .99, 0, 0)
	if s.Actors[0].HP != 0 || c.DamageCalls != 1 || s.Actors[1].HP != 900 {
		t.Fatal(s, c)
	}
}
