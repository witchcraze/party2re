package skill

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/core/random"
)

func TestOrdinaryToolWholeAdmissionIsInert(t *testing.T) {
	for _, row := range []struct {
		id    string
		setup func(*corebattle.LifetimeState, *corebattle.OrdinaryToolContext)
	}{
		{"tool-41", func(s *corebattle.LifetimeState, c *corebattle.OrdinaryToolContext) { c.Defeat = nil }},
		{"tool-42", func(s *corebattle.LifetimeState, c *corebattle.OrdinaryToolContext) {
			other := s.Actors[1]
			other.ID = "other"
			other.Gear = nil
			s.Actors = append(s.Actors, other)
			c.EnemyIDs = append(c.EnemyIDs, "other")
		}},
		{"tool-79", func(s *corebattle.LifetimeState, c *corebattle.OrdinaryToolContext) {
			s.Actors[0].ItemDefinitionIDs = []string{"item-205"}
			c.RequesterLevel = nil
		}},
		{"tool-77", func(s *corebattle.LifetimeState, c *corebattle.OrdinaryToolContext) {
			s.Actors[1].Tension = 5
			c.RequesterLevel = nil
		}},
		{"tool-86", func(s *corebattle.LifetimeState, c *corebattle.OrdinaryToolContext) {
			s.Actors[0].Tension = 5
			c.RequesterLevel = nil
		}},
		{"tool-87", func(s *corebattle.LifetimeState, c *corebattle.OrdinaryToolContext) {
			s.Actors[2].Gear.AccessoryID = "item-110"
		}},
		{"tool-87", func(s *corebattle.LifetimeState, c *corebattle.OrdinaryToolContext) { c.EquipmentReset = nil }},
		{"tool-213", func(s *corebattle.LifetimeState, c *corebattle.OrdinaryToolContext) { c.RequesterMaxHP = nil }},
		{"tool-151", func(s *corebattle.LifetimeState, c *corebattle.OrdinaryToolContext) { delete(c.Inspection, "enemy") }},
		{"job-78-entry-1", func(s *corebattle.LifetimeState, c *corebattle.OrdinaryToolContext) {
			c.Inspection["enemy"] = corebattle.InspectionContent{Treasure: true, Price: 10}
		}},
		{"tool-75", func(s *corebattle.LifetimeState, c *corebattle.OrdinaryToolContext) {
			c.Cursor = corebattle.TargetCursor{Source: "submitted", ActorID: "enemy"}
		}},
		{"tool-86", func(s *corebattle.LifetimeState, c *corebattle.OrdinaryToolContext) {
			c.Cursor = corebattle.TargetCursor{ActorID: "enemy"}
		}},
	} {
		t.Run(row.id, func(t *testing.T) {
			s, c, d := ordinaryToolFixture()
			row.setup(&s, c)
			c.Arguments = []string{"enemy", "tail"}
			before, err := json.Marshal(s)
			if err != nil {
				t.Fatal(err)
			}
			args := append([]string(nil), c.Arguments...)
			f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: args}
			ops := []corebattle.CallbackInvocation{{Kind: "entry", EntryID: row.id}}
			_, discoveryErr := d.InvocationAction(&s, &f, []string{row.id}, noArithmeticDraws{t})
			runErr := s.ApplyInvocations(&f, d.Invocations, ops, noArithmeticDraws{t})
			after, err := json.Marshal(s)
			if err != nil {
				t.Fatal(err)
			}
			if !errors.Is(discoveryErr, corebattle.ErrUnsupportedEffect) || !errors.Is(runErr, corebattle.ErrUnsupportedEffect) || string(before) != string(after) || !reflect.DeepEqual(args, c.Arguments) || c.Results != nil || c.DamageCalls != 0 || c.TensionCalls != 0 {
				t.Fatal(discoveryErr, runErr, s, c)
			}
		})
	}
}

func TestOrdinaryToolDirectExecutorsRejectWrongFamily(t *testing.T) {
	s, c, _ := ordinaryToolFixture()
	if err := s.ApplyOrdinaryTool(c, "job-9-entry-6", noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
		t.Fatal(err)
	}
	if err := s.ApplyInspection(c, "tool-130", noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
		t.Fatal(err)
	}
	if len(c.Results) != 0 || len(c.Arguments) != 0 {
		t.Fatal(c)
	}
}

func TestOrdinaryToolSelectionErrorVersusAcceptedNoEffect(t *testing.T) {
	for _, id := range []string{"tool-41", "tool-42", "tool-75", "tool-77", "tool-82", "tool-213"} {
		s, c, d := ordinaryToolFixture()
		s.Actors = append(s.Actors[:1], s.Actors[2])
		c.EnemyIDs = nil
		f := corebattle.CallbackContext{TargetContext: c.TargetContext}
		_, err := d.InvocationAction(&s, &f, []string{id}, noArithmeticDraws{t})
		if !errors.Is(err, corebattle.ErrNoBattleTarget) || len(c.Results) != 0 {
			t.Fatal(id, err, c.Results)
		}
	}
	// A fallen enemy is an accepted helper no-effect, and failed capture consumes.
	for _, id := range []string{"tool-41", "tool-75", "tool-82", "tool-213", "tool-130"} {
		s, c, d := ordinaryToolFixture()
		s.Actors[1].HP = 0
		draws := []float64{}
		if id == "tool-75" {
			draws = []float64{0}
		}
		result := executeOrdinaryTool(t, &s, c, d, id, []string{"enemy"}, draws...)
		if !result.Consume || len(c.Results) != 1 || s.Actors[1].HP != 0 || s.Actors[1].Temporary != "" || s.Actors[1].Status != "" {
			t.Fatal(id, result, s.Actors)
		}
	}
	// tenshon's absent recipient defaults after binding, unlike a resolved ally.
	s, c, d := ordinaryToolFixture()
	s.Actors[1].HP = 0
	s.Actors[0].Status = corebattle.StatusSabaku
	executeOrdinaryTool(t, &s, c, d, "tool-77", []string{"enemy"}, 0)
	if s.Actors[0].Tension != 1.7 || s.Actors[0].Status != corebattle.StatusSabaku {
		t.Fatal(s.Actors[0])
	}
}

