package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"slices"
	"testing"
)

// wiredScene is the part of an observation the wiring check reads; scene data
// differ per facility, so it decodes only the navigation facts they share.
type wiredScene struct {
	Character json.RawMessage `json:"character"`
	Scene     struct {
		LocationID string `json:"location_id"`
		Data       struct {
			Parent       string             `json:"parent"`
			Destinations []wiredDestination `json:"destinations"`
			Page         struct {
				Next json.RawMessage `json:"next"`
			} `json:"page"`
		} `json:"data"`
		Support struct {
			Actions []struct {
				Action    string `json:"action"`
				Connected bool   `json:"connected"`
			} `json:"actions"`
		} `json:"support"`
	} `json:"scene"`
}

type wiredDestination struct {
	ID          string          `json:"id"`
	EnterParams json.RawMessage `json:"enter_params"`
}

func (s wiredScene) hasNext() bool {
	return len(s.Scene.Data.Page.Next) != 0 && string(s.Scene.Data.Page.Next) != "null"
}

// TestCompositionEveryOfferedSceneObservesThroughProductionServices enters every
// destination the town and each entered scene offer, so a production scene reader
// or an admitted command without a connected handler fails here.
func TestCompositionEveryOfferedSceneObservesThroughProductionServices(t *testing.T) {
	a := newScenarioApp(t)
	token, hero := a.register()
	path := "/api/v1/characters/" + hero

	observe := func() wiredScene {
		t.Helper()
		var s wiredScene
		a.call(token, http.MethodGet, path+"/context", "", http.StatusOK, &s)
		return s
	}
	navigate := func(action string, params json.RawMessage) wiredScene {
		t.Helper()
		if params == nil {
			params = json.RawMessage("{}")
		}
		body, err := json.Marshal(map[string]any{"action": action, "params": params})
		if err != nil {
			t.Fatal(err)
		}
		var out struct {
			Context wiredScene `json:"context"`
		}
		a.call(token, http.MethodPost, path+"/actions", string(body), http.StatusOK, &out)
		return out.Context
	}

	entered := map[string]bool{}
	var visit func(parentID string, parent wiredScene)
	visit = func(parentID string, parent wiredScene) {
		t.Helper()
		ids := []string{}
		for {
			for _, d := range parent.Scene.Data.Destinations {
				ids = append(ids, d.ID)
			}
			// Other scenes page their own lists (guilds, wares), not destinations.
			if len(parent.Scene.Data.Destinations) == 0 || !parent.hasNext() {
				break
			}
			parent = navigate("scene_page", parent.Scene.Data.Page.Next)
		}
		// Reverse order starts on the last page just listed; each return
		// reopens the first page, from which entry pages forward.
		for _, id := range slices.Backward(ids) {
			// The walk spans HTTP rate-limit windows; navigation sets no action wait.
			a.resetRateLimits(token)
			var scene wiredScene
			for current := parent; ; current = navigate("scene_page", current.Scene.Data.Page.Next) {
				destinations := current.Scene.Data.Destinations
				if i := slices.IndexFunc(destinations, func(d wiredDestination) bool { return d.ID == id }); i >= 0 {
					scene = navigate("scene_enter", destinations[i].EnterParams)
					break
				}
				if !current.hasNext() {
					t.Fatalf("%s is no longer offered by %s", id, parentID)
				}
			}
			entered[id] = true
			first, second := observe(), observe()
			if !bytes.Equal(first.Character, second.Character) {
				t.Fatalf("observing %s changed the character:\n%s\n%s", id, first.Character, second.Character)
			}
			if scene.Scene.LocationID != id || first.Scene.LocationID != id || first.Scene.Data.Parent != parentID {
				t.Fatalf("entering %s observed location %q/%q with parent %q; want parent %q",
					id, scene.Scene.LocationID, first.Scene.LocationID, first.Scene.Data.Parent, parentID)
			}
			// Offers are drawn from these entries, so each must reach a handler.
			for _, s := range first.Scene.Support.Actions {
				if !s.Connected {
					t.Errorf("%s admits %s without a connected handler", id, s.Action)
				}
			}
			visit(id, first)
			if parent = navigate("scene_back", nil); parent.Scene.LocationID != parentID {
				t.Fatalf("scene_back from %s observed %s; want %s", id, parent.Scene.LocationID, parentID)
			}
		}
	}
	visit("town", observe())

	// Ungated scenes must have been offered; gated ones (CanEnter) appear only
	// to qualified characters, so the scenario does not force them.
	pc := newPlayerContext(&coreServices{}, &socServices{}, &econServices{}, &cmbtServices{}, &miscServices{})
	for _, d := range pc.SceneDefinitions() {
		if d.ID != "town" && d.CanEnter == nil && !entered[d.ID] {
			t.Errorf("registered destination %s was never offered", d.ID)
		}
	}
}
