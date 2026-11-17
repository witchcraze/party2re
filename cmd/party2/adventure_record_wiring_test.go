package main

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/witchcraze/party2re/internal/adventure"
)

func TestAdventureRecordProductionEntry(t *testing.T) {
	a := newScenarioApp(t)
	token, actor := a.register()
	a.command(token, actor, "scene_enter", map[string]any{"destination": "home"}, http.StatusOK)
	out := a.command(token, actor, "scene_enter", map[string]any{"destination": "home_adventure_record"}, http.StatusOK)
	var result struct {
		Record *adventure.RecordEntryResult `json:"adventure_record"`
	}
	if err := json.Unmarshal(out.Result, &result); err != nil {
		t.Fatal(err)
	}
	if result.Record == nil || result.Record.TotalClears != 0 || result.Record.NewlyUnlocked == nil || len(result.Record.NewlyUnlocked) != 0 {
		t.Fatalf("record entry was not wired: %s", out.Result)
	}
}
