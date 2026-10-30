package skill

import (
	"errors"
	"fmt"
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func TestIncidentalJobEveryPositionBoundary(t *testing.T) {
	_, _, catalog := incidentalFixture()
	for job := 7; job <= 9; job++ {
		for _, entry := range catalog.Invocations.JobTables[job].Variants[0] {
			if catalog.Invocations.Entries[entry.ID].Execute == nil {
				continue
			}
			t.Run(entry.ID, func(t *testing.T) {
				s, c, d := incidentalFixture()
				args, draws := []string{"enemy"}, []float64(nil)
				switch entry.ID {
				case "job-7-entry-1":
					s.Actors[0].Defense = 200
				case "job-7-entry-3":
					s.Actors[1].Hit = 50
				case "job-7-entry-5", "job-9-entry-7", "job-9-entry-9":
					s.Actors[1].Temporary = corebattle.TemporaryOneHitDefense
				case "job-7-entry-6":
					args[0], s.Actors[0].Status, draws = "self", corebattle.StatusMagicSeal, []float64{.75}
				case "job-7-entry-7":
					s.Actors[0].Status, draws = corebattle.StatusDanceSeal, []float64{.75}
				case "job-8-entry-1":
					s.Actors[0].HP = 0
				case "job-8-entry-2", "job-8-entry-3", "job-8-entry-4":
					s.Actors[2].Tension, draws = 5, []float64{.9}
				case "job-8-entry-5":
					draws = []float64{.499}
				case "job-8-entry-6":
					s.Actors[1].Status = corebattle.StatusSabaku
				case "job-8-entry-7":
					draws = []float64{0, .333, .333}
				case "job-8-entry-8":
					s.Actors[1].HP, s.Actors[1].Tension = 0, 5
				case "job-9-entry-0":
					s.Actors[0].Agility = 0
				case "job-9-entry-1":
					s.Actors[1].Agility = 6
				case "job-9-entry-2":
					args[0], s.Actors[0].Agility = "self", 100
				case "job-9-entry-3":
					s.Actors[1].HP = 1000
				case "job-9-entry-5":
					draws = []float64{.35}
				case "job-9-entry-8":
					s.Actors[0].Status, draws = corebattle.StatusPhysicalSeal, []float64{.75}
				default:
					t.Fatal("missing boundary", entry.ID)
				}
				before := append([]corebattle.LifetimeActor(nil), s.Actors...)
				executeIncidental(t, &s, c, d, entry.ID, args, draws...)
				for i, a := range s.Actors {
					want := before[i]
					switch entry.ID {
					case "job-8-entry-1":
						if i == 0 {
							want.Status = corebattle.StatusSleep
						}
					case "job-8-entry-2", "job-8-entry-3", "job-8-entry-4":
						if i == 2 {
							want.Tension = 1
							want.Status = map[string]string{"job-8-entry-2": corebattle.StatusParalyze, "job-8-entry-3": corebattle.StatusDofuu, "job-8-entry-4": corebattle.StatusDeadlyPoison}[entry.ID]
						}
					case "job-8-entry-8":
						if i == 1 {
							want.Tension = 1
						}
					case "job-7-entry-5", "job-9-entry-7", "job-9-entry-9":
						if i == 1 {
							want.Temporary = ""
						}
					}
					if !reflect.DeepEqual(a, want) {
						t.Fatal(i, a, want)
					}
				}
				if c.End.ImmediateRiskCalls != 0 || c.HealCalls != 0 || c.MPHealCalls != 0 {
					t.Fatal(c)
				}
			})
		}
	}
}

func TestIncidentalJobDrainRestoresAfterDefeatAndFailure(t *testing.T) {
	failure := errors.New("reward unavailable")
	for _, fail := range []bool{false, true} {
		s, c, d := incidentalFixture()
		s.Actors[0].Hit = 47
		s.Actors[1].HP, s.Actors[1].Defense, s.Actors[1].Gear.ArmorDefense = 1, 200, 50
		calls := 0
		c.Status.Defeat.Rewards.Apply = func(state *corebattle.LifetimeState, _ corebattle.TargetContext, _ string) error {
			calls++
			if state.Actors[0].Hit != 100 || state.Actors[1].Defense != 130 {
				t.Fatal("restored before damage tail", state)
			}
			if fail {
				return failure
			}
			return nil
		}
		c.Status.Defeat.Rewards.Validate = func(*corebattle.LifetimeState, corebattle.TargetContext, string) error { return nil }
		f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy"}}
		ops, err := d.InvocationAction(&s, &f, []string{"job-9-entry-9"}, noArithmeticDraws{t})
		if err != nil {
			t.Fatal(err)
		}
		draws := []float64{0, 0}
		if fail {
			draws = draws[:1]
		}
		rng := &damageGemDraws{t: t, draws: draws}
		err = s.ApplyInvocations(&f, d.Invocations, ops, rng)
		if fail && !errors.Is(err, failure) || !fail && err != nil {
			t.Fatal(err)
		}
		if s.Actors[0].Hit != 47 || s.Actors[1].Defense != 200 || s.Actors[1].HP != 0 || calls != 1 || rng.used != len(draws) {
			t.Fatal(s, c, calls)
		}
		wantHP := 508
		if fail {
			wantHP = 500
		}
		if s.Actors[0].HP != wantHP {
			t.Fatal(s)
		}
	}
}

func TestIncidentalJobArmorBreakResolvesBothHelpers(t *testing.T) {
	s, c, d := incidentalFixture()
	s.Actors[0].Defense, s.Actors[0].Tension, s.Actors[0].Temporary = 200, 5, corebattle.TemporaryDouble
	s.Actors[1].Temporary = corebattle.TemporaryReflectAttack
	executeIncidental(t, &s, c, d, "job-9-entry-8", []string{"enemy"}, 0)
	if s.Actors[0].Defense != 72 || s.Actors[0].Tension != 1 || s.Actors[0].Temporary != "" || s.Actors[1].Defense != 80 || s.Actors[1].Temporary != corebattle.TemporaryReflectAttack {
		t.Fatal(s)
	}
}

func TestIncidentalJobManaTransferBoundaries(t *testing.T) {
	for _, mp := range []int{0, 1, 199, 200, 201, 400} {
		for _, target := range []string{"self", "fallen", "ally"} {
			t.Run(fmt.Sprintf("mp%d/%s", mp, target), func(t *testing.T) {
				s, c, d := incidentalFixture()
				s.Actors[0].MP, s.Actors[0].MaxMP = mp, 1000
				if target == "ally" {
					s.Actors[2].ID, s.Actors[2].HP, s.Actors[2].MaxMP = "ally", 1, 1000
					c.PartyIDs[1] = "ally"
				}
				amount := min(100, mp/2)
				draws := []float64(nil)
				if amount > 0 {
					draws = []float64{0}
				}
				executeIncidental(t, &s, c, d, "job-7-entry-6", []string{target}, draws...)
				recovery := 0
				if amount > 0 {
					recovery = int(float64(amount)*.9) + 1
				}
				want := mp - amount
				if target != "ally" {
					want += recovery
				}
				if s.Actors[0].MP != want || target == "ally" && s.Actors[2].MP != recovery {
					t.Fatal(mp, target, s.Actors)
				}
			})
		}
	}
	// Absorption restores MP during the first target helper; transfer reads it afterward.
	s, c, d := incidentalFixture()
	s.Actors[0].MP, s.Actors[0].MaxMP, s.Actors[0].Temporary = 199, 1000, corebattle.TemporaryAbsorbMagic
	executeIncidental(t, &s, c, d, "job-7-entry-6", []string{"self"}, 0, 0, 0)
	if s.Actors[0].MP != 240 {
		t.Fatal(s)
	}
}

func TestIncidentalJobJokeDrawOrderAndRawTargets(t *testing.T) {
	for gag := 0; gag < 7; gag++ {
		s, c, d := incidentalFixture()
		s.Actors[0].Status, s.Actors[1].Tension, s.Actors[2].Tension = corebattle.StatusDanceSeal, 5, 5
		executeIncidental(t, &s, c, d, "job-8-entry-7", nil, float64(gag)/7, 1.0/3, .333)
		if c.JokeOutcomes[0] != gag || s.Actors[1].Status != corebattle.StatusDofuu || s.Actors[1].Tension != 1 || s.Actors[2].Status != "" || s.Actors[2].Tension != 5 || s.Actors[0].Status != corebattle.StatusDanceSeal {
			t.Fatal(s, c)
		}
	}
	for _, pos := range []int{2, 3, 4} {
		for _, draw := range []float64{0, .4, .9} {
			s, c, d := incidentalFixture()
			executeIncidental(t, &s, c, d, fmt.Sprintf("job-8-entry-%d", pos), nil, draw)
			index := int(draw * 3)
			if s.Actors[index].Status == "" || s.Actors[index].Tension != 1 {
				t.Fatal(s)
			}
		}
	}
}

func TestIncidentalJobDrainAliasAndHitGuard(t *testing.T) {
	s, c, d := incidentalFixture()
	s.Actors[0].Status, s.Actors[0].Hit = corebattle.StatusConfusion, 47
	executeIncidental(t, &s, c, d, "job-9-entry-9", []string{"self"}, 0, 0, 0)
	if s.Actors[0].HP != 498 || s.Actors[0].Defense != 80 || s.Actors[0].Hit != 47 {
		t.Fatal(s, c)
	}
	for _, agility := range []int{39, 40, 41} {
		s, c, d := incidentalFixture()
		s.Actors[1].Agility = agility
		c.GuaranteedHit = false
		draws := []float64{.99, .5, 0, 0}
		if agility > 40 {
			draws = []float64{.99}
		}
		executeIncidental(t, &s, c, d, "job-9-entry-9", []string{"enemy"}, draws...)
		if s.Actors[0].Hit != 95 || s.Actors[1].Defense != 80 || (c.HealCalls == 1) != (agility <= 40) {
			t.Fatal(agility, s, c)
		}
	}
}

func TestIncidentalJobTauntBindingPrecedesMissingTargetFallback(t *testing.T) {
	for _, id := range []string{"job-8-entry-6", "job-55-entry-5"} {
		for _, target := range []string{"self", "missing"} {
			s, c, d := incidentalFixture()
			peer := &corebattle.TreatmentMischiefContext{DamageContext: c.DamageContext}
			d.RegisterTreatmentMischief(peer)
			s.Actors[0].Status, s.Actors[1].HP = corebattle.StatusSabaku, 0
			draws := []float64(nil)
			want := 1.0
			if target == "missing" {
				draws = []float64{0}
				want = 1.7
			}
			executeIncidental(t, &s, c, d, id, []string{target}, draws...)
			if s.Actors[0].Tension != want || s.Actors[0].Status != corebattle.StatusSabaku || c.TensionCalls+peer.TensionCalls != 1 {
				t.Fatal(id, target, s, c, peer)
			}
		}
	}
}
