package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/casino"
	"github.com/witchcraze/party2re/internal/playercontext"
)

func TestCasinoConfirmedCommandsSurviveFailedObservation(t *testing.T) {
	for _, command := range []string{"casino_room_join", "casino_room_start", "casino_room_leave"} {
		t.Run(command, func(t *testing.T) {
			f := newCasinoSceneFixture(t)
			f.room("room", casino.GameTypeHighLow, command != "casino_room_join")
			var executions int
			// Test-only adapters exercise the existing dispatcher boundary; this PR
			// does not connect Casino mutations in production.
			adapter := withActionCommand(command, func(ctx context.Context, actor string, p CasinoRoomParams) (any, error) {
				executions++
				var result any
				var err error
				switch command {
				case "casino_room_join":
					result, err = f.service.JoinRoom(ctx, p.RoomID, actor, "secret", f.char.Tired)
				case "casino_room_start":
					result, err = f.service.StartGame(ctx, p.RoomID, actor)
				case "casino_room_leave":
					err = f.service.LeaveRoom(ctx, p.RoomID, actor)
					result = map[string]bool{"left": err == nil}
				}
				if err == nil {
					f.reads.viewErr = errors.New("detail refresh failed")
					f.accounts.err = errors.New("lobby refresh failed")
				}
				return result, err
			}, gatewayRejection)
			router := f.router(adapter)
			status, raw := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"`+command+`","params":{"room_id":"room"}}`, nil)
			if status != 200 || string(raw["success"]) != "true" || string(raw["context"]) != "null" || raw["result"] == nil || executions != 1 {
				t.Fatalf("known outcome lost: %d %s", status, raw)
			}
			var e ErrorDetail
			if err := json.Unmarshal(raw["context_error"], &e); err != nil || e.Code != "CONTEXT_REFRESH_FAILED" {
				t.Fatalf("recovery: %s %v", raw, err)
			}
			if status, _ := navigationGET(t, router); status != 500 || executions != 1 {
				t.Fatal("failed GET hid error or replayed command")
			}
			f.reads.viewErr, f.accounts.err = nil, nil
			status, observed := navigationGET(t, router)
			if status != 200 || executions != 1 || f.store.writes != 0 {
				t.Fatalf("GET recovery: %d %+v", status, observed)
			}
			if command == "casino_room_leave" {
				if observed.Scene.Kind != "facility" || f.char.Tired != 1 {
					t.Fatalf("leave replayed or remained active: %+v", observed)
				}
			} else {
				if observed.Scene.Kind != "activity" || decodeShopScene[ActivitySceneData](t, observed).Casino == nil {
					t.Fatalf("admission hidden: %+v", observed)
				}
			}
		})
	}
}

func TestCasinoAdmissionAndLifetimeRemainServiceOwned(t *testing.T) {
	f := newCasinoSceneFixture(t)
	f.room("room", casino.GameTypeIndian, false)
	f.store.selection.Subject = playercontext.Subject{Kind: "room", ID: "room"}
	router := f.router()
	ctx := context.Background()
	before, err := f.rooms.GetRoom(ctx, "room")
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if status, _ := navigationGET(t, router); status != 200 {
			t.Fatal(status)
		}
	}
	after, err := f.rooms.GetRoom(ctx, "room")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatal("GET changed room state or lifetime")
	}
	if _, err := f.service.JoinRoom(ctx, "room", "hero", "wrong", 0); !errors.Is(err, casino.ErrInvalidPassword) {
		t.Fatalf("join password: %v", err)
	}
	if _, err := f.service.SpectateRoom(ctx, "room", "hero", "wrong"); !errors.Is(err, casino.ErrInvalidPassword) {
		t.Fatalf("spectate password: %v", err)
	}
	after.AllowSpectators = false
	if err := f.rooms.UpdateRoom(ctx, *after); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.SpectateRoom(ctx, "room", "hero", "secret"); !errors.Is(err, casino.ErrSpectatorsNotAllowed) {
		t.Fatalf("spectator gate: %v", err)
	}
	_, observed := navigationGET(t, router)
	if decodeShopScene[CasinoSelectedSceneData](t, observed).SpectateParams != nil {
		t.Fatal("disabled spectator candidate")
	}
	// Expiry is feature-owned lifecycle cleanup, separate from gameplay mutation.
	after.UpdatedAt = time.Now().Add(-casino.IdleRoomTimeout - time.Minute)
	if err := f.rooms.UpdateRoom(ctx, *after); err != nil {
		t.Fatal(err)
	}
	if status, observed := navigationGET(t, router); status != 200 || observed.Scene.Kind != "selection_unavailable" {
		t.Fatalf("expired selection: %d %+v", status, observed)
	}
	if f.store.writes != 0 {
		t.Fatal("GET cleared or renewed selection")
	}
}

func TestCasinoDetailFailuresAndChangingPhase(t *testing.T) {
	f := newCasinoSceneFixture(t)
	f.room("room", casino.GameTypeIndian, true)
	router := f.router()
	for _, failure := range []error{errors.New("storage/enrichment failed"), casino.ErrRoomNotFound, casino.ErrRoomViewForbidden} {
		f.reads.viewErr = failure
		if status, _ := navigationGET(t, router); status != 500 {
			t.Fatalf("partial activity: %d", status)
		}
	}
	f.reads.viewErr = nil
	f.facts = []playercontext.Activity{{Kind: "casino", ID: "room", Role: "leader", Phase: "in_progress", Round: 1, Actions: []string{"casino_room_leave", "casino_room_action"}}}
	if status, _ := navigationGET(t, router); status != 500 {
		t.Fatal("mixed phase observation")
	}
	f.facts = nil
	if status, _ := navigationGET(t, router); status != 200 {
		t.Fatal("GET recovery")
	}
}

func TestCasinoLobbyWindowAndEmptyArrays(t *testing.T) {
	f := newCasinoSceneFixture(t)
	router := f.router()
	_, observed := navigationGET(t, router)
	if data := decodeShopScene[CasinoLobbySceneData](t, observed); data.Rooms == nil || len(data.Rooms) != 0 {
		t.Fatal("empty rooms must be []")
	}
	for i := range 105 {
		f.room(fmt.Sprintf("room-%03d", i), casino.GameTypeIndian, false)
	}
	rooms, err := f.service.ListRooms(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(rooms) != 100 {
		t.Fatalf("window: %d", len(rooms))
	}
	for i := 1; i < len(rooms); i++ {
		if rooms[i-1].ID >= rooms[i].ID {
			t.Fatal("unstable window order")
		}
	}
	f.store.selection = playercontext.Selection{Destination: "casino", Offset: 90, Limit: 20}
	_, observed = navigationGET(t, router)
	data := decodeShopScene[CasinoLobbySceneData](t, observed)
	if len(data.Rooms) != 10 || data.Page.Next != nil || data.WindowLimit != 100 {
		t.Fatalf("bounded final page: %+v", data)
	}
	if status, _ := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"scene_select","params":{"target_kind":"room","target_id":"room-104"}}`, nil); status != 404 {
		t.Fatalf("selection outside public window: %d", status)
	}
}

func TestExpiredCasinoActivityReturnsToStoredSelection(t *testing.T) {
	f := newCasinoSceneFixture(t)
	f.room("expired", casino.GameTypeIndian, true)
	r, err := f.rooms.GetRoom(context.Background(), "expired")
	if err != nil {
		t.Fatal(err)
	}
	r.UpdatedAt = time.Now().Add(-casino.IdleRoomTimeout - time.Minute)
	if err := f.rooms.UpdateRoom(context.Background(), *r); err != nil {
		t.Fatal(err)
	}
	status, observed := navigationGET(t, f.router())
	if status != 200 || observed.Scene.Kind != "facility" || observed.Scene.LocationID != "casino" || f.store.writes != 0 {
		t.Fatalf("expired actual activity: %d %+v", status, observed)
	}
}
