package skill

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func TestIncidentalJobReloadContinuesFrozenFactsWithoutReplay(t *testing.T) {
	s, c, d := incidentalFixture()
	level := 60
	c.RequesterLevel, c.Command = &level, "frozen-incidental"
	executeIncidental(t, &s, c, d, "job-8-entry-7", nil, .9, .5, .5)
	executeIncidental(t, &s, c, d, "job-9-entry-8", []string{"enemy"})
	type checkpoint struct {
		State                                                      corebattle.LifetimeState
		Target                                                     corebattle.TargetContext
		Cursor                                                     corebattle.TargetCursor
		End                                                        corebattle.EndProcessing
		DamageCalls, HealCalls, StatDownCalls, TensionCalls, Level int
		Jokes                                                      []int
		Arguments                                                  []string
		EntryID                                                    string
	}
	frozen := checkpoint{s, c.TargetContext, c.Cursor, c.End, c.DamageCalls, c.HealCalls, c.StatDownCalls, c.TensionCalls, level, c.JokeOutcomes, []string{"enemy", "tail"}, "job-9-entry-9"}
	data, err := json.Marshal(frozen)
	if err != nil {
		t.Fatal(err)
	}
	var loaded checkpoint
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatal(err)
	}
	_, resumed, _ := incidentalFixture()
	resumed.TargetContext, resumed.Cursor, resumed.End = loaded.Target, loaded.Cursor, loaded.End
	resumed.DamageCalls, resumed.HealCalls, resumed.StatDownCalls, resumed.TensionCalls = loaded.DamageCalls, loaded.HealCalls, loaded.StatDownCalls, loaded.TensionCalls
	resumed.RequesterLevel, resumed.JokeOutcomes = &loaded.Level, loaded.Jokes
	var rebuilt BattleDefinitions
	rebuilt.RegisterIncidentalJobs(resumed)
	// Mutating the original request after serialization cannot retarget continuation.
	frozen.Arguments[0] = "self"
	for _, run := range []struct {
		s *corebattle.LifetimeState
		c *corebattle.IncidentalJobContext
		d BattleDefinitions
	}{{&s, c, d}, {&loaded.State, resumed, rebuilt}} {
		executeIncidental(t, run.s, run.c, run.d, loaded.EntryID, loaded.Arguments, 0, 0)
	}
	if !reflect.DeepEqual(s, loaded.State) || s.Actors[0].HP != 519 || s.Actors[1].HP != 878 || s.Actors[1].Defense != 72 || c.StatDownCalls != 1 || resumed.StatDownCalls != 1 || c.DamageCalls != 1 || resumed.DamageCalls != 1 || resumed.Command != "frozen-incidental" || *resumed.RequesterLevel != 60 || !reflect.DeepEqual(c.JokeOutcomes, []int{6}) || !reflect.DeepEqual(c.JokeOutcomes, resumed.JokeOutcomes) || !reflect.DeepEqual(c.Arguments, []string{"tail"}) || !reflect.DeepEqual(c.Arguments, resumed.Arguments) {
		t.Fatal(s, loaded, c, resumed)
	}
}

func TestIncidentalJobRealExtendedInvocationChargesAfterBody(t *testing.T) {
	s, c, d := incidentalFixture()
	s.Actors[0].Jobs = corebattle.BattleJobs{JobID: "job-9", SP: 90}
	s.Actors[0].MP = 20
	d.Invocations.Entries["extended"] = corebattle.CallbackDefinition{Invocation: corebattle.CallbackInvocation{Kind: "gem-86"}}
	executeIncidental(t, &s, c, d, "extended", []string{"enemy", "ignored"}, 0, 0)
	if s.Actors[0].MP != 0 || s.Actors[0].HP != 519 || s.Actors[1].HP != 878 || len(c.Arguments) != 0 {
		t.Fatal(s, c)
	}
	// All ordinary job9 positions remain required for a tool's pool, regardless of SP/MP.
	d.Invocations.Entries["tool"] = corebattle.CallbackDefinition{Invocation: corebattle.CallbackInvocation{Kind: "tool-84"}}
	f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy"}}
	if _, err := d.InvocationAction(&s, &f, []string{"tool"}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
		t.Fatal(err)
	}
}

func TestIncidentalJobRealToolPoolWaivesListedCost(t *testing.T) {
	s, c, d := incidentalFixture()
	_, inspection, _ := ordinaryToolFixture()
	d.RegisterOrdinaryTools(inspection)
	s.Actors[0].Jobs = corebattle.BattleJobs{JobID: "job-9"}
	s.Actors[0].MP = 0
	d.Invocations.Entries["tool"] = corebattle.CallbackDefinition{Invocation: corebattle.CallbackInvocation{Kind: "tool-84"}}
	// The complete real job9 table admits its extended entries too. Selection
	// of ordinary position1 ignores its SP3/listedMP3, with no synthetic bodies.
	executeIncidental(t, &s, c, d, "tool", []string{"enemy", "tail"}, .15)
	if s.Actors[1].Agility != 18 || s.Actors[0].MP != 0 || s.Actors[0].CMP != 90 || !reflect.DeepEqual(c.Arguments, []string{"tail"}) {
		t.Fatal(s, c)
	}
	delete(d.Invocations.Entries, "job-9-entry-9")
	f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy"}}
	if _, err := d.InvocationAction(&s, &f, []string{"tool"}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
		t.Fatal(err)
	}
}
