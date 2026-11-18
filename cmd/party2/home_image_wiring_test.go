package main

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"testing"

	"github.com/witchcraze/party2re/internal/character"
)

// The adventure record unlocks gate own-Home いめーじ through production wiring.
func TestHomeImageProductionWiring(t *testing.T) {
	a := newScenarioApp(t)
	token, actor := a.register()
	a.command(token, actor, "scene_enter", map[string]any{"destination": "home"}, http.StatusOK)
	a.command(token, actor, "scene_enter", map[string]any{"destination": "home_image"}, http.StatusForbidden)

	if _, err := a.db.ExecContext(context.Background(), `INSERT INTO character_adventure_unlocks (character_id, image, avatar) VALUES (?, TRUE, TRUE)`, actor); err != nil {
		t.Fatal(err)
	}
	observed := a.context(token, actor)
	if !slices.ContainsFunc(observed.Scene.Data.Destinations, func(d struct {
		ID          string          `json:"id"`
		EnterParams json.RawMessage `json:"enter_params"`
	}) bool {
		return d.ID == "home_image"
	}) {
		t.Fatalf("home_image not offered: %+v", observed.Scene.Data.Destinations)
	}
	a.command(token, actor, "scene_enter", map[string]any{"destination": "home_image"}, http.StatusOK)
	out := a.command(token, actor, "home_image_update", map[string]any{"aura_effect": 5, "avatar_id": "Ab3de"}, http.StatusOK)
	var profile character.Profile
	if err := json.Unmarshal(out.Result, &profile); err != nil {
		t.Fatal(err)
	}
	if profile.AuraEffect != 5 || profile.AvatarURL != "https://i.imgur.com/Ab3de.gif" {
		t.Fatalf("image was not saved: %s", out.Result)
	}
}
