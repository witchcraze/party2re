package http

import (
	"context"
	"errors"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
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

func TestPvPLeaveInProgressGateway(t *testing.T) {
	ctx := context.Background()
	f := &pvpLeaveFixture{gatewayFixture: newGatewayFixture(t)}
	rival, _ := corecharacter.New("Rival")
	rival.ID = "rival"
	rival.PlayerID = "rival-owner"
	rival.Money = 100
	rival.Stats.HP = 100
	rival.Stats.MaxHP = 100
	outsider, _ := corecharacter.New("Outsider")
	outsider.ID = "outsider"
	outsider.PlayerID = "outsider-owner"
	rooms := pvp.NewMemoryRoomRepository()
	svc, err := pvp.NewService(rooms, &multiCharFixture{chars: map[string]corecharacter.Character{"hero": f.char, "rival": rival, "outsider": outsider}}, &inProgressTestEngine{}, pvp.WithTransitionRepository(pvp.NewMemoryTransitionRepository()))
	if err != nil {
		t.Fatal(err)
	}
	f.service = svc
	room, err := svc.CreateRoom(ctx, "hero", pvp.CreateRoomRequest{Name: "Live", Bet: 10})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.JoinRoom(ctx, "rival", room.Room.ID, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SelectTeam(ctx, "hero", room.Room.ID, pvp.ColorRed); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SelectTeam(ctx, "rival", room.Room.ID, pvp.ColorBlue); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.StartMatch(ctx, "hero", room.Room.ID); err != nil {
		t.Fatal(err)
	}
	pc := playercontext.NewService(f, f, timer.NewService(nil), playercontext.WithActivities(func(ctx context.Context, owner, actor string) ([]playercontext.Activity, error) {
		detail, err := svc.GetCharacterRoom(ctx, actor)
		if err != nil {
			return nil, nil
		}
		role := "member"
		if detail.Room.LeaderCharacterID == actor {
			role = "leader"
		}
		return []playercontext.Activity{{Kind: "pvp", ID: detail.Room.ID, Phase: detail.Room.Status, Round: detail.Room.Round, Role: role, Actions: []string{"pvp_leave", "pvp_advance"}}}, nil
	}))
	router := f.gatewayFixture.router("rescue_request", timer.NewService(nil), WithPvP(f), WithPlayerContext(pc))

	// Unrelated actor is rejected at gateway & service level
	status, _ := gatewayRequest(t, router, "other", "session", "application/json", `{"action":"pvp_leave","params":{"room_id":"`+room.Room.ID+`"}}`, nil)
	if status != 403 {
		t.Fatalf("expected 403 for unrelated actor, got %d", status)
	}
	if err := svc.LeaveRoom(ctx, "outsider", room.Room.ID); !errors.Is(err, pvp.ErrCharacterNotInRoom) {
		t.Fatalf("expected ErrCharacterNotInRoom for unrelated actor, got %v", err)
	}

	// Leader leaves in_progress via pvp_leave: 200 OK, no refund, claim released, rival becomes leader
	status, raw := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"pvp_leave","params":{"room_id":"`+room.Room.ID+`"}}`, nil)
	if status != 200 || string(raw["success"]) != "true" {
		t.Fatalf("expected 200 success for in-progress leave, got %d %s", status, raw)
	}
	claim, err := svc.GetCharacterClaim(ctx, "hero")
	if err != nil || claim != "" {
		t.Fatalf("expected hero claim released, got %q err=%v", claim, err)
	}
	updated, err := svc.GetRoom(ctx, room.Room.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Room.LeaderCharacterID != "rival" || updated.Room.PrizePool != 20 {
		t.Fatalf("expected rival promoted to leader with 20G pool intact, got %+v", updated.Room)
	}
}

type multiCharFixture struct {
	chars map[string]corecharacter.Character
}

func (m *multiCharFixture) FindByID(_ context.Context, id string) (corecharacter.Character, error) {
	c, ok := m.chars[id]
	if !ok {
		return corecharacter.Character{}, corecharacter.ErrNotFound
	}
	return c, nil
}

func (m *multiCharFixture) FindByIDForUpdate(ctx context.Context, id string) (corecharacter.Character, error) {
	return m.FindByID(ctx, id)
}

func (m *multiCharFixture) Update(_ context.Context, char corecharacter.Character) error {
	m.chars[char.ID] = char
	return nil
}

type inProgressTestEngine struct{}

func (inProgressTestEngine) ResolvePartyBattle(_ corebattle.PartyBattleRequest) (corebattle.PartyBattleResult, error) {
	return corebattle.PartyBattleResult{}, errors.New("unexpected battle call")
}

func TestPvPLeaveUnconfigured(t *testing.T) {
	f := newGatewayFixture(t)
	router := f.router("rescue_request", timer.NewService(nil), WithPvP(nil))
	status, raw := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"pvp_leave","params":{"room_id":"room"}}`, nil)
	if status != 501 || f.queryCalls != 0 {
		t.Fatalf("unconfigured command queried or executed: %d %s", status, raw)
	}
}
