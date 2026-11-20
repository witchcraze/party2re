package http

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/witchcraze/party2re/internal/character"
	"github.com/witchcraze/party2re/internal/playercontext"
)

func (f *homeSceneFixture) GetImage(context.Context, string) (character.ImageView, error) {
	return character.ImageView{AuraEffect: 4, AvatarURL: "https://i.imgur.com/abcde.gif", AvatarUnlocked: f.imageUnlocks.Avatar}, nil
}

func (f *homeSceneFixture) UpdateImage(_ context.Context, id string, aura int, avatarID string) (character.Profile, error) {
	if avatarID != "" && !f.imageUnlocks.Avatar {
		return character.Profile{}, character.ErrAvatarLocked
	}
	f.imageWrites = append(f.imageWrites, id)
	return character.Profile{CharacterID: id, AuraEffect: aura}, nil
}

// withFixtureHomeImage serves the image through the router's own fixture.
func withFixtureHomeImage(h *Handler) { WithHomeImage(h.characters.(HomeImageService))(h) }

// ＠いめーじ is offered only in one's own home after the image unlock
// (home.cgi:87,104-106); visitors are offered nothing.
func TestHomeImageOfferedOnlyInOwnHomeAfterUnlock(t *testing.T) {
	for _, tc := range []struct {
		name            string
		unlock, visitor bool
	}{{"locked owner", false, false}, {"unlocked owner", true, false}, {"unlocked visitor", true, true}} {
		t.Run(tc.name, func(t *testing.T) {
			f, store, router := homeSceneRouter(t, withFixtureHomeImage)
			f.imageUnlocks.Image = tc.unlock
			if tc.visitor {
				store.selection.Subject = playercontext.Subject{Kind: "home", ID: "other"}
			}
			_, response := navigationGET(t, router)
			ids := []string{}
			for _, d := range decodeHomeData(t, response).Destinations {
				ids = append(ids, d.ID)
			}
			if slices.Contains(ids, "home_image") != (tc.unlock && !tc.visitor) || hasHomeAction(response, "home_image_update") {
				t.Fatalf("destinations = %v", ids)
			}
			status, raw := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"scene_enter","params":{"destination":"home_image"}}`, nil)
			if !tc.unlock && (status != 403 || !strings.Contains(string(raw["error"]), "SCENE_ACCESS_DENIED")) {
				t.Fatalf("locked entry: %d %s", status, raw)
			}
			if status, _ := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"home_image_update","params":{"aura_effect":1}}`, nil); status == 200 && tc.visitor {
				t.Fatal("home_image_update ran outside the image scene")
			}
		})
	}
}

func TestHomeImageSceneAndCommand(t *testing.T) {
	for _, tc := range []struct {
		name   string
		avatar bool
		params string
		status int
		code   string
	}{
		{"aura", false, `{"aura_effect":8}`, 200, ""},
		{"avatar before unlock", false, `{"aura_effect":1,"avatar_id":"abcde"}`, 403, "IMAGE_LOCKED"},
		{"avatar after unlock", true, `{"aura_effect":1,"avatar_id":"abcde"}`, 200, ""},
		{"missing aura", true, `{"avatar_id":"abcde"}`, 400, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, store, router := homeSceneRouter(t, withFixtureHomeImage)
			f.imageUnlocks = character.ImageUnlocks{Image: true, Avatar: tc.avatar}
			if status, raw := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"scene_enter","params":{"destination":"home_image"}}`, nil); status != 200 || store.selection.Destination != "home_image" {
				t.Fatalf("enter: %d %s", status, raw)
			}
			_, response := navigationGET(t, router)
			data := decodeSceneData[HomeImageSceneData](t, response)
			if data.Parent != "home" || data.AuraEffect != 4 || data.AvatarUnlocked != tc.avatar || len(data.Effects) != 9 || data.Effects[2].ID != 4 || !hasHomeAction(response, "home_image_update") {
				t.Fatalf("scene = %+v actions = %+v", data, response.AvailableActions)
			}
			status, raw := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"home_image_update","params":`+tc.params+`}`, nil)
			if status != tc.status || !strings.Contains(string(raw["error"]), tc.code) || (len(f.imageWrites) == 1) != (tc.status == 200) {
				t.Fatalf("update: %d %s writes=%v", status, raw, f.imageWrites)
			}
		})
	}
}

