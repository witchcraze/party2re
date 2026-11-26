package main

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"testing"
)

// editRun rewrites a run's stored snapshot: explicit setup for states that
// would take many commands to reach.
func (a *scenarioApp) editRun(runID string, edit func(state map[string]any)) {
	a.t.Helper()
	ctx := context.Background()
	var raw []byte
	if err := a.db.QueryRowContext(ctx, `SELECT state_json FROM runs WHERE run_id = ?`, runID).Scan(&raw); err != nil {
		a.t.Fatal(err)
	}
	var state map[string]any
	if err := json.Unmarshal(raw, &state); err != nil {
		a.t.Fatal(err)
	}
	edit(state)
	raw, err := json.Marshal(state)
	if err != nil {
		a.t.Fatal(err)
	}
	if _, err := a.db.ExecContext(ctx, `UPDATE runs SET state_json = ? WHERE run_id = ?`, raw, runID); err != nil {
		a.t.Fatal(err)
	}
}

func offeredEnum(t *testing.T, o runObserved, action, field string) []string {
	t.Helper()
	var raw json.RawMessage
	for _, a := range o.AvailableActions {
		if a.Action == action {
			raw = a.ParamsSchema
		}
	}
	if raw == nil {
		t.Fatalf("%s is not offered: %+v", action, o.Scene.Data.Run)
	}
	var schema struct {
		Properties map[string]struct{ Enum []string } `json:"properties"`
	}
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatal(err)
	}
	return schema.Properties[field].Enum
}

// The production observation offers what the run host executes: an asset
// callback needs the requester's asset view (#1359) and しらべる needs an
// unopened box target (vs_monster.cgi:37–41; _npc_action.cgi:5–85).
func TestNormalRunGatewayObservationMatchesExecution(t *testing.T) {
	a := newScenarioApp(t)
	token, hero := a.register()
	started := a.runCommand(token, hero, a.runContext(token, hero), "adventure_start", map[string]any{"name": "solo-quest", "stage_id": "stage-00", "speed": 3, "command_id": "start"}, http.StatusOK)
	runID := started.Result.RunID
	a.elapseActionWait(hero)
	a.runCommand(token, hero, a.runContext(token, hero), "run_operation", map[string]any{"command_id": "advance", "operation": "advance"}, http.StatusOK)

	// job7's coin strike spends the wallet, so it executes only in an asset session.
	a.editRun(runID, func(state map[string]any) {
		for _, raw := range state["battle"].(map[string]any)["actors"].([]any) {
			if actor := raw.(map[string]any); actor["ID"] == hero {
				actor["jobs"] = map[string]any{"JobID": "job-07", "SP": 7}
				actor["MP"], actor["MaxMP"] = 10, 10
			}
		}
	})
	a.expireRunInterval(runID, hero)
	observed := a.runContext(token, hero)
	if !slices.Contains(offeredEnum(t, observed, "run_operation", "operation"), "job-7-entry-2") {
		t.Fatalf("asset callback not submittable: %+v", observed.Scene.Data.Run.Operations)
	}
	foe := ""
	for _, actor := range observed.Scene.Data.Run.Actors {
		if actor.Enemy && actor.HP > 0 {
			foe = actor.ID
		}
	}
	struck := a.runCommand(token, hero, observed, "run_operation", map[string]any{"command_id": "coin", "operation": "job-7-entry-2", "target_id": foe}, http.StatusOK)
	if struck.Result.Version != observed.Scene.Data.Run.Version+1 {
		t.Fatalf("coin strike: %+v", struck.Result)
	}

	a.editRun(runID, func(state map[string]any) {
		state["round"], state["enemies"] = 11, []any{}
		state["boxes"] = []any{map[string]any{"id": "box-1", "name": "宝箱", "item_id": "item-001"}}
	})
	a.expireRunInterval(runID, hero)
	observed = a.runContext(token, hero)
	if boxes := offeredEnum(t, observed, "run_chest", "box_id"); !slices.Equal(boxes, []string{"box-1"}) {
		t.Fatalf("offered boxes: %v", boxes)
	}
	opened := a.runCommand(token, hero, observed, "run_chest", map[string]any{"command_id": "open", "box_id": "box-1"}, http.StatusOK)
	if opened.Result.Box == nil || opened.Result.Box.OpenedBy != hero || opened.Result.Box.ItemID != "item-001" {
		t.Fatalf("chest: %+v", opened.Result)
	}
	a.expireRunInterval(runID, hero)
	a.runCommand(token, hero, a.runContext(token, hero), "run_exit", map[string]any{"command_id": "exit"}, http.StatusOK)
}
