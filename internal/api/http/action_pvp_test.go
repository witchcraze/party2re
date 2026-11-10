package http

import (
	"context"
	"errors"
	"testing"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/playercontext"
	"github.com/witchcraze/party2re/internal/pvp"
)

type pvpLeaveFixture struct {
	*gatewayFixture
	PvPService
	service *pvp.Service
}

func (f *pvpLeaveFixture) FindByIDForUpdate(ctx context.Context, actor string) (corecharacter.Character, error) {
	return f.FindByID(ctx, actor)
}

func (f *pvpLeaveFixture) Update(_ context.Context, char corecharacter.Character) error {
	f.char = char
	return nil
}

func (f *pvpLeaveFixture) LeaveRoom(ctx context.Context, actor, room string) error {
	f.executions++
	if f.executionErr != nil {
		return f.executionErr
	}
	return f.service.LeaveRoom(ctx, actor, room)
}

func TestPvPRecoveryGateway(t *testing.T) {
	for _, tc := range []struct {
		name, actor, token, params string
		err                        error
		status                     int
	}{
		{"recover", "hero", "session", `{"room_id":"ROOM"}`, nil, 200},
		{"other character", "other", "session", `{"room_id":"ROOM"}`, nil, 403},
		{"no session", "hero", "", `{"room_id":"ROOM"}`, nil, 401},
		{"missing room", "hero", "session", `{}`, nil, 400},
		{"wrong type", "hero", "session", `{"room_id":7}`, nil, 400},
		{"unowned room", "hero", "session", `{"room_id":"other"}`, nil, 404},
		{"foreign membership", "hero", "session", `{"room_id":"ROOM"}`, pvp.ErrCharacterNotInRoom, 403},
		{"live match", "hero", "session", `{"room_id":"ROOM"}`, pvp.ErrMatchAlreadyStarted, 409},
		{"stale version", "hero", "session", `{"room_id":"ROOM"}`, pvp.ErrTransitionConflict, 409},
		{"store down", "hero", "session", `{"room_id":"ROOM"}`, errors.New("store down"), 500},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			f := &pvpLeaveFixture{gatewayFixture: newGatewayFixture(t)}
			rooms := pvp.NewMemoryRoomRepository()
			var err error
			f.service, err = pvp.NewService(rooms, f, &struct{ pvp.BattleEngine }{}, pvp.WithTransitionRepository(pvp.NewMemoryTransitionRepository()))
			if err != nil {
				t.Fatal(err)
			}
			room, err := f.service.CreateRoom(ctx, "hero", pvp.CreateRoomRequest{Name: "Lost", Bet: 10})
			if err != nil {
				t.Fatal(err)
			}
			if err := rooms.DeleteRoom(ctx, room.Room.ID); err != nil {
				t.Fatal(err)
			}
			pc := playercontext.NewService(f, f, timer.NewService(nil), playercontext.WithActivities(func(ctx context.Context, owner, actor string) ([]playercontext.Activity, error) {
				claim, err := f.service.GetCharacterClaim(ctx, actor)
				if err != nil || claim == "" {
					return nil, err
				}
				return []playercontext.Activity{{Kind: "pvp", ID: claim, Phase: "recovery_pending", Actions: []string{"pvp_leave"}}}, nil
			}))
			router := f.gatewayFixture.router("rescue_request", timer.NewService(nil), WithPvP(f), WithPlayerContext(pc))
			status, observed := navigationGET(t, router)
			var offered bool
			for _, action := range observed.AvailableActions {
				if action.Action == "pvp_leave" && action.ParamsTemplate["room_id"] == room.Room.ID {
					offered = true
				}
			}
			if status != 200 || !offered {
				t.Fatalf("recovery not offered: %d %+v", status, observed)
			}
			f.executionErr = tc.err
			params := tc.params
			if params == `{"room_id":"ROOM"}` {
				params = `{"room_id":"` + room.Room.ID + `"}`
			}
			status, raw := gatewayRequest(t, router, tc.actor, tc.token, "application/json", `{"action":"pvp_leave","params":`+params+`}`, nil)
			if status != tc.status || f.char.Money != 90 {
				t.Fatalf("outcome: %d %s money=%d", status, raw, f.char.Money)
			}
			claim, err := f.service.GetCharacterClaim(ctx, "hero")
			if err != nil || (claim == "") != (status == 200) {
				t.Fatalf("incorrect claim release: %s %v", claim, err)
			}
			if status == 200 {
				if string(raw["success"]) != "true" || f.executions != 1 {
					t.Fatalf("confirmed recovery lost: %s", raw)
				}
				status, observed = navigationGET(t, router)
				if status != 200 || observed.Scene.Kind == "activity" || f.executions != 1 {
					t.Fatalf("activity remains or GET replayed recovery: %d %+v", status, observed)
				}
			}
		})
	}
}

func TestPvPLeaveUnconfigured(t *testing.T) {
	f := newGatewayFixture(t)
	router := f.router("rescue_request", timer.NewService(nil), WithPvP(nil))
	status, raw := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"pvp_leave","params":{"room_id":"room"}}`, nil)
	if status != 501 || f.queryCalls != 0 {
		t.Fatalf("unconfigured command queried or executed: %d %s", status, raw)
	}
}
