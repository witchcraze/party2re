package skill

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/core/item"
)

const storedEntry = "job-50-entry-7"

// self HP500/1000, a living enemy and a fallen ally; the forwarded arguments
// are ["enemy","tail"], so a heal on self proves the outer target was bound.
func storedFixture(item string, rows ...corebattle.StoredTool) (corebattle.LifetimeState, corebattle.CallbackContext, BattleDefinitions) {
	s, c, f, _ := jobAssetFixture()
	var d BattleDefinitions
	d.RegisterPlayerCallbacks(PlayerCallbackContexts{JobAssets: c}, 2)
	d.RegisterOrdinaryTools(&corebattle.OrdinaryToolContext{DamageContext: c.DamageContext})
	d.RegisterNonJobInvocations()
	f.Assets.StoredTools = append([]corebattle.StoredTool{}, rows...)
	f.Assets.StoredItem, f.Assets.StoredTarget = item, " >self "
	return s, f, d
}

func herb(id string) corebattle.StoredTool {
	return corebattle.StoredTool{InstanceID: id, DefinitionID: "item-001", Category: item.UsageCategoryCombatOnly}
}

func runStored(t *testing.T, s *corebattle.LifetimeState, f *corebattle.CallbackContext, d BattleDefinitions, draws ...float64) {
	t.Helper()
	executeJobAsset(t, s, f, d, storedEntry, draws...)
}

func rejectStored(t *testing.T, s corebattle.LifetimeState, f corebattle.CallbackContext, d BattleDefinitions) {
	t.Helper()
	snapshot := func() string {
		raw, err := json.Marshal([]any{s, f.Assets})
		if err != nil {
			t.Fatal(err)
		}
		return string(raw)
	}
	before := snapshot()
	if _, err := d.InvocationAction(&s, &f, []string{storedEntry}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
		t.Fatal("admitted", err)
	}
	if after := snapshot(); after != before {
		t.Fatal("admission mutated", after)
	}
}

// Vectors: ff97fc5:_skill.cgi:3612–3626,10433–10487,10949–10973; herb40 via
// _heal 9067 is int(40×.9)=36 at draw0.
func TestStoredToolConsumesCategoryOneWithOuterTarget(t *testing.T) {
	s, f, d := storedFixture("item-001", herb("first"), herb("second"))
	runStored(t, &s, &f, d, 0)
	if s.Actors[0].HP != 536 || s.Actors[1].HP != 1000 {
		t.Fatal("outer target not bound", s.Actors)
	}
	if !reflect.DeepEqual(f.Assets.StoredOutcomes, []corebattle.StoredToolOutcome{{InstanceID: "first", Result: corebattle.StoredToolConsumed}}) || f.Assets.SelectionError {
		t.Fatal(f.Assets.StoredOutcomes)
	}
	if !s.StoredToolHistory["party"]["item-001"] || !reflect.DeepEqual(f.Arguments, []string{"enemy", "tail"}) {
		t.Fatal(s.StoredToolHistory, f.Arguments)
	}

	// Every copy is now excluded for this color, also after reload.
	raw, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	var reloaded corebattle.LifetimeState
	if err := json.Unmarshal(raw, &reloaded); err != nil {
		t.Fatal(err)
	}
	f.Assets.StoredOutcomes = nil
	runStored(t, &reloaded, &f, d)
	if reloaded.Actors[0].HP != 536 || f.Assets.StoredOutcomes[0].Result != corebattle.StoredToolNoMatch || !f.Assets.SelectionError {
		t.Fatal("reselected used definition", reloaded.Actors[0], f.Assets.StoredOutcomes)
	}

	// Another color has its own history.
	enemy := f
	enemy.TargetContext = corebattle.TargetContext{ActorID: "enemy", PartyIDs: []string{"enemy"}, EnemyIDs: []string{"self", "fallen"}}
	enemy.Assets = &corebattle.JobAssets{RequesterID: "self", HeldToolID: f.Assets.HeldToolID, StoredTools: []corebattle.StoredTool{herb("first")}, StoredItem: "item-001", StoredTarget: "enemy"}
	reloaded.Actors[1].HP = 500
	runStored(t, &reloaded, &enemy, d, 0)
	if reloaded.Actors[1].HP != 536 || enemy.Assets.StoredOutcomes[0].Result != corebattle.StoredToolConsumed {
		t.Fatal("color history leaked", reloaded.Actors[1], enemy.Assets.StoredOutcomes)
	}
}

