package skill

import (
	"fmt"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func TestCounterResetEveryPositionGuardBoundary(t *testing.T) {
	_, _, catalog := counterResetFixture()
	for id := range catalog.Invocations.Entries {
		t.Run(id, func(t *testing.T) {
			s, c, d := counterResetFixture()
			a, e := &s.Actors[0], &s.Actors[1]
			args, draws := []string{"enemy", "tail"}, []float64(nil)
			c.Defeat, c.StatusDefeat = nil, nil
			wantTmp := corebattle.TemporaryEffect("")
			switch id {
			case "job-35-entry-0", "job-36-entry-0", "job-36-entry-1", "job-36-entry-2", "job-36-entry-3":
				// These writes have neither a seal guard nor a living guard.
				a.HP, a.Status = 0, corebattle.StatusMagicSeal
				wantTmp = map[string]corebattle.TemporaryEffect{"job-35-entry-0": corebattle.TemporaryDeflect, "job-36-entry-0": corebattle.TemporaryReflectDance, "job-36-entry-1": corebattle.TemporaryReflectBreath, "job-36-entry-2": corebattle.TemporaryReflectMagic, "job-36-entry-3": corebattle.TemporaryReflectAttack}[id]
			case "job-35-entry-2", "job-37-entry-5":
				e.HP, e.Gear = 1000, nil
			case "job-35-entry-3":
				e.Temporary, e.Gear = corebattle.TemporaryOneHitDefense, nil
			case "job-35-entry-4", "job-37-entry-6":
				a.HP, a.Status = a.MaxHP, corebattle.StatusMagicSeal
				draws = []float64{0}
			case "job-35-entry-5", "job-37-entry-1", "job-37-entry-4":
				a.Status, draws = corebattle.StatusMagicSeal, []float64{.25}
			case "job-35-entry-6":
				e.HP, e.Temporary, e.Gear = 1000, corebattle.TemporaryOneHitDefense, nil
			case "job-37-entry-0":
				a.Status, draws = corebattle.StatusMagicSeal, []float64{.25}
			case "job-37-entry-2":
				a.Status, draws = corebattle.StatusDanceSeal, []float64{.25}
			case "job-37-entry-3":
				a.Status = corebattle.StatusMagicSeal
				wantTmp = corebattle.TemporaryReduceMagic
			case "job-37-entry-7":
				a.Status, draws = corebattle.StatusPhysicalSeal, []float64{.25}
			}
			beforeA, beforeE := *a, *e
			executeCounterReset(t, &s, c, d, id, args, draws...)
			a, e = &s.Actors[0], &s.Actors[1]
			if a.HP != beforeA.HP || e.HP != beforeE.HP || a.Temporary != wantTmp || e.Attack != beforeE.Attack || e.Defense != beforeE.Defense || e.Status != "" || a.MP != 80 || s.Actors[2].Temporary != "" {
				t.Fatal(a, e, c)
			}
		})
	}
}

func TestCounterResetSealCuresAndIndependentComposition(t *testing.T) {
	for _, row := range []struct {
		id, seal string
		draws    []float64
		tmp      corebattle.TemporaryEffect
	}{
		{"job-35-entry-5", corebattle.StatusMagicSeal, []float64{0}, corebattle.TemporaryNullMagic},
		{"job-37-entry-1", corebattle.StatusMagicSeal, []float64{0}, corebattle.TemporaryAbsorbMagic},
		{"job-37-entry-4", corebattle.StatusMagicSeal, []float64{0}, corebattle.TemporaryReflectMagic},
	} {
		s, c, d := counterResetFixture()
		s.Actors[0].Status = row.seal
		executeCounterReset(t, &s, c, d, row.id, []string{"self"}, row.draws...)
		if s.Actors[0].Status != "" || s.Actors[0].Temporary != row.tmp {
			t.Fatal(row, s)
		}
	}
	// The damage seal can fail while the later status seal cures independently.
	s, c, d := counterResetFixture()
	s.Actors[0].Status = corebattle.StatusMagicSeal
	executeCounterReset(t, &s, c, d, "job-35-entry-6", nil, .9, 0, 0)
	if s.Actors[1].HP != 900 || s.Actors[1].Status != corebattle.StatusParalyze || c.AreaDamageCalls != 0 || c.AreaStatusCalls != 1 || s.Actors[0].Status != "" {
		t.Fatal(s, c)
	}
}

func TestCounterResetStatusProbabilityBoundaries(t *testing.T) {
	for _, row := range []struct {
		id, status string
		chance     float64
		death      bool
	}{
		{"job-35-entry-2", "", .2, true}, {"job-35-entry-6", corebattle.StatusParalyze, .2, false},
		{"job-37-entry-0", corebattle.StatusMagicSeal, .7, false}, {"job-37-entry-2", corebattle.StatusDanceSeal, .7, false},
		{"job-37-entry-5", corebattle.StatusPhysicalSeal, .8, false},
	} {
		for _, delta := range []float64{-.001, 0, .001} {
			t.Run(fmt.Sprintf("%s/%g", row.id, delta), func(t *testing.T) {
				s, c, d := counterResetFixture()
				draws := []float64{row.chance + delta}
				if row.id == "job-35-entry-6" {
					draws = append([]float64{0}, draws...)
				}
				executeCounterReset(t, &s, c, d, row.id, []string{"enemy"}, draws...)
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

func TestCounterResetReflectionAliasingAndBaselines(t *testing.T) {
	for _, category := range []corebattle.CallbackCategory{corebattle.CategoryMagic, corebattle.CategoryBreath, corebattle.CategoryDance, corebattle.CategoryPhysical} {
		s, c, d := counterResetFixture()
		pos := map[corebattle.CallbackCategory]int{corebattle.CategoryDance: 0, corebattle.CategoryBreath: 1, corebattle.CategoryMagic: 2, corebattle.CategoryPhysical: 3}[category]
		executeCounterReset(t, &s, c, d, fmt.Sprintf("job-36-entry-%d", pos), nil)
		incoming := corebattle.DamageContext{EffectContext: corebattle.EffectContext{TargetContext: corebattle.TargetContext{ActorID: "enemy", PartyIDs: []string{"enemy"}, EnemyIDs: []string{"self", "fallen"}}}, GuaranteedHit: true, Defeat: func(string) error { return nil }}
		for _, effectCategory := range []corebattle.CallbackCategory{corebattle.CategoryUntyped, category} {
			rng := &damageGemDraws{t: t, draws: []float64{0}}
			r, err := s.ApplyDamage(&incoming, corebattle.TargetEffect{TargetID: "self", Category: effectCategory, Payload: corebattle.CallbackPayload{Number: 100}, Arguments: []string{"1"}}, rng)
			want := "self"
			if effectCategory == category {
				want = "enemy"
			}
			if err != nil || r.TargetID != want || r.Amount != 90 {
				t.Fatal(category, r, err)
			}
		}
	}
	// Break follows physical reflection; it resets the returned actor only and
	// preserves both baselines, status and extended state without a down tail.
	s, c, d := counterResetFixture()
	s.Actors[0].Attack, s.Actors[0].Defense, s.Actors[0].Tension = 300, 240, 5
	s.Actors[0].Temporary, s.Actors[0].Status, s.Actors[0].Extended = corebattle.TemporaryReduceMagic, corebattle.StatusPoison, corebattle.ExtendedCrown
	s.Actors[1].Temporary = corebattle.TemporaryReflectAttack
	executeCounterReset(t, &s, c, d, "job-37-entry-7", []string{"enemy"})
	if a := s.Actors[0]; a.Attack != 100 || a.Defense != 80 || a.BaseStats.Attack != 100 || a.BaseStats.Defense != 80 || a.Temporary != "" || a.Tension != 1 || a.Status != corebattle.StatusPoison || a.Extended != corebattle.ExtendedCrown || c.StatDownCalls != 0 || s.Actors[1].Attack != 200 {
		t.Fatal(s, c)
	}
	// Ally reflection writes through to the caster; a fallen ally falls back.
	for _, alive := range []bool{false, true} {
		s, c, d = counterResetFixture()
		if alive {
			s.Actors[2].HP, s.Actors[2].Temporary = 100, corebattle.TemporaryReflectMagic
		}
		executeCounterReset(t, &s, c, d, "job-37-entry-4", []string{"fallen"})
		if s.Actors[0].Temporary != corebattle.TemporaryReflectMagic {
			t.Fatal(s)
		}
	}
}
