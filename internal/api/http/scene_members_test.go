package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/bank"
	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/dungeon"
	"github.com/witchcraze/party2re/internal/placelog"
	"github.com/witchcraze/party2re/internal/playercontext"
)

type fakePlaceMembers struct {
	rows              []placelog.Member
	readErr, writeErr error
	refreshed         []placelog.Place
	refreshedAs       []placelog.Member
	left              []string
	reads             int
}

func (f *fakePlaceMembers) Refresh(_ context.Context, p placelog.Place, m placelog.Member, _ time.Time) error {
	if f.writeErr != nil {
		return f.writeErr
	}
	f.refreshed, f.refreshedAs = append(f.refreshed, p), append(f.refreshedAs, m)
	return nil
}

func (f *fakePlaceMembers) Leave(_ context.Context, characterID string) error {
	f.left = append(f.left, characterID)
	return f.writeErr
}

func (f *fakePlaceMembers) Read(context.Context, placelog.Place, time.Time) ([]placelog.Member, error) {
	f.reads++
	return f.rows, f.readErr
}

func TestFacilitySceneObservesMembersWithoutWriting(t *testing.T) {
	members := &fakePlaceMembers{rows: []placelog.Member{
		{CharacterID: "other", Name: "B", Color: "#00ff00", IconURL: "https://i.imgur.com/abcde.gif", AuraEffect: 3},
	}}
	_, _, _, router := bankSceneRouter(t, WithPlaceMembers(members))
	status, observation := navigationGET(t, router)
	want := []PlaceMember{
		{Kind: "npc", Name: bank.NPCName, Color: npcColor},
		{Kind: "character", Name: "B", Color: "#00ff00", IconURL: "https://i.imgur.com/abcde.gif", AuraEffect: 3},
	}
	if status != 200 || len(observation.Scene.Members) != 2 || observation.Scene.Members[0] != want[0] || observation.Scene.Members[1] != want[1] {
		t.Fatalf("members = %d %+v", status, observation.Scene.Members)
	}
	if len(members.refreshed) != 0 || len(members.left) != 0 {
		t.Fatalf("GET wrote: refreshed %v, left %v", members.refreshed, members.left)
	}
	raw, _ := json.Marshal(observation.Scene.Members)
	if bytes.Contains(raw, []byte("other")) || bytes.Contains(raw, []byte("character_id")) {
		t.Fatalf("members leak character IDs: %s", raw)
	}

	_, _, _, router = bankSceneRouter(t, WithPlaceMembers(&fakePlaceMembers{readErr: errors.New("valkey down")}))
	if status, _ := navigationGET(t, router); status != 500 {
		t.Fatalf("member read failure must fail the observation, got %d", status)
	}
}

func TestCommandsRefreshMembershipBeforeRunning(t *testing.T) {
	members := &fakePlaceMembers{}
	f, timers, router := actionWaitRouterWith(t, "bank", timer.NewService(nil), playercontext.DefaultActionWait, WithPlaceMembers(members))
	f.char.Name, f.char.Color = "Hero", "#123456"

	if status, raw := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"bank_deposit","params":{"amount":10}}`, nil); status != 200 {
		t.Fatalf("deposit: %d %s", status, raw["error"])
	}
	wantPlace := placelog.Place{Kind: placelog.KindFacility, ID: "bank"}
	wantMember := placelog.Member{CharacterID: "hero", Name: "Hero", Color: "#123456", IconURL: "avatar"}
	if len(members.refreshed) != 1 || members.refreshed[0] != wantPlace || members.refreshedAs[0] != wantMember {
		t.Fatalf("refresh = %+v as %+v", members.refreshed, members.refreshedAs)
	}

	// A command rejected by the wait still refreshes, as read_member precedes the action.
	if status, raw := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"bank_deposit","params":{"amount":10}}`, nil); status != 409 || actionErrorCode(t, raw) != "ACTION_WAIT" {
		t.Fatalf("second deposit: %d %s", status, raw["error"])
	}
	if len(members.refreshed) != 2 {
		t.Fatalf("rejected command did not refresh: %d", len(members.refreshed))
	}

	// presence_refresh passes the wait, refreshes and starts no wait.
	before, _ := timers.GetRemainingLock(context.Background(), timer.CategoryActionWait, "hero")
	if status, raw := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"presence_refresh"}`, nil); status != 200 {
		t.Fatalf("presence_refresh during the wait: %d %s", status, raw["error"])
	}
	if len(members.refreshed) != 3 || f.depositCalls != 1 {
		t.Fatalf("presence_refresh: refreshes %d, deposits %d", len(members.refreshed), f.depositCalls)
	}
	if after, _ := timers.GetRemainingLock(context.Background(), timer.CategoryActionWait, "hero"); after > before {
		t.Fatalf("presence_refresh restarted the wait: %v -> %v", before, after)
	}
	if status, _ := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"presence_refresh","params":{"x":1}}`, nil); status != 400 {
		t.Fatalf("unknown params must be rejected, got %d", status)
	}
}