func TestStoredToolAcceptedNoEffectStillConsumes(t *testing.T) {
	s, f, d := storedFixture("item-001", herb("only"))
	f.Assets.StoredTarget = "fallen"
	runStored(t, &s, &f, d, 0)
	if s.Actors[2].HP != 0 || f.Assets.StoredOutcomes[0].Result != corebattle.StoredToolConsumed {
		t.Fatal(s.Actors[2], f.Assets.StoredOutcomes)
	}
}

func TestStoredToolNoMatchAndUsedDefinition(t *testing.T) {
	for name, prepare := range map[string]func(*corebattle.LifetimeState, *corebattle.CallbackContext){
		"absent": func(_ *corebattle.LifetimeState, f *corebattle.CallbackContext) { f.Assets.StoredItem = "item-002" },
		"used": func(s *corebattle.LifetimeState, _ *corebattle.CallbackContext) {
			s.StoredToolHistory = map[string]map[string]bool{"party": {"item-001": true}}
		},
		"no tools": func(_ *corebattle.LifetimeState, f *corebattle.CallbackContext) {
			f.Assets.StoredTools = []corebattle.StoredTool{}
		},
	} {
		t.Run(name, func(t *testing.T) {
			s, f, d := storedFixture("item-001", herb("only"))
			prepare(&s, &f)
			history := len(s.StoredToolHistory)
			runStored(t, &s, &f, d)
			if s.Actors[0].HP != 500 || len(s.StoredToolHistory) != history || !f.Assets.SelectionError ||
				!reflect.DeepEqual(f.Assets.StoredOutcomes, []corebattle.StoredToolOutcome{{Result: corebattle.StoredToolNoMatch}}) {
				t.Fatal(s, f.Assets)
			}
		})
	}
}

func TestStoredToolExchangesOtherCategories(t *testing.T) {
	for _, held := range []string{"", "item-003"} {
		t.Run("held="+held, func(t *testing.T) {
			s, f, d := storedFixture("item-018", corebattle.StoredTool{InstanceID: "book", DefinitionID: "item-018", Category: item.UsageCategoryAnytime})
			*f.Assets.HeldToolID, *s.Actors[0].HeldToolID = held, held
			runStored(t, &s, &f, d)
			if *f.Assets.HeldToolID != "item-018" || *s.Actors[0].HeldToolID != "item-018" || s.Actors[0].HP != 500 {
				t.Fatal(*f.Assets.HeldToolID, *s.Actors[0].HeldToolID, s.Actors[0])
			}
			if !reflect.DeepEqual(f.Assets.StoredOutcomes, []corebattle.StoredToolOutcome{{InstanceID: "book", Result: corebattle.StoredToolExchanged}}) || !s.StoredToolHistory["party"]["item-018"] {
				t.Fatal(f.Assets.StoredOutcomes, s.StoredToolHistory)
			}
		})
	}
	s, f, d := storedFixture("item-018", corebattle.StoredTool{InstanceID: "book", DefinitionID: "item-018", Category: item.UsageCategoryAnytime})
	f.Assets.HeldToolID = nil
	rejectStored(t, s, f, d)
}

// The NPC branch heals its forwarded target by magic500 (int(500×.9)=450) and
// never reads the requester depot or the outer submission.
func TestStoredToolNPCHealsWithoutDepot(t *testing.T) {
	s, f, d := storedFixture("item-001", herb("only"))
	s.Actors[1].NPC, s.Actors[1].HP = true, 100
	f.TargetContext = corebattle.TargetContext{ActorID: "enemy", PartyIDs: []string{"enemy"}, EnemyIDs: []string{"self", "fallen"}}
	f.Arguments = []string{"enemy"}
	f.Assets.StoredTools = nil
	runStored(t, &s, &f, d, 0)
	if s.Actors[1].HP != 550 || s.Actors[0].HP != 500 || f.Assets.StoredOutcomes != nil || s.StoredToolHistory != nil {
		t.Fatal(s.Actors, f.Assets)
	}
}

func TestStoredToolRejectsUnsupportedContextsBeforeMutation(t *testing.T) {
	s, f, d := storedFixture("item-001", herb("only"))
	f.Assets.StoredTools = nil
	rejectStored(t, s, f, d)

	// Job memory has no catalog binding yet, and there is no travel port.
	s, f, d = storedFixture("item-168", corebattle.StoredTool{InstanceID: "memory", DefinitionID: "item-168", Category: item.UsageCategoryCombatOnly})
	rejectStored(t, s, f, d)
}

type travelCall struct{ tool, target string }

