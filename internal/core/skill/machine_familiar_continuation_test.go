package skill

import (
	"encoding/json"
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func TestMachineFamiliarReloadContinuesWithoutReplay(t *testing.T) {
	s, c, d := machineFamiliarFixture()
	level := 99
	c.RequesterLevel, c.Command, c.Custom = &level, "frozen-command", true
	executeMachineFamiliar(t, &s, c, d, "job-79-entry-0", []string{"enemy"}, 0)
	type checkpoint struct {
		State                      corebattle.LifetimeState
		Target                     corebattle.TargetContext
		StatDownCalls, MPHealCalls int
		RequesterLevel             int
		RequesterGender, EntryID   string
		Arguments                  []string
		Draws                      []float64
	}
	// Ports are transient; persist the already committed counters and the
	// frozen next command, not the earlier drain callback or function values.
	s.Actors[0].Status = corebattle.StatusPoison
	data, err := json.Marshal(checkpoint{s, c.TargetContext, c.StatDownCalls, c.MPHealCalls, level, c.RequesterGender, "job-82-entry-2", []string{"self", "tail"}, []float64{0}})
	if err != nil {
		t.Fatal(err)
	}
	var loaded checkpoint
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatal(err)
	}
	resumed := &corebattle.MachineFamiliarContext{DamageContext: corebattle.DamageContext{
		EffectContext: corebattle.EffectContext{TargetContext: loaded.Target, StatDownCalls: loaded.StatDownCalls, MPHealCalls: loaded.MPHealCalls, RequesterLevel: &loaded.RequesterLevel},
		GuaranteedHit: true, Defeat: func(string) error { return nil },
	}, RequesterGender: loaded.RequesterGender}
	var rebuilt BattleDefinitions
	rebuilt.RegisterMachineFamiliar(resumed)
	for _, run := range []struct {
		s *corebattle.LifetimeState
		c *corebattle.MachineFamiliarContext
		d BattleDefinitions
	}{{&s, c, d}, {&loaded.State, resumed, rebuilt}} {
		executeMachineFamiliar(t, run.s, run.c, run.d, loaded.EntryID, loaded.Arguments, loaded.Draws...)
	}
	if !reflect.DeepEqual(s, loaded.State) || s.Actors[1].MP != 75 || s.Actors[0].MP != 103 || s.Actors[0].HP != 635 || s.Actors[0].Status != "" || resumed.StatDownCalls != 1 || resumed.MPHealCalls != 1 || resumed.HealCalls != 1 || resumed.CureCalls != 1 || resumed.RequesterGender != "f" || *resumed.RequesterLevel != 99 || resumed.Command != "frozen-command" || !resumed.Custom || !reflect.DeepEqual(c.Arguments, resumed.Arguments) {
		t.Fatal(s, loaded.State, c, resumed)
	}
}

func TestMachineFamiliarIndirectRestoresOnlyLostMP(t *testing.T) {
	for _, kind := range []string{"entry", "tool-84", "random"} {
		s, c, d := machineFamiliarFixture()
		s.Actors[0].MP, s.Actors[0].Abilities = 50, []string{"seal_kotowari"}
		id, draws := "job-81-entry-0", []float64{0}
		if kind != "entry" {
			// A synthetic invocation fixture isolates restoration of this actual
			// body's conversion debit; it does not certify missing player tables.
			d.Invocations.JobTables = make(map[int]corebattle.CallbackTable)
			for job := 1; job <= 87; job++ {
				d.Invocations.JobTables[job] = corebattle.CallbackTable{Variants: [][]corebattle.CallbackEntry{{}}}
			}
			d.Invocations.JobTables[1] = corebattle.CallbackTable{Variants: [][]corebattle.CallbackEntry{{corebattle.FamiliarEntries()[0]}}}
			s.Actors[0].Jobs.JobID = "job-1"
			id = "indirect"
			d.Invocations.Entries[id] = corebattle.CallbackDefinition{Invocation: corebattle.CallbackInvocation{Kind: kind}}
			draws = append([]float64{0}, draws...)
			if kind == "random" {
				draws = append([]float64{0}, draws...)
			}
		}
		executeMachineFamiliar(t, &s, c, d, id, []string{"enemy", "tail"}, draws...)
		wantMP := 50
		if kind == "entry" {
			wantMP = 47
		}
		if s.Actors[0].MP != wantMP || s.Actors[1].HP != 879 || c.DamageCalls != 1 || !reflect.DeepEqual(c.Arguments, []string{"tail"}) {
			t.Fatal(kind, s, c)
		}
	}
}
