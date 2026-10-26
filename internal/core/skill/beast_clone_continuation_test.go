package skill

import (
	"encoding/json"
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func TestBeastCloneReloadContinuesWithoutReplay(t *testing.T) {
	s, c, d := beastCloneFixture()
	c.Command, c.Custom = "frozen-command", true
	executeBeastClone(t, &s, c, d, "job-12-entry-4", []string{"enemy"}, 0, 0)
	s.Actors[0].Status = corebattle.StatusDeadlyPoison
	executeBeastClone(t, &s, c, d, "job-26-entry-8", nil, 0, 0, 0, 0, 0)
	type checkpoint struct {
		State                                    corebattle.LifetimeState
		Target                                   corebattle.TargetContext
		DamageCalls, StatusCalls, RequesterLevel int
		End                                      corebattle.EndProcessing
		Arguments                                []string
		EntryID                                  string
	}
	frozen := checkpoint{s, c.TargetContext, c.DamageCalls, c.StatusCalls, *c.RequesterLevel, c.End, []string{"enemy", "tail"}, "job-26-entry-8"}
	data, err := json.Marshal(frozen)
	if err != nil {
		t.Fatal(err)
	}
	var loaded checkpoint
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatal(err)
	}
	resumed := &corebattle.BeastCloneContext{DamageContext: corebattle.DamageContext{
		EffectContext: corebattle.EffectContext{TargetContext: loaded.Target, DamageCalls: loaded.DamageCalls, RequesterLevel: &loaded.RequesterLevel},
		End:           loaded.End, GuaranteedHit: true, Defeat: func(string) error { return nil },
	}, StatusCalls: loaded.StatusCalls}
	var rebuilt BattleDefinitions
	rebuilt.RegisterBeastClone(resumed)
	for _, run := range []struct {
		s *corebattle.LifetimeState
		c *corebattle.BeastCloneContext
		d BattleDefinitions
	}{{&s, c, d}, {&loaded.State, resumed, rebuilt}} {
		// Resume from the next frozen input. Creation is already committed.
		executeBeastClone(t, run.s, run.c, run.d, loaded.EntryID, loaded.Arguments)
		executeBeastClone(t, run.s, run.c, run.d, "job-12-entry-7", loaded.Arguments, 0, 0)
	}
	if !reflect.DeepEqual(s, loaded.State) || s.RosterSequence != 1 || len(s.Actors) != 4 || s.Actors[0].HP != 611 || s.Actors[3].HP != 1 || s.Actors[3].Status != corebattle.StatusDeadlyPoison || s.Actors[0].Status != "" || s.Actors[1].HP != 707 || c.DamageCalls != 3 || resumed.DamageCalls != 3 || resumed.StatusCalls != 1 || !reflect.DeepEqual(c.Arguments, resumed.Arguments) || resumed.Command != "frozen-command" || !resumed.Custom || *resumed.RequesterLevel != 40 {
		t.Fatal(s, loaded.State, c, resumed)
	}
}

func TestBeastCloneExtendedCostAndThreshold(t *testing.T) {
	for _, row := range []struct {
		sp, mp  int
		created bool
	}{{109, 30, false}, {110, 29, false}, {110, 30, true}} {
		s, c, d := beastCloneFixture()
		s.Actors[0].Jobs = corebattle.BattleJobs{JobID: "job-26", SP: row.sp}
		s.Actors[0].MP = row.mp
		d.Invocations.Entries["extended"] = corebattle.CallbackDefinition{Invocation: corebattle.CallbackInvocation{Kind: "gem-86"}}
		draws := []float64(nil)
		if row.created {
			draws = make([]float64, 5)
		}
		executeBeastClone(t, &s, c, d, "extended", []string{"enemy", "tail"}, draws...)
		wantMP := row.mp
		if row.created {
			wantMP -= 30
		}
		if (len(s.Actors) == 4) != row.created || s.Actors[0].MP != wantMP {
			t.Fatal(row, s)
		}
	}
}
