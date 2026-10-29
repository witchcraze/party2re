package skill

import (
	"encoding/json"
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func TestTreatmentChanceFrozenReloadWithoutReplay(t *testing.T) {
	s, c, d := chanceFixture()
	executeChance(t, &s, c, d, "job-45-entry-4", []string{"enemy"})
	type checkpoint struct {
		State     corebattle.LifetimeState
		Target    corebattle.TargetContext
		Arguments []string
		EntryID   string
		StatDown  int
	}
	frozen := checkpoint{s, c.TargetContext, []string{"enemy", "tail"}, "job-46-entry-2", c.StatDownCalls}
	data, err := json.Marshal(frozen)
	if err != nil {
		t.Fatal(err)
	}
	var loaded checkpoint
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatal(err)
	}
	_, resumed, _ := chanceFixture()
	resumed.TargetContext, resumed.StatDownCalls = loaded.Target, loaded.StatDown
	var rebuilt BattleDefinitions
	rebuilt.RegisterTreatmentChance(resumed)
	frozen.Arguments[0] = "self"
	for _, run := range []struct {
		s *corebattle.LifetimeState
		c *corebattle.TreatmentChanceContext
		d BattleDefinitions
	}{{&s, c, d}, {&loaded.State, resumed, rebuilt}} {
		executeChance(t, run.s, run.c, run.d, loaded.EntryID, loaded.Arguments, 0, 0, .99, 0)
	}
	if !reflect.DeepEqual(s, loaded.State) || s.Actors[1].HP != 851 || s.Actors[1].Attack != 60 || s.Actors[1].Defense != 48 || s.Actors[1].Icon != "assets/images/placeholders/battle/frog.svg" || resumed.StatDownCalls != 2 || c.StatDownCalls != 2 || resumed.End.ImmediateRiskCalls != 1 || !reflect.DeepEqual(loaded.Arguments, []string{"enemy", "tail"}) {
		t.Fatal(s, loaded, c, resumed)
	}
}

func TestTreatmentChanceRandomAndExtendedCosts(t *testing.T) {
	for _, kind := range []string{"random", "gem-86"} {
		s, c, d := chanceFixture()
		s.Actors[0].Jobs.JobID, s.Actors[0].Jobs.SP = "job-46", 140
		d.Invocations.Entries["invoke"] = corebattle.CallbackDefinition{Invocation: corebattle.CallbackInvocation{Kind: kind}}
		draws := []float64{.5}
		wantMP := 36
		if kind == "random" {
			// Other tables are empty test fixtures, not production certification.
			for job := 1; job <= 87; job++ {
				if job != 46 {
					d.Invocations.JobTables[job] = corebattle.CallbackTable{Variants: [][]corebattle.CallbackEntry{{}}}
				}
			}
			// Select the extended position then redraw to the ordinary roulette.
			draws, wantMP = []float64{45.1 / 87, 5.1 / 6, 3.1 / 6, .5}, 80
		}
		executeChance(t, &s, c, d, "invoke", nil, draws...)
		if s.Actors[0].MP != wantMP || kind == "gem-86" && s.Actors[0].HP != 250 || kind == "random" && s.Actors[1].HP != 0 {
			t.Fatal(kind, s.Actors)
		}
	}
	s, c, d := chanceFixture()
	s.Actors[0].Jobs.JobID, s.Actors[0].Jobs.SP = "job-45", 0
	d.Invocations.Entries["invoke"] = corebattle.CallbackDefinition{Invocation: corebattle.CallbackInvocation{Kind: "tool-84"}}
	executeChance(t, &s, c, d, "invoke", nil, 3.1/8, 0, 0)
	if s.Actors[0].MP != 97 || s.Actors[1].MP != 82 {
		t.Fatal("delegated recovery must retain the MP increase", s.Actors)
	}
}

func TestTreatmentChanceNativeCoinReloadWithoutNewPorts(t *testing.T) {
	s, c, d := chanceFixture()
	s.Actors[1].HP, s.Actors[1].MaxHP = 1100, 3600
	later := s.Actors[1]
	later.ID, later.HP, later.BaseStats.Defense = "later", 500, 1000
	s.Actors = append(s.Actors, later)
	c.EnemyIDs = append(c.EnemyIDs, later.ID)
	executeChance(t, &s, c, d, "job-45-entry-2", nil)
	type checkpoint struct {
		State     corebattle.LifetimeState
		Target    corebattle.TargetContext
		Arguments []string
		StatUp    int
	}
	frozen := checkpoint{s, c.TargetContext, []string{"enemy", "tail"}, c.StatUpCalls}
	data, err := json.Marshal(frozen)
	if err != nil {
		t.Fatal(err)
	}
	var loaded checkpoint
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatal(err)
	}
	_, resumed, rebuilt := chanceFixture()
	resumed.TargetContext, resumed.StatUpCalls = loaded.Target, loaded.StatUp
	frozen.Arguments[0] = "self"
	for _, run := range []struct {
		s *corebattle.LifetimeState
		c *corebattle.TreatmentChanceContext
		d BattleDefinitions
	}{{&s, c, d}, {&loaded.State, resumed, rebuilt}} {
		executeChance(t, run.s, run.c, run.d, "job-46-entry-5", loaded.Arguments, 0, 0, 0, 0)
	}
	if !reflect.DeepEqual(s, loaded.State) || s.Actors[1].HP != 0 || s.Actors[3].HP != 250 || c.StatUpCalls != 1 || resumed.StatUpCalls != 1 || c.DamageCalls != 1 || resumed.DamageCalls != 1 || s.Actors[0].Defense != 120 || !reflect.DeepEqual(loaded.Arguments, []string{"enemy", "tail"}) {
		t.Fatal(s, loaded, c, resumed)
	}
}