func TestOrdinaryToolLaterUnsupportedAndUnknownEffects(t *testing.T) {
	s, c, d := ordinaryToolFixture()
	c.EquipmentReset = nil
	f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"self"}}
	before, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	ops := []corebattle.CallbackInvocation{{Kind: "entry", EntryID: "tool-1"}, {Kind: "entry", EntryID: "tool-87"}}
	if err := s.ApplyInvocations(&f, d.Invocations, ops, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
		t.Fatal(err)
	}
	after, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) || len(c.Results) != 0 || c.HealCalls != 0 {
		t.Fatal(s, c)
	}
	for _, id := range []string{"tool-110", "gem-101", "gem-102", "tool-999"} {
		if _, err := d.InvocationAction(&s, &f, []string{id}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
			t.Fatal(id, err)
		}
	}
	// Unknown preceding effects can replace the roster or cursor. Reject before
	// executing them rather than discover missing inspection facts afterwards.
	executed := false
	d.Invocations.Entries["prior"] = corebattle.CallbackDefinition{
		Validate: func(*corebattle.LifetimeState, *corebattle.CallbackContext) error { return nil },
		Execute: func(*corebattle.LifetimeState, *corebattle.CallbackContext, random.Generator) error {
			executed = true
			return nil
		},
	}
	if _, err := d.InvocationAction(&s, &f, []string{"prior", "tool-151"}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) || executed {
		t.Fatal(err, executed)
	}
}

func TestOrdinaryToolReset188PassPrecedesRefill(t *testing.T) {
	for _, revive := range []bool{false, true} {
		s, c, d := ordinaryToolFixture()
		s.Actors[0].Gear.AccessoryID, s.Actors[0].ItemDefinitionIDs = "item-188", []string{"item-188"}
		s.Actors[2].Gear.AccessoryID, s.Actors[2].ItemDefinitionIDs = "item-188", []string{"item-188"}
		s.Actors[2].MaxHP = 200
		draw := .9
		if revive {
			draw = 0
		}
		executeOrdinaryTool(t, &s, c, d, "tool-87", nil, draw)
		if s.Actors[2].HP != 200 || (s.Actors[2].Temporary == corebattle.TemporaryRevival) != revive || s.Actors[0].Temporary != corebattle.TemporaryRevival {
			t.Fatal(revive, s.Actors)
		}
	}
}

func TestOrdinaryToolReloadFrozenInputAndReceipt(t *testing.T) {
	type snapshot struct {
		State   corebattle.LifetimeState
		Input   corebattle.OrdinaryToolInput
		Results []corebattle.OrdinaryToolResult
		Cursor  corebattle.TargetCursor
	}
	s, c, d := ordinaryToolFixture()
	c.Inspection["enemy"] = corebattle.InspectionContent{Treasure: true, RewardKind: 2, DefinitionID: "armor-1", Price: 10000}
	executeOrdinaryTool(t, &s, c, d, "tool-151", nil)
	encoded, err := json.Marshal(snapshot{s, c.OrdinaryToolInput, c.Results, c.Cursor})
	if err != nil {
		t.Fatal(err)
	}
	var saved snapshot
	if err := json.Unmarshal(encoded, &saved); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(c.Results, saved.Results) || !reflect.DeepEqual(c.OrdinaryToolInput, saved.Input) {
		t.Fatal(saved)
	}
	// A receipt returns its saved consumption intent/facts. It does not invoke
	// a rebuilt callback, draw again, or mutate the saved reward definitions.
	receipt := saved.Results[0]
	if !receipt.Consume || receipt.Facts[0].DefinitionID != "armor-1" {
		t.Fatal(receipt)
	}
	// Resuming the next operation reuses frozen input and reconstructed ports.
	_, reload, _ := ordinaryToolFixture()
	reload.OrdinaryToolInput, reload.Results, reload.Cursor = saved.Input, saved.Results, saved.Cursor
	var rebuilt BattleDefinitions
	rebuilt.RegisterOrdinaryTools(reload)
	executeOrdinaryTool(t, &saved.State, reload, rebuilt, "tool-213", []string{"enemy"})
	executeOrdinaryTool(t, &s, c, d, "tool-213", []string{"enemy"})
	if !reflect.DeepEqual(saved.State, s) || !reflect.DeepEqual(reload.Results, c.Results) {
		t.Fatal(saved, reload.Results, s, c.Results)
	}
}
