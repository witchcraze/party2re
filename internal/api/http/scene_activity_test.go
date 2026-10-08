package http

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/casino"
	"github.com/witchcraze/party2re/internal/core/scheduling"
	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/playercontext"
)

func TestActiveScenesOverrideSelectionAndNeverMutate(t *testing.T) {
	for _, kind := range []string{"party", "pvp", "gvg", "dungeon", "challenge", "casino"} {
		t.Run(kind, func(t *testing.T) {
			f := newGatewayFixture(t)
			store := &gatewayNavigationStore{selection: playercontext.Selection{Destination: "bank"}, loadErr: errors.New("selection unavailable")}
			facts := []playercontext.Activity{{Kind: kind, ID: "active", Role: "member", Phase: "in_progress", Actions: []string{}}}
			var readErr error
			reads := 0
			read := func(ctx context.Context, owner, actor string) ([]playercontext.Activity, error) {
				f.checkContext(ctx)
				reads++
				if owner != "owner" || actor != "hero" {
					t.Fatal("lost owner/actor identity")
				}
				return facts, readErr
			}
			pc := playercontext.NewService(f, f, timer.NewService(nil), playercontext.WithNavigation(store, playercontext.SceneDefinition{ID: "town"}, playercontext.SceneDefinition{ID: "bank", Parent: "town"}), playercontext.WithActivities(read))
			opts := []Option{WithPlayerContext(pc)}
			if kind == "casino" {
				opts = append(opts, WithCasino(&casinoActivityProjection{view: &casino.RoomView{Room: casino.RoomObservation{RoomSummary: casino.RoomSummary{ID: "active", GameType: casino.GameTypeIndian, Status: casino.RoomStatusInProgress}}, Members: []casino.MemberObservation{{CharacterID: "hero", Action: "fold"}}}}))
			}
			router := f.router("rescue_request", timer.NewService(nil), opts...)
			status, observed := navigationGET(t, router)
			if status != 200 || observed.Scene.Kind != "activity" || observed.Scene.LocationID != kind || observed.Scene.Subject != nil || observed.Scene.Speaker != nil || observed.Scene.Opponent != nil || observed.Scene.BgimgURL != "" || observed.AvailableActions == nil || observed.OngoingActions == nil || store.writes != 0 || f.executions != 0 {
				t.Fatalf("active GET: %d %+v", status, observed)
			}
			status, raw := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"rescue_request","params":{"reason":"stuck"}}`, nil)
			var refreshed PlayerContextResponse
			if err := json.Unmarshal(raw["context"], &refreshed); err != nil {
				t.Fatal(err)
			}
			if status != 200 || !reflect.DeepEqual(observed.Scene, refreshed.Scene) || f.executions != 1 {
				t.Fatalf("GET/refresh: %d %s", status, raw)
			}
			readErr = errors.New("required activity read failed")
			if status, _ := navigationGET(t, router); status != 500 {
				t.Fatalf("hidden activity error: %d", status)
			}
			before := reads
			status, _ = gatewayRequest(t, router, "other", "session", "application/json", `{"action":"rescue_request","params":{"reason":"stuck"}}`, nil)
			if status != 403 || reads != before {
				t.Fatalf("unowned feature read: %d reads=%d", status, reads-before)
			}
		})
	}
}

func TestVerifiedContinuationAndConflictRecovery(t *testing.T) {
	f := newGatewayFixture(t)
	facts := []playercontext.Activity{{Kind: "gvg", ID: "room", Role: "leader", Phase: "in_progress", Actions: []string{"gvg_advance", "gvg_leave"}}}
	var readErr error
	timers := timer.NewService(nil)
	pc := playercontext.NewService(f, f, timers, playercontext.WithActivities(func(context.Context, string, string) ([]playercontext.Activity, error) { return facts, readErr }))
	type roomParams struct {
		RoomID string `json:"room_id"`
	}
	exec := func(ctx context.Context, actor string, p roomParams) (any, error) {
		if p.RoomID != "room" {
			t.Fatal("lost explicit room")
		}
		return f.execute(ctx, actor, 0)
	}
	router := f.router("rescue_request", timers, WithPlayerContext(pc), withActionCommand("gvg_advance", exec, gatewayRejection), withActionCommand("gvg_leave", exec, gatewayRejection))
	status, obs := navigationGET(t, router)
	if status != 200 || len(obs.AvailableActions) != 3 || obs.AvailableActions[1].ParamsTemplate["room_id"] != "room" {
		t.Fatalf("continuation schema: %d %+v", status, obs)
	}
	status, raw := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"gvg_advance","params":{"room_id":"room"}}`, nil)
	if status != 200 || string(raw["success"]) != "true" || f.executions != 1 {
		t.Fatalf("legal continuation: %d %s", status, raw)
	}
	f.actions = []scheduling.ScheduledAction{{ID: "overdue", State: scheduling.StateProcessing, ExecuteAt: time.Now().Add(-time.Hour)}, {ID: "done", State: scheduling.StateCompleted}, {ID: "failed", State: scheduling.StateFailed}}
	if err := timers.SetLock(context.Background(), timer.CategoryAsleep, "hero", time.Hour); err != nil {
		t.Fatal(err)
	}
	status, obs = navigationGET(t, router)
	if status != 200 || obs.Scene.Kind != "activity_conflict" || len(decodeShopScene[ActivitySceneData](t, obs).Activities) != 3 || len(obs.OngoingActions) != 2 || !obs.OngoingActions[0].IsReady || slices.ContainsFunc(obs.AvailableActions, func(a ContextAction) bool { return a.Action == "gvg_advance" }) {
		t.Fatalf("conflict preserves unfinished work: %d %+v", status, obs)
	}
	if len(f.actions) != 3 || len(facts[0].Actions) != 2 {
		t.Fatal("observation mutated feature/scheduling input")
	}
	data := decodeShopScene[ActivitySceneData](t, obs)
	if len(data.Activities[0].Actions) != 1 || data.Activities[0].Actions[0] != "gvg_leave" {
		t.Fatalf("conflict exposed continuation: %+v", data)
	}
	// Verified leave is not blocked by sleep or the generic unfinished-work gate.
	f.afterExecute = func() { readErr = errors.New("refresh failed after confirmed leave") }
	status, raw = gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"gvg_leave","params":{"room_id":"room"}}`, nil)
	if status != 200 || string(raw["success"]) != "true" || string(raw["context"]) != "null" || f.executions != 2 || raw["result"] == nil {
		t.Fatalf("lost confirmed leave: %d %s", status, raw)
	}
	var detail ErrorDetail
	if err := json.Unmarshal(raw["context_error"], &detail); err != nil || detail.Code != "CONTEXT_REFRESH_FAILED" {
		t.Fatalf("refresh recovery: %s %v", raw, err)
	}
	readErr = nil
	if status, _ := navigationGET(t, router); status != 200 || f.executions != 2 {
		t.Fatalf("GET recovery replayed operation: %d", status)
	}
}

func TestConfirmedRoomEntrySurvivesSelectionFailure(t *testing.T) {
	f := newGatewayFixture(t)
	store := &gatewayNavigationStore{}
	facts := []playercontext.Activity{}
	pc := playercontext.NewService(f, f, timer.NewService(nil), playercontext.WithNavigation(store, playercontext.SceneDefinition{ID: "town"}), playercontext.WithActivities(func(context.Context, string, string) ([]playercontext.Activity, error) { return facts, nil }))
	f.afterExecute = func() {
		facts = []playercontext.Activity{{Kind: "pvp", ID: "room", Role: "leader", Phase: "recruiting", Actions: []string{"pvp_team", "pvp_leave"}}}
		store.loadErr = errors.New("selector failed")
	}
	router := f.router("pvp_room_create", timer.NewService(nil), WithPlayerContext(pc))
	status, raw := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"pvp_room_create","params":{}}`, nil)
	var observed PlayerContextResponse
	if err := json.Unmarshal(raw["context"], &observed); err != nil {
		t.Fatal(err)
	}
	if status != 200 || string(raw["success"]) != "true" || f.executions != 1 || observed.Scene.LocationID != "pvp" || store.writes != 0 {
		t.Fatalf("lost confirmed entry: %d %s", status, raw)
	}
	if status, observed := navigationGET(t, router); status != 200 || observed.Scene.LocationID != "pvp" || f.executions != 1 {
		t.Fatalf("GET replayed entry: %d %+v", status, observed)
	}
}
