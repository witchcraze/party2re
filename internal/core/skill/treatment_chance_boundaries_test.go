package skill

import (
	"fmt"
	"math"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func TestTreatmentChanceAllSymbolTuples(t *testing.T) {
	for x := range 5 {
		for y := range 5 {
			for z := range 5 {
				t.Run(fmt.Sprintf("%d%d%d", x, y, z), func(t *testing.T) {
					s, c, d := chanceFixture()
					draws := []float64{(float64(x) + .1) / 5, (float64(y) + .1) / 5, (float64(z) + .1) / 5}
					wantHP, wantEnemy := 500, 900
					heal, damage, death := 0, 1, 0
					if x == y && y == z {
						switch x {
						case 0, 2:
							heal, damage = 1, 0
							// Fallen ally falls back to self, so two heals draw.
							wantHP = min(1000, 500+(x+2)*180)
							draws = append(draws, 0, 0)
						case 4:
							wantEnemy, damage, death = 0, 0, 1
							draws = append(draws, 0)
						default:
							wantEnemy -= int((float64((x+2)*100)*.5 - 24) * .9)
							draws = append(draws, 0)
						}
					} else {
						wantEnemy -= int(float64((x+y+z)*7) * .9)
						draws = append(draws, 0)
					}
					executeChance(t, &s, c, d, "job-46-entry-0", nil, draws...)
					if s.Actors[0].HP != wantHP || s.Actors[1].HP != wantEnemy || c.PartyHealCalls != heal || c.AreaDamageCalls != damage || c.Status.AreaDeathCalls != death {
						t.Fatal(x, y, z, s.Actors, c)
					}
				})
			}
		}
	}
}

func TestTreatmentChanceTreatmentGuards(t *testing.T) {
	for _, id := range []string{"job-43-entry-0", "job-43-entry-1", "job-43-entry-2", "job-43-entry-4"} {
		s, c, d := chanceFixture()
		// A valid cure with no matching status is accepted, including empty cure.
		executeChance(t, &s, c, d, id, []string{"self"})
		if s.Actors[0].HP != 500 || s.Actors[0].Status != "" {
			t.Fatal(id, s.Actors)
		}
	}
	for _, id := range []string{"job-43-entry-3", "job-45-entry-5", "job-45-entry-0"} {
		s, c, d := chanceFixture()
		executeChance(t, &s, c, d, id, []string{"fallen"})
		if id == "job-45-entry-0" && s.Actors[0].Hit != 95 || id != "job-45-entry-0" && s.Actors[0].Temporary != corebattle.TemporaryRegeneration || s.Actors[2].Temporary != "" {
			t.Fatal(id, s.Actors)
		}
	}
	s, c, d := chanceFixture()
	s.Actors[0].HP = s.Actors[0].MaxHP
	executeChance(t, &s, c, d, "job-43-entry-5", []string{"self"}, 0)
	if s.Actors[0].HP != 1000 || c.HealCalls != 1 {
		t.Fatal(s.Actors, c)
	}
	// The raw self-mitigation tail runs even when the defense helper is capped.
	s, c, d = chanceFixture()
	s.Actors[0].Defense = 200
	executeChance(t, &s, c, d, "job-45-entry-2", nil)
	if s.Actors[0].Defense != 200 || s.Actors[0].Temporary != corebattle.TemporaryReduceMagic {
		t.Fatal(s.Actors)
	}
}

func TestTreatmentChanceStatAndStatusGuards(t *testing.T) {
	for _, draw := range []float64{.8499, .85} {
		s, c, d := chanceFixture()
		executeChance(t, &s, c, d, "job-45-entry-1", []string{"enemy"}, draw)
		if (s.Actors[1].Status == corebattle.StatusDofuu) != (draw < .85) {
			t.Fatal(s.Actors[1])
		}
	}
	s, c, d := chanceFixture()
	s.Actors[1].Abilities = []string{"curse"}
	executeChance(t, &s, c, d, "job-45-entry-3", []string{"enemy"})
	if s.Actors[0].MP != 80 || s.Actors[1].MP != 100 || c.MPHealCalls != 0 {
		t.Fatal(s.Actors, c)
	}
	s, c, d = chanceFixture()
	s.Actors[1].Attack = 20
	executeChance(t, &s, c, d, "job-45-entry-4", []string{"enemy"})
	if s.Actors[1].Defense != 80 || s.Actors[1].Icon != "" || c.StatDownCalls != 1 {
		t.Fatal(s.Actors, c)
	}
	// Phantom redirects can return different recipients from the two helpers.
	s, c, d = chanceFixture()
	s.Actors[1].Temporary = corebattle.TemporaryPhantom
	executeChance(t, &s, c, d, "job-45-entry-4", []string{"enemy"}, 0, .6, 0, 0, 0)
	if s.Actors[0].Attack != 60 || s.Actors[0].Icon == "" || s.Actors[1].Attack != 100 || s.Actors[1].Defense != 48 || s.Actors[1].Icon != "" {
		t.Fatal(s.Actors)
	}
	s, c, d = chanceFixture()
	s.Actors[1].Defense = 16
	executeChance(t, &s, c, d, "job-45-entry-4", []string{"enemy"})
	if s.Actors[1].Attack != 60 || s.Actors[1].Defense != 16 || s.Actors[1].Icon == "" {
		t.Fatal(s.Actors)
	}
	s, c, d = chanceFixture()
	s.Actors[1].Temporary = corebattle.TemporaryOneHitDefense
	executeChance(t, &s, c, d, "job-45-entry-6", []string{"enemy"})
	if s.Actors[1].HP != 900 || s.Actors[1].Temporary != "" || c.DamageCalls != 1 {
		t.Fatal(s.Actors, c)
	}
}

func TestTreatmentChanceSealsAndCapturedCure(t *testing.T) {
	for _, id := range []string{"job-43-entry-3", "job-43-entry-5", "job-45-entry-1", "job-45-entry-5", "job-46-entry-4"} {
		s, c, d := chanceFixture()
		s.Actors[0].Status = corebattle.StatusMagicSeal
		draws := []float64{.25}
		if id == "job-46-entry-4" {
			draws = []float64{0, 0, .99, .25}
		}
		executeChance(t, &s, c, d, id, []string{"self"}, draws...)
		if s.Actors[0].HP != 500 || s.Actors[0].Temporary != "" || s.Actors[1].HP != 900 || c.DamageCalls != 0 || c.Status.StatusCalls != 0 {
			t.Fatal(id, s.Actors, c)
		}
	}
	s, c, d := chanceFixture()
	s.Actors[0].Status = corebattle.StatusSleep
	s.Actors[2].HP, s.Actors[2].Status, s.Actors[2].Temporary = 100, corebattle.StatusSleep, corebattle.TemporaryReflectMagic
	executeChance(t, &s, c, d, "job-43-entry-4", []string{"fallen"})
	if s.Actors[0].Status != "" || s.Actors[2].Status != corebattle.StatusSleep || c.Status.CureCalls != 1 {
		t.Fatal(s.Actors, c)
	}
}

func TestTreatmentChanceDanceHPMPBoundary(t *testing.T) {
	for _, hp := range []int{79, 80, 81} {
		s, c, d := chanceFixture()
		// One living ally avoids hiding the boundary behind fallen fallback.
		s.Actors = s.Actors[:2]
		c.PartyIDs = []string{"self"}
		s.Actors[0].HP = hp
		draws := []float64{0}
		if hp > 80 {
			draws = append(draws, 0)
		}
		executeChance(t, &s, c, d, "job-45-entry-7", nil, draws...)
		if hp <= 80 {
			if s.Actors[0].HP != hp+153 || s.Actors[0].MP != 80 {
				t.Fatal(hp, s.Actors)
			}
		} else if s.Actors[0].HP != hp || s.Actors[0].MP != 96 {
			t.Fatal(hp, s.Actors)
		}
	}
	s, c, d := chanceFixture()
	s.Actors[0].Status = corebattle.StatusDanceSeal
	executeChance(t, &s, c, d, "job-45-entry-7", nil, .25)
	if c.TensionCalls != 0 || s.Actors[0].HP != 500 {
		t.Fatal(s.Actors, c)
	}
}

func TestTreatmentChanceDiceUnusedDrawAndRecoil(t *testing.T) {
	for _, id := range []string{"job-46-entry-2", "job-46-entry-4"} {
		for _, third := range []float64{0, math.Nextafter(1, 0)} {
			s, c, d := chanceFixture()
			executeChance(t, &s, c, d, id, []string{"enemy"}, .99, .99, third, 0)
			wantEnemy, wantHP := 601, 499 // 666/2=333; floor333×.9=299.
			if id == "job-46-entry-4" {
				wantEnemy, wantHP = 574, 500 // 363×.9=326.
			}
			if s.Actors[1].HP != wantEnemy || s.Actors[0].HP != wantHP {
				t.Fatal(id, third, s.Actors)
			}
		}
	}
	s, c, d := chanceFixture()
	s.Actors[0].HP, s.Actors[0].Status = 1, corebattle.StatusMagicSeal
	// Both dice consume three draws before the sealed damage; recoil still runs.
	executeChance(t, &s, c, d, "job-46-entry-2", []string{"enemy"}, 0, 0, .99, .25)
	if s.Actors[0].HP != 1 || s.Actors[1].HP != 900 || c.End.ImmediateRiskCalls != 1 {
		t.Fatal(s.Actors, c)
	}
}

func TestTreatmentChanceDeathAndCoinThresholds(t *testing.T) {
	for _, hp := range []int{999, 1000} {
		for _, defense := range []int{999, 1000} {
			for _, id := range []string{"job-46-entry-1", "job-46-entry-3", "job-46-entry-5"} {
				s, c, d := chanceFixture()
				s.Actors[1].HP, s.Actors[1].MaxHP = hp, 1000
				s.Actors[1].Defense, s.Actors[1].BaseStats.Defense = defense, defense
				draws := []float64{0}
				want := hp
				if hp <= 999 && defense <= 999 {
					want = 0
				}
				switch id {
				case "job-46-entry-1":
					if want != 0 {
						draws = nil
					}
				case "job-46-entry-3":
					draws = []float64{.5}
				case "job-46-entry-5":
					draws = []float64{0, 0}
					if hp > 999 {
						// BaseDF>999 switches direct damage to ordinary defense.
						want = 550
						if defense > 999 {
							want = 999
							draws = append(draws, 0) // nonpositive roll uses the floor draw
						}
						draws = append(draws, 0)
					} else if defense > 999 {
						want = 500
					}
				}
				executeChance(t, &s, c, d, id, []string{"enemy"}, draws...)
				if s.Actors[1].HP != want {
					t.Fatal(id, hp, defense, s.Actors)
				}
			}
		}
	}
	for _, currentDefense := range []int{999, 1000} {
		for _, id := range []string{"job-46-entry-1", "job-46-entry-3", "job-46-entry-5"} {
			s, c, d := chanceFixture()
			s.Actors[1].Defense, s.Actors[1].BaseStats.Defense = currentDefense, 1999-currentDefense
			want, draws := 900, []float64(nil)
			if id == "job-46-entry-3" {
				draws = []float64{.5}
				if currentDefense == 999 {
					want = 0
				}
			} else if id == "job-46-entry-5" {
				draws, want = []float64{0, 0}, 450
				if currentDefense == 1000 {
					want = 0
				}
			} else if currentDefense == 1000 {
				draws, want = []float64{0}, 0
			}
			executeChance(t, &s, c, d, id, []string{"enemy"}, draws...)
			if s.Actors[1].HP != want {
				t.Fatal(id, currentDefense, s.Actors)
			}
		}
	}
	for _, draw := range []float64{.7, .70001} {
		s, c, d := chanceFixture()
		s.Actors[1].BaseStats.Defense = 1000
		executeChance(t, &s, c, d, "job-46-entry-5", nil, 0, draw)
		want := 450
		if draw > .7 {
			want = 900
		}
		if s.Actors[1].HP != want {
			t.Fatal(draw, s.Actors)
		}
	}
	for _, selfBranch := range []bool{false, true} {
		s, c, d := chanceFixture()
		s.Actors[0].HP, s.Actors[1].HP, s.Actors[1].BaseStats.Defense = 1, 1, 1000
		draws := []float64{0, .7}
		if selfBranch {
			draws = []float64{.5}
		}
		executeChance(t, &s, c, d, "job-46-entry-5", nil, draws...)
		if s.Actors[0].HP != 1 || s.Actors[1].HP != 1 {
			t.Fatal(s.Actors)
		}
	}
}

func TestTreatmentChanceDeathProbabilityBoundaries(t *testing.T) {
	for _, id := range []string{"job-46-entry-0", "job-46-entry-1"} {
		chance := .2
		if id == "job-46-entry-0" {
			chance = .9
		}
		for _, draw := range []float64{chance, chance + .0001} {
			s, c, d := chanceFixture()
			draws := []float64{draw}
			if id == "job-46-entry-0" {
				draws = []float64{.99, .99, .99, draw}
			}
			executeChance(t, &s, c, d, id, []string{"enemy"}, draws...)
			if (s.Actors[1].HP == 0) != (draw == chance) {
				t.Fatal(id, draw, s.Actors)
			}
		}
	}
}

func TestTreatmentChanceCoinNativeNonlethalAndZeroPowerNeedNoDefeat(t *testing.T) {
	for _, maxHP := range []int{1, 1000} {
		s, c, d := chanceFixture()
		s.Actors[1].HP, s.Actors[1].MaxHP = 1100, maxHP
		c.Status.Defeat = nil
		draws := []float64{0, 0}
		want, calls := 1100, 0
		if maxHP == 1000 {
			draws, want, calls = append(draws, 0), 650, 1
		}
		executeChance(t, &s, c, d, "job-46-entry-5", nil, draws...)
		if s.Actors[1].HP != want || c.DamageCalls != calls {
			t.Fatal(maxHP, s.Actors, c)
		}
	}
}
