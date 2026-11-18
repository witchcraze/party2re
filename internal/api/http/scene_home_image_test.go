package http

import (
	"context"
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

// A failed unlock read fails the observation instead of hiding the scene; a
// locked image is an ordinary 200 without it.
func TestHomeImageUnlockReadFailurePropagates(t *testing.T) {
	f, store, router := homeSceneRouter(t, withFixtureHomeImage)
	if status, response := navigationGET(t, router); status != 200 || slices.ContainsFunc(decodeHomeData(t, response).Destinations, func(d SceneDestination) bool { return d.ID == "home_image" }) {
		t.Fatalf("locked: %d %+v", status, response.Scene)
	}
	f.imageUnlockErr = errors.New("unlock read failed")
	if status, _ := navigationGET(t, router); status != 500 {
		t.Fatalf("read failure observed as %d", status)
	}
	// The offered scene_enter enum checks every destination, from any scene.
	store.selection = playercontext.Selection{Destination: "town"}
	if status, _ := navigationGET(t, router); status != 500 {
		t.Fatalf("scene_enter enum hid the read failure: %d", status)
	}
	f.imageUnlockErr = nil
	if status, response := navigationGET(t, router); status != 200 || response.Scene.Kind != "town" {
		t.Fatalf("town after recovery: %d %+v", status, response.Scene)
	}
}
