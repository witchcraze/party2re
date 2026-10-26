package skill

import (
	"fmt"
	"strings"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func existingCoverClone(s *corebattle.LifetimeState, c *corebattle.BeastCloneContext, hp int) {
	s.Actors = append(s.Actors, corebattle.LifetimeActor{Participant: corebattle.Participant{ID: "clone", TeamID: "party", HP: hp, MaxHP: 500, Gear: &corebattle.BattleGearStats{}}, RosterIdentity: "cover-clone", Tension: 1})
	c.PartyIDs = append(c.PartyIDs, "clone")
}

func TestBeastCloneEveryPositionGuard(t *testing.T) {
	_, _, catalog := beastCloneFixture()
	for id := range catalog.Invocations.Entries {
		t.Run(id, func(t *testing.T) {
			s, c, d := beastCloneFixture()
			a, e := &s.Actors[0], &s.Actors[1]
			args, draws := []string{"enemy", "tail"}, []float64(nil)
			switch id {
			case "job-10-entry-0", "job-12-entry-0", "job-10-entry-3", "job-12-entry-1", "job-12-entry-3", "job-12-entry-6", "job-26-entry-0", "job-12-entry-4":
				e.Temporary, e.Gear = corebattle.TemporaryOneHitDefense, nil
				c.Defeat = nil
			case "job-10-entry-1":
				a.HP = 0
			case "job-10-entry-2", "job-10-entry-7":
				args[0], a.Defense = "self", 200
			case "job-10-entry-4":
				args[0], a.HP = "self", 1000
				draws = []float64{0}
			case "job-10-entry-5", "job-12-entry-2", "job-12-entry-5", "job-26-entry-1", "job-26-entry-3", "job-26-entry-5":
				e.HP, e.Gear = 1000, nil
				c.StatusDefeat = nil
			case "job-10-entry-6":
				a.Status = corebattle.StatusMagicSeal
				draws = []float64{.25}
			case "job-10-entry-8":
				a.HP = 0
				draws = []float64{0}
			case "job-12-entry-7":
				a.HP = 0
			case "job-26-entry-2":
				e.Hit = 50
			case "job-26-entry-4":
				a.Agility = 100
			case "job-26-entry-6":
				a.Agility = 100
			case "job-26-entry-7":
				e.HP = 0
			case "job-26-entry-8":
				existingCoverClone(&s, c, 1)
				c.RequesterLevel = nil
			}
			beforeA, beforeE := s.Actors[0], s.Actors[1]
			executeBeastClone(t, &s, c, d, id, args, draws...)
			a, e = &s.Actors[0], &s.Actors[1]
			if a.HP != beforeA.HP || e.HP != beforeE.HP || a.Defense != beforeA.Defense || a.Agility != beforeA.Agility || e.Hit != beforeE.Hit || c.End.ImmediateRiskCalls != 0 {
				t.Fatal(a, e, c)
			}
			if id == "job-10-entry-7" && a.Temporary != corebattle.TemporaryReduceMagic {
				t.Fatal("unguarded guard tail lost", a)
			}
			if id == "job-26-entry-8" && (a.Temporary != corebattle.TemporaryCover || s.Actors[3].Temporary != corebattle.TemporaryCovering) {
				t.Fatal("cover must precede early return", s)
			}
		})
	}
}

func TestBeastOpeningHPAndReturnedRecoil(t *testing.T) {
	for _, id := range []string{"job-10-entry-0", "job-12-entry-0"} {
		for _, row := range []struct {
			hp, damage, remaining, risk int
			draws                       []float64
		}{
			{0, 0, 0, 0, nil}, {1, 1, 1, 1, []float64{0, 0}},
			{999, 337, 965, 1, []float64{0}}, {1000, 337, 966, 1, []float64{0}},
		} {
			t.Run(fmt.Sprintf("%s/hp%d", id, row.hp), func(t *testing.T) {
				s, c, d := beastCloneFixture()
				s.Actors[0].HP = row.hp
				executeBeastClone(t, &s, c, d, id, []string{"enemy"}, row.draws...)
				if s.Actors[0].HP != row.remaining || 900-s.Actors[1].HP != row.damage || c.End.ImmediateRiskCalls != row.risk {
					t.Fatal(s, c)
				}
			})
		}
	}
	// Recoil uses the returned damage, including amplification and item160 cure.
	s, c, d := beastCloneFixture()
	s.Actors[0].ItemDefinitionIDs = []string{"item-160"}
	s.Actors[0].Status = corebattle.StatusSleep
	s.Actors[1].Temporary = corebattle.TemporaryDouble
	executeBeastClone(t, &s, c, d, "job-10-entry-0", []string{"enemy"}, 0, 0)
	if s.Actors[1].HP != 563 || s.Actors[0].HP != 534 || s.Actors[0].Status != "" || s.Actors[0].Attack != 170 {
		t.Fatal(s)
	}
}

func TestCoverCloneCreationRequesterBoundaries(t *testing.T) {
	for _, row := range []struct{ level, form, hp, at, df, ag int }{
		{39, 0, 92, 315, 45, 135}, {40, 1, 112, 360, 162, 72},
		{69, 1, 190, 360, 162, 72}, {70, 2, 225, 405, 180, 45},
	} {
		for _, bonus := range []bool{false, true} {
			t.Run(fmt.Sprintf("lv%d/bonus%t", row.level, bonus), func(t *testing.T) {
				s, c, d := beastCloneFixture()
				c.RequesterLevel = &row.level
				if bonus {
					c.RequesterItemID = "item-165"
				}
				s.Actors[0].Status = corebattle.StatusDeadlyPoison
				// The current effect actor's unrelated stats do not choose the form.
				s.Actors[0].MaxHP = 9999
				executeBeastClone(t, &s, c, d, "job-26-entry-8", nil, 0, 0, 0, 0, 0)
				h := s.Actors[3]
				values := [5]int{row.hp, 450, row.at, row.df, row.ag}
				if bonus {
					for i, v := range values {
						values[i] = int(float64(v) * 1.2)
					}
				}
				if [5]int{h.HP, h.MP, h.Attack, h.Defense, h.Agility} != values || h.SummonForm != row.form || h.Status != corebattle.StatusDeadlyPoison || s.Actors[0].Status != "" || h.BaseStats.Attack != h.Attack || h.ExperienceReward != row.level/2 || h.GoldReward != row.level/4 || !strings.HasPrefix(h.Icon, "assets/images/placeholders/battle/helpers.svg#cover-clone-") {
					t.Fatal(h, s.Actors[0])
				}
			})
		}
	}
}

func TestCoverClonePresenceAndHPTransfer(t *testing.T) {
	for _, hp := range []int{0, 1, 999, 1000} {
		for _, cloneHP := range []int{0, 1, 2, 500} {
			t.Run(fmt.Sprintf("actor%d/clone%d", hp, cloneHP), func(t *testing.T) {
				s, c, d := beastCloneFixture()
				s.Actors[0].HP = hp
				s.Actors[0].Status = corebattle.StatusSleep
				existingCoverClone(&s, c, cloneHP)
				c.RequesterLevel = nil
				executeBeastClone(t, &s, c, d, "job-26-entry-8", nil)
				a, h := s.Actors[0], s.Actors[3]
				want := 0
				if hp < 1000 && cloneHP > 1 {
					want = min(1000-hp, cloneHP-1)
				}
				if len(s.Actors) != 4 || s.RosterSequence != 0 || a.HP != hp+want || h.HP != cloneHP-want || a.HP+h.HP != hp+cloneHP || a.Status != corebattle.StatusSleep || h.Status != "" || a.Temporary != corebattle.TemporaryCover || h.Temporary != corebattle.TemporaryCovering {
					t.Fatal(a, h)
				}
			})
		}
	}
}

func TestBeastStatusProbabilityBoundaries(t *testing.T) {
	for _, row := range []struct {
		id, status string
		chance     float64
		death      bool
	}{
		{"job-10-entry-5", corebattle.StatusSleep, .45, false},
		{"job-12-entry-2", corebattle.StatusDeadlyPoison, .6, false},
		{"job-12-entry-4", corebattle.StatusParalyze, .4, false},
		{"job-12-entry-5", corebattle.StatusDofuu, .85, false},
		{"job-26-entry-1", corebattle.StatusParalyze, .35, false},
		{"job-26-entry-3", corebattle.StatusDeadlyPoison, .6, false},
		{"job-26-entry-5", "", .19, true},
	} {
		for _, delta := range []float64{-.001, 0, .001} {
			t.Run(fmt.Sprintf("%s/%g", row.id, delta), func(t *testing.T) {
				s, c, d := beastCloneFixture()
				draws := []float64{row.chance + delta}
				if row.id == "job-12-entry-4" {
					draws = append([]float64{0}, draws...)
				}
				executeBeastClone(t, &s, c, d, row.id, []string{"enemy"}, draws...)
				success := delta < 0 || row.death && delta == 0
				if row.death {
					if (s.Actors[1].HP == 0) != success {
						t.Fatal(s)
					}
				} else if (s.Actors[1].Status == row.status) != success {
					t.Fatal(s)
				}
			})
		}
	}
}

func TestBeastStatusHPImmunityBoundaries(t *testing.T) {
	for _, id := range []string{"job-10-entry-5", "job-12-entry-2", "job-12-entry-5", "job-26-entry-1", "job-26-entry-3", "job-26-entry-5"} {
		for _, hp := range []int{0, 1, 999, 1000} {
			t.Run(fmt.Sprintf("%s/hp%d", id, hp), func(t *testing.T) {
				s, c, d := beastCloneFixture()
				s.Actors[1].HP = hp
				draws := []float64(nil)
				effective := hp > 0 && hp <= 999
				if effective {
					draws = []float64{0}
				}
				executeBeastClone(t, &s, c, d, id, []string{"enemy"}, draws...)
				if id == "job-26-entry-5" {
					if (s.Actors[1].HP == 0) != (effective || hp == 0) {
						t.Fatal(s)
					}
				} else if (s.Actors[1].Status != "") != effective {
					t.Fatal(s)
				}
			})
		}
	}
}