func TestMemberRefreshFailureFailsThePreflight(t *testing.T) {
	f, _, router := actionWaitRouterWith(t, "bank", timer.NewService(nil), playercontext.DefaultActionWait, WithPlaceMembers(&fakePlaceMembers{writeErr: errors.New("valkey down")}))
	status, raw := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"bank_deposit","params":{"amount":10}}`, nil)
	if status != 500 || actionErrorCode(t, raw) != "ACTION_PREFLIGHT_FAILED" || f.depositCalls != 0 {
		t.Fatalf("refresh failure: %d %s, deposits %d", status, raw["error"], f.depositCalls)
	}
}

func TestMovingLeavesThePlace(t *testing.T) {
	members := &fakePlaceMembers{}
	_, _, router := actionWaitRouterWith(t, "bank", timer.NewService(nil), playercontext.DefaultActionWait, WithPlaceMembers(members))
	if status, raw := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"scene_back"}`, nil); status != 200 {
		t.Fatalf("scene_back: %d %s", status, raw["error"])
	}
	if len(members.left) != 1 || members.left[0] != "hero" {
		t.Fatalf("left = %v", members.left)
	}
}

func TestActivitySceneOffersPresenceRefreshAndEmptyMembers(t *testing.T) {
	f := newGatewayFixture(t)
	facts := []playercontext.Activity{{Kind: "dungeon", ID: "run", Role: "member", Phase: "in_progress", Actions: []string{}}}
	pc := playercontext.NewService(f, f, timer.NewService(nil), playercontext.WithActivities(func(context.Context, string, string) ([]playercontext.Activity, error) { return facts, nil }))
	router := f.router("rescue_request", timer.NewService(nil), WithPlayerContext(pc), WithPlaceMembers(&fakePlaceMembers{}),
		WithDungeon(&dungeonSceneFixture{observeExpeditionFn: func(context.Context, string) (*dungeon.ExpeditionObservation, error) {
			return &dungeon.ExpeditionObservation{ID: "run", Status: "in_progress", Directions: []dungeon.Direction{}}, nil
		}}))
	status, obs := navigationGET(t, router)
	if status != 200 || obs.Scene.Kind != "activity" || obs.Scene.Members == nil || len(obs.Scene.Members) != 0 {
		t.Fatalf("activity scene: %d kind=%q members=%v", status, obs.Scene.Kind, obs.Scene.Members)
	}
	if !slices.ContainsFunc(obs.AvailableActions, func(a ContextAction) bool { return a.Action == "presence_refresh" }) {
		t.Fatalf("presence_refresh not offered during an activity: %+v", obs.AvailableActions)
	}
	if !slices.Contains(obs.Scene.Support.Actions, SceneActionSupport{Action: "presence_refresh", Connected: true, EntryEligible: true}) {
		t.Fatalf("support: %+v", obs.Scene.Support.Actions)
	}
	if status, raw := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"presence_refresh"}`, nil); status != 200 {
		t.Fatalf("presence_refresh during an activity: %d %s", status, raw["error"])
	}
}
