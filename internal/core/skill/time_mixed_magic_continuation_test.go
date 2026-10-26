package skill

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func TestTimeMixedMagicReloadFrozenCommandWithoutReplay(t *testing.T) {
	s, c, d := timeMagicFixture()
	executeTimeMagic(t, &s, c, d, "job-30-entry-1", []string{"enemy"}, 0)
	type checkpoint struct {
		State     corebattle.LifetimeState
		Target    corebattle.TargetContext
		Entry     string
		Arguments []string
	}
	data, err := json.Marshal(checkpoint{s, c.TargetContext, "job-30-entry-8", []string{"missing", "tail"}})
	if err != nil {
		t.Fatal(err)
	}
	var loaded checkpoint
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatal(err)
	}
	for _, state := range []*corebattle.LifetimeState{&s, &loaded.State} {
		_, rebuilt, _ := timeMagicFixture()
		rebuilt.TargetContext = loaded.Target
		rebuilt.Defeat = func(string) error { t.Fatal("unexpected defeat"); return nil }
		var definitions BattleDefinitions
		definitions.RegisterTimeMixedMagic(rebuilt)
		executeTimeMagic(t, state, rebuilt, definitions, loaded.Entry, loaded.Arguments, 0, 0, 0, 0, 0, 0)
		if rebuilt.DamageCalls != 3 || rebuilt.StatDownCalls != 0 || rebuilt.Arguments[0] != "tail" {
			t.Fatal(rebuilt)
		}
	}
	if !reflect.DeepEqual(s, loaded.State) || s.Actors[1].HP != 671 || loaded.Arguments[0] != "missing" {
		t.Fatal(s, loaded)
	}
}

func TestTimeMixedMagicIndirectPoolsCostsAndMissingOwner(t *testing.T) {
	s, c, d := timeMagicFixture()
	d.RegisterArithmetic(&corebattle.ArithmeticContext{TargetContext: c.TargetContext, Defeat: c.Defeat})
	d.Invocations.Entries["tool"] = corebattle.CallbackDefinition{Invocation: corebattle.CallbackInvocation{Kind: "tool-84"}}
	s.Actors[0].MP, s.Actors[0].Jobs.SP, s.Actors[0].Jobs.JobID = 0, 0, "job-30"
	executeTimeMagic(t, &s, c, d, "tool", []string{"enemy"}, 1.0/9, 0)
	if s.Actors[0].MP != 0 || s.Actors[1].HP != 869 {
		t.Fatal(s)
	}
	// The complete job29 pool includes the unbound extended field even when
	// the tool draw would select an implemented ordinary callback.
	s.Actors[0].Jobs.JobID = "job-29"
	f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy"}}
	if _, err := d.InvocationAction(&s, &f, []string{"tool"}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
		t.Fatal(err)
	}
	if len(d.Invocations.JobTables[29].Variants[0]) != 8 || len(d.Invocations.JobTables[31].Variants[0]) != 8 {
		t.Fatal(d.Invocations.JobTables)
	}
	// Empty unrelated tables are a scoped invocation fixture, not certification
	// of the production catalog. The three real tables retain all positions.
	for job := 1; job <= 87; job++ {
		if _, exists := d.Invocations.JobTables[job]; !exists {
			d.Invocations.JobTables[job] = corebattle.CallbackTable{Variants: [][]corebattle.CallbackEntry{{}}}
		}
	}
	d.Invocations.Entries["random"] = corebattle.CallbackDefinition{Invocation: corebattle.CallbackInvocation{Kind: "random"}}
	executeTimeMagic(t, &s, c, d, "random", nil, 28.0/87, 7.5/8, 2.5/8, 0, 0, 0, 0, 0)
	if s.Actors[1].HP != 779 || s.Actors[0].MP != 0 {
		t.Fatal(s)
	}
	delete(d.Invocations.Entries, "job-31-entry-5")
	if _, err := d.InvocationAction(&s, &f, []string{"random"}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
		t.Fatal(err)
	}
}

func TestTimeMixedMagicDelegatedExtendedCostAndExplicitTarget(t *testing.T) {
	s, c, d := timeMagicFixture()
	s.Actors[0].Jobs.JobID, s.Actors[0].Jobs.SP = "job-30", 150
	d.Invocations.Entries["extended"] = corebattle.CallbackDefinition{Invocation: corebattle.CallbackInvocation{Kind: "gem-86"}}
	executeTimeMagic(t, &s, c, d, "extended", []string{"enemy", "self"}, 0)
	if s.Actors[0].MP != 22 || s.Actors[1].HP != 585 || len(c.Arguments) != 0 {
		t.Fatal(s, c)
	}
	s, c, d = timeMagicFixture()
	s.Actors[0].Jobs.JobID, s.Actors[0].Jobs.SP, s.Actors[0].MP = "job-30", 149, 80
	d.Invocations.Entries["extended"] = corebattle.CallbackDefinition{Invocation: corebattle.CallbackInvocation{Kind: "gem-86"}}
	executeTimeMagic(t, &s, c, d, "extended", []string{"enemy"})
	if s.Actors[0].MP != 80 || s.Actors[1].HP != 900 {
		t.Fatal(s)
	}
}

func TestTimeMixedMagicExtendedReadsStatsAfterEarlierDefeat(t *testing.T) {
	s, c, d := timeMagicFixture()
	s.Actors[1].HP = 50
	c.Defeat = func(id string) error {
		if id != "enemy" {
			t.Fatal(id)
		}
		s.Actors[1].HP = 900
		s.Actors[0].Defense, s.Actors[0].Agility = 200, 60
		return nil
	}
	executeTimeMagic(t, &s, c, d, "job-30-entry-8", nil, 0, 0, 0, 0, 0, 0)
	if s.Actors[1].HP != 666 || c.DamageCalls != 3 {
		t.Fatal(s, c)
	}
}