func sceneEnterDestinations(t *testing.T, response PlayerContextResponse) []string {
	t.Helper()
	i := slices.IndexFunc(response.AvailableActions, func(a ContextAction) bool { return a.Action == "scene_enter" })
	if i < 0 {
		t.Fatalf("missing scene_enter in %+v", response.AvailableActions)
	}
	var schema struct {
		Properties map[string]struct{ Enum []string }
	}
	if err := json.Unmarshal(response.AvailableActions[i].ParamsSchema, &schema); err != nil {
		t.Fatal(err)
	}
	return schema.Properties["destination"].Enum
}

// Own-home unlock read errors propagate, while town and visited homes skip
// the unlock reader so context stays 200 without offering home_image.
func TestHomeImageUnlockReadBoundaryAndErrorPropagation(t *testing.T) {
	f, store, router := homeSceneRouter(t, withFixtureHomeImage)
	if status, response := navigationGET(t, router); status != 200 || slices.ContainsFunc(decodeHomeData(t, response).Destinations, func(d SceneDestination) bool { return d.ID == "home_image" }) || slices.Contains(sceneEnterDestinations(t, response), "home_image") {
		t.Fatalf("locked own home: %d %+v", status, response.Scene)
	}
	if status, raw := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"scene_enter","params":{"destination":"home_image"}}`, nil); status != 403 || !strings.Contains(string(raw["error"]), "SCENE_ACCESS_DENIED") {
		t.Fatalf("locked direct entry: %d %s", status, raw)
	}

	f.imageUnlockErr = errors.New("unlock read failed")
	for _, tc := range []struct {
		name      string
		selection playercontext.Selection
		wantKind  string
	}{
		{"town", playercontext.Selection{Destination: "town"}, "town"},
		{"visited home", playercontext.Selection{Destination: "home", Subject: playercontext.Subject{Kind: "home", ID: "other"}}, "subject"},
	} {
		store.selection = tc.selection
		before := f.imageUnlockReads
		status, response := navigationGET(t, router)
		if status != 200 || response.Scene.Kind != tc.wantKind || f.imageUnlockReads != before {
			t.Fatalf("%s GET: status=%d kind=%s reads=%d->%d", tc.name, status, response.Scene.Kind, before, f.imageUnlockReads)
		}
		if slices.Contains(sceneEnterDestinations(t, response), "home_image") {
			t.Fatalf("%s offered home_image in scene_enter", tc.name)
		}
		if tc.selection.Destination == "home" && slices.ContainsFunc(decodeHomeData(t, response).Destinations, func(d SceneDestination) bool { return d.ID == "home_image" }) {
			t.Fatalf("%s offered home_image in home destinations", tc.name)
		}
		status, raw := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"scene_enter","params":{"destination":"home_image"}}`, nil)
		if status != 403 || !strings.Contains(string(raw["error"]), "SCENE_ACCESS_DENIED") || f.imageUnlockReads != before {
			t.Fatalf("%s direct entry: status=%d raw=%s reads=%d->%d", tc.name, status, raw, before, f.imageUnlockReads)
		}
	}

	for _, own := range []playercontext.Selection{
		{Destination: "home"},
		{Destination: "home", Subject: playercontext.Subject{Kind: "home", ID: "hero"}},
	} {
		store.selection = own
		before := f.imageUnlockReads
		if status, _ := navigationGET(t, router); status != 500 || f.imageUnlockReads == before {
			t.Fatalf("own home %+v GET: status=%d reads=%d->%d", own, status, before, f.imageUnlockReads)
		}
		before = f.imageUnlockReads
		if status, _ := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"scene_enter","params":{"destination":"home_image"}}`, nil); status != 500 || f.imageUnlockReads == before {
			t.Fatalf("own home %+v enter: status=%d reads=%d->%d", own, status, before, f.imageUnlockReads)
		}
	}

	f.imageUnlockErr = nil
	f.imageUnlocks.Image = true
	store.selection = playercontext.Selection{Destination: "home"}
	status, response := navigationGET(t, router)
	if status != 200 || !slices.Contains(sceneEnterDestinations(t, response), "home_image") || !slices.ContainsFunc(decodeHomeData(t, response).Destinations, func(d SceneDestination) bool { return d.ID == "home_image" }) {
		t.Fatalf("unlocked own home: %d %+v", status, response.Scene)
	}
}