func TestStoredToolTravelSelectionErrorKeepsHistory(t *testing.T) {
	wing := corebattle.StoredTool{InstanceID: "wing", DefinitionID: "item-150", Category: item.UsageCategoryCombatOnly}
	for _, tc := range []struct {
		name   string
		err    error
		result corebattle.StoredToolResult
	}{
		{"applied", nil, corebattle.StoredToolConsumed},
		{"selection error", errors.New("floor required"), corebattle.StoredToolRetained},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, f, d := storedFixture("item-150", wing)
			f.Assets.StoredTarget = ` {"floor":3} `
			var applied []travelCall
			travel := &corebattle.StoredToolTravel{
				Validate: func(tool, target string) error { return tc.err },
				Apply: func(tool, target string, frame corebattle.TargetContext) (corebattle.TargetContext, error) {
					applied = append(applied, travelCall{tool, target})
					frame.EnemyIDs = []string{"replacement"}
					return frame, nil
				},
			}
			d.Invocations.Entries[storedEntry] = storedWithTravel(d, travel)
			runStored(t, &s, &f, d)
			if !reflect.DeepEqual(f.Assets.StoredOutcomes, []corebattle.StoredToolOutcome{{InstanceID: "wing", Result: tc.result}}) || !s.StoredToolHistory["party"]["item-150"] {
				t.Fatal(f.Assets.StoredOutcomes, s.StoredToolHistory)
			}
			want := []travelCall{{"item-150", `{"floor":3}`}}
			if tc.err != nil {
				want = nil
			}
			if !reflect.DeepEqual(applied, want) || f.Assets.SelectionError != (tc.err != nil) {
				t.Fatal(applied, f.Assets.SelectionError)
			}
			if tc.err == nil && !reflect.DeepEqual(f.EnemyIDs, []string{"replacement"}) {
				t.Fatal("travel frame not returned", f.TargetContext)
			}
		})
	}
	s, f, d := storedFixture("item-150", wing)
	d.Invocations.Entries[storedEntry] = storedWithTravel(d, &corebattle.StoredToolTravel{
		Validate: func(string, string) error { return corebattle.ErrUnsupportedEffect },
	})
	rejectStored(t, s, f, d)
}

// storedWithTravel rebinds job50 to a context carrying the travel port, as the
// Adventure consumer does when it registers the operation's JobAssetContext.
func storedWithTravel(d BattleDefinitions, travel *corebattle.StoredToolTravel) corebattle.CallbackDefinition {
	_, c, _, _ := jobAssetFixture()
	c.Travel = travel
	var rebound BattleDefinitions
	rebound.Invocations = d.Invocations
	rebound.RegisterJobAssets(c)
	return rebound.Invocations.Entries[storedEntry]
}

// Stored ordinary84 runs the current-job table including extended entries, not
// enhanced84's MP theft. A nested job50 sees the history written before the
// child, so it does not match; that selection message keeps the outer row.
func TestStoredToolNestedJob50RetainsRow(t *testing.T) {
	s, f, d := storedFixture("item-084", corebattle.StoredTool{InstanceID: "ticket", DefinitionID: "item-084", Category: item.UsageCategoryCombatOnly})
	s.Actors[0].Jobs = corebattle.BattleJobs{JobID: "job-50", SP: 200}
	d.Invocations.JobTables[50] = corebattle.CallbackTable{Variants: [][]corebattle.CallbackEntry{{{ID: storedEntry, RequiredSP: 145, MPCost: 1, Extended: true}}}}
	runStored(t, &s, &f, d, 0)
	want := []corebattle.StoredToolOutcome{{Result: corebattle.StoredToolNoMatch}, {InstanceID: "ticket", Result: corebattle.StoredToolRetained}}
	if !reflect.DeepEqual(f.Assets.StoredOutcomes, want) || !f.Assets.SelectionError || !s.StoredToolHistory["party"]["item-084"] || s.Actors[0].HP != 500 {
		t.Fatal(f.Assets.StoredOutcomes, s.StoredToolHistory, s.Actors)
	}
}

// Reached indirectly from an ordinary held tool84, job50 still binds the outer
// selector/target rather than tool84's forwarded argument.
func TestStoredToolIndirectInvocationBindsOuterSubmission(t *testing.T) {
	s, f, d := storedFixture("item-001", herb("only"))
	s.Actors[0].Jobs = corebattle.BattleJobs{JobID: "job-50", SP: 200}
	d.Invocations.JobTables[50] = corebattle.CallbackTable{Variants: [][]corebattle.CallbackEntry{{{ID: storedEntry, RequiredSP: 145, MPCost: 1, Extended: true}}}}
	executeJobAsset(t, &s, &f, d, "tool-84", 0, 0)
	if s.Actors[0].HP != 536 || s.Actors[1].HP != 1000 || f.Assets.StoredOutcomes[0].Result != corebattle.StoredToolConsumed {
		t.Fatal(s.Actors, f.Assets.StoredOutcomes)
	}
}
