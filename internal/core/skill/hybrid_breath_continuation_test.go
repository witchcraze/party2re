package skill

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func TestHybridBreathReloadContinuesWithoutReplay(t *testing.T) {
	s, c, d := hybridBreathFixture()
	level := 99
	c.RequesterLevel, c.Command, c.Custom = &level, "frozen-command", true
	executeHybridBreath(t, &s, c, d, "job-40-entry-2", nil, 0)
	type checkpoint struct {
		State                                        corebattle.LifetimeState
		Target                                       corebattle.TargetContext
		DamageCalls, AreaDamageCalls, RequesterLevel int
		End                                          corebattle.EndProcessing
		Arguments                                    []string
		EntryID                                      string
	}
	frozen := checkpoint{s, c.TargetContext, c.DamageCalls, c.AreaDamageCalls, level, c.End, []string{"enemy", "tail"}, "job-44-entry-2"}
	data, err := json.Marshal(frozen)
	if err != nil {
		t.Fatal(err)
	}
	var loaded checkpoint
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatal(err)
	}
	resumed := &corebattle.HybridBreathContext{DamageContext: corebattle.DamageContext{
		EffectContext: corebattle.EffectContext{TargetContext: loaded.Target, DamageCalls: loaded.DamageCalls, AreaDamageCalls: loaded.AreaDamageCalls, RequesterLevel: &loaded.RequesterLevel},
		End:           loaded.End, GuaranteedHit: true, Defeat: func(string) error { return nil },
	}}
	var rebuilt BattleDefinitions
	rebuilt.RegisterHybridBreath(resumed)
	for _, run := range []struct {
		s *corebattle.LifetimeState
		c *corebattle.HybridBreathContext
		d BattleDefinitions
	}{{&s, c, d}, {&loaded.State, resumed, rebuilt}} {
		executeHybridBreath(t, run.s, run.c, run.d, loaded.EntryID, loaded.Arguments, 0)
	}
	if !reflect.DeepEqual(s, loaded.State) || s.Actors[1].HP != 706 || s.Actors[0].HP != 496 || s.Actors[0].MP != 1 || c.DamageCalls != 2 || resumed.DamageCalls != 2 || resumed.AreaDamageCalls != 1 || resumed.End.ImmediateRiskCalls != 1 || !reflect.DeepEqual(c.Arguments, resumed.Arguments) || resumed.Command != "frozen-command" || !resumed.Custom || *resumed.RequesterLevel != 99 {
		t.Fatal(s, loaded.State, c, resumed)
	}
}

func TestHybridBreathPositionalWeightsIgnoreDisplayNameCollisions(t *testing.T) {
	for _, row := range []struct{ job, pos, damage int }{{39, 0, 22}, {39, 6, 162}, {40, 0, 63}, {40, 2, 144}} {
		t.Run(fmt.Sprintf("job%d/pos%d", row.job, row.pos), func(t *testing.T) {
			s, c, d := hybridBreathFixture()
			d.RegisterRevivals(&corebattle.RevivalContext{TargetContext: c.TargetContext})
			// Shared display aliases may cross unequal tables; identity and
			// positional weight stay in the captured entry, never a name map.
			for _, job := range []int{39, 40} {
				for i := range d.Invocations.JobTables[job].Variants[0] {
					d.Invocations.JobTables[job].Variants[0][i].Name = "shared-display-alias"
				}
			}
			s.Actors[0].Jobs.JobID = fmt.Sprintf("job-%d", row.job)
			s.Actors[0].Jobs.SP = 0
			d.Invocations.Entries["tool"] = corebattle.CallbackDefinition{Invocation: corebattle.CallbackInvocation{Kind: "tool-84"}}
			entries := d.Invocations.JobTables[row.job].Variants[0]
			executeHybridBreath(t, &s, c, d, "tool", []string{"enemy"}, (float64(row.pos)+.5)/float64(len(entries)), 0)
			if s.Actors[1].HP != 900-row.damage || s.Actors[0].MP != 80 {
				t.Fatal(s)
			}
		})
	}
}

func TestHybridBreathStatusChanceBoundaries(t *testing.T) {
	for _, row := range []struct {
		id, status string
		chance     float64
	}{{"job-39-entry-4", corebattle.StatusConfusion, .5}, {"job-41-entry-3", corebattle.StatusParalyze, .35}} {
		for _, delta := range []float64{-.001, 0, .001} {
			s, c, d := hybridBreathFixture()
			s.Actors[1].Tension = 3
			executeHybridBreath(t, &s, c, d, row.id, nil, row.chance+delta)
			want, tension := "", 3.0
			if delta < 0 {
				want, tension = row.status, 1
			}
			if s.Actors[1].Status != want || s.Actors[1].Tension != tension {
				t.Fatal(row, delta, s)
			}
		}
	}
}
