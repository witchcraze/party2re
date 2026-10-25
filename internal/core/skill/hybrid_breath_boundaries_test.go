package skill

import (
	"errors"
	"fmt"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func TestHybridBreathMPWriteAndRandomRestoration(t *testing.T) {
	for _, mp := range []int{0, 1, 150, 151} {
		for _, kind := range []string{"entry", "tool-84", "random"} {
			t.Run(fmt.Sprintf("%s/mp%d", kind, mp), func(t *testing.T) {
				s, c, d := hybridBreathFixture()
				s.Actors[0].MP, s.Actors[0].Jobs.JobID = mp, "job-40"
				id, draws := "job-40-entry-2", []float64(nil)
				if kind != "entry" {
					id = "indirect"
					d.Invocations.Entries[id] = corebattle.CallbackDefinition{Invocation: corebattle.CallbackInvocation{Kind: kind}}
					draws = append(draws, 2.5/3)
				}
				if kind == "random" {
					// Empty synthetic tables isolate this source-bound callback;
					// they do not certify production random-catalog completeness.
					for job := 1; job <= 87; job++ {
						if _, ok := d.Invocations.JobTables[job]; !ok {
							d.Invocations.JobTables[job] = corebattle.CallbackTable{Variants: [][]corebattle.CallbackEntry{{}}}
						}
					}
					d.RegisterRevivals(&corebattle.RevivalContext{TargetContext: c.TargetContext})
					draws = append([]float64{39.5 / 87}, draws...)
				}
				if mp > 0 {
					draws = append(draws, 0)
				}
				executeHybridBreath(t, &s, c, d, id, nil, draws...)
				wantMP := 1
				if kind != "entry" {
					wantMP = max(1, mp)
				}
				if s.Actors[0].MP != wantMP || s.Actors[1].HP != 900-int(float64(mp*2)*.9) || c.AreaDamageCalls != 1 {
					t.Fatal(s, c)
				}
			})
		}
	}
	// The MP tail runs even when a magic seal rejects the area helper.
	s, c, d := hybridBreathFixture()
	s.Actors[0].Status = corebattle.StatusMagicSeal
	executeHybridBreath(t, &s, c, d, "job-40-entry-2", nil, .25)
	if s.Actors[0].MP != 1 || c.AreaDamageCalls != 0 {
		t.Fatal(s, c)
	}
}

func TestHybridBreathAssaultResolvesBeforeChoosingPower(t *testing.T) {
	for _, row := range []struct {
		tmp                corebattle.TemporaryEffect
		damage, selfDamage int
		draws              []float64
	}{
		{"", 162, 0, []float64{0}},
		{corebattle.TemporaryRegeneration, 180, 0, []float64{0}},
		{corebattle.TemporaryDefense, 90, 0, []float64{0}},
		{corebattle.TemporaryReflectAttack, 0, 180, []float64{0}},
		{corebattle.TemporaryNullAttack, 0, 0, nil},
	} {
		t.Run(string(row.tmp), func(t *testing.T) {
			s, c, d := hybridBreathFixture()
			s.Actors[1].Temporary = row.tmp
			executeHybridBreath(t, &s, c, d, "job-41-entry-5", []string{"enemy", "tail"}, row.draws...)
			if s.Actors[1].HP != 900-row.damage || s.Actors[0].HP != 500-row.selfDamage {
				t.Fatal(s)
			}
		})
	}
	// The untyped first helper follows cover and inspects the returned ally's
	// temporary before the physical helper resolves that target again.
	s, c, d := hybridBreathFixture()
	other := s.Actors[1]
	other.ID, other.Temporary = "coverer", corebattle.TemporaryCovering
	s.Actors = append(s.Actors, other)
	c.EnemyIDs = append(c.EnemyIDs, "coverer")
	s.Actors[1].Temporary = corebattle.TemporaryCover
	executeHybridBreath(t, &s, c, d, "job-41-entry-5", []string{"enemy"}, 0)
	if s.Actors[1].HP != 900 || s.Actors[3].HP != 720 {
		t.Fatal(s)
	}
}

func TestHybridBreathPhysicalAndMagicFlagsAndBaseModifiers(t *testing.T) {
	for _, row := range []struct {
		id     string
		damage int
	}{
		{"job-39-entry-7", 71}, {"job-41-entry-5", 178},
		{"job-44-entry-0", 125}, {"job-44-entry-2", 146},
		{"job-44-entry-3", 168}, {"job-44-entry-5", 59},
	} {
		t.Run(row.id, func(t *testing.T) {
			s, c, d := hybridBreathFixture()
			s.Actors[0].ItemDefinitionIDs = []string{"item-149"}
			s.Actors[0].Extended = corebattle.ExtendedWarGod
			s.Actors[1].ItemDefinitionIDs = []string{"item-200"}
			executeHybridBreath(t, &s, c, d, row.id, []string{"enemy"}, 0)
			if 900-s.Actors[1].HP != row.damage {
				t.Fatal(s)
			}
		})
	}
	// AG-based magic bypasses physical sealing/reflection and obeys magic seal.
	for _, seal := range []bool{false, true} {
		s, c, d := hybridBreathFixture()
		s.Actors[0].Status = corebattle.StatusPhysicalSeal
		s.Actors[1].Temporary = corebattle.TemporaryReflectAttack
		draws := []float64{0}
		if seal {
			s.Actors[0].Status, draws = corebattle.StatusMagicSeal, []float64{.25}
		}
		executeHybridBreath(t, &s, c, d, "job-44-entry-5", []string{"enemy"}, draws...)
		want := 846
		if seal {
			want = 900
		}
		if s.Actors[1].HP != want || s.Actors[0].HP != 500 {
			t.Fatal(s)
		}
	}
}

func TestHybridBreathRecoilUsesReturnedDamageAndSurvival(t *testing.T) {
	for _, row := range []struct {
		mode     string
		hp, risk int
		draws    []float64
	}{
		{"hit", 496, 1, []float64{0}}, {"miss", 500, 0, []float64{.99}},
		{"darkness", 500, 1, []float64{0}}, {"heal", 504, 1, []float64{0}},
		{"reflected-death", 0, 0, []float64{0}},
	} {
		t.Run(row.mode, func(t *testing.T) {
			s, c, d := hybridBreathFixture()
			switch row.mode {
			case "miss":
				c.GuaranteedHit = false
			case "darkness":
				s.Fields.Default = "darkness"
			case "heal":
				s.Actors[0].ItemDefinitionIDs = []string{"item-160"}
			case "reflected-death":
				s.Actors[0].HP, s.Actors[1].Temporary = 1, corebattle.TemporaryReflectAttack
			}
			executeHybridBreath(t, &s, c, d, "job-44-entry-2", []string{"enemy"}, row.draws...)
			if s.Actors[0].HP != row.hp || c.End.ImmediateRiskCalls != row.risk {
				t.Fatal(s, c.End)
			}
		})
	}
}

func TestHybridBreathAreaOrderAndActorDeath(t *testing.T) {
	for _, death := range []bool{false, true} {
		s, c, d := hybridBreathFixture()
		other := s.Actors[1]
		other.ID = "other"
		s.Actors = append(s.Actors, other)
		c.EnemyIDs = append(c.EnemyIDs, "other")
		draws := []float64{0, 0}
		if death {
			s.Actors[0].HP, s.Actors[1].Temporary, draws = 1, corebattle.TemporaryReflectMagic, []float64{0}
		}
		executeHybridBreath(t, &s, c, d, "job-40-entry-2", nil, draws...)
		wantSecond, calls := 778, 2 // floor(160*.85*.9).
		if death {
			wantSecond, calls = 900, 1
		}
		if s.Actors[3].HP != wantSecond || c.DamageCalls != calls || s.Actors[0].MP != 1 {
			t.Fatal(s, c)
		}
	}
	// An unknown transition remains gated before the first supported slot runs.
	s, c, d := hybridBreathFixture()
	f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy"}}
	if _, err := d.InvocationAction(&s, &f, []string{"job-44-entry-1", "job-44-entry-2"}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) || s.Actors[0].Defense != 80 {
		t.Fatal(err, s)
	}
}
