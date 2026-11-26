package http

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/gvg"
	"github.com/witchcraze/party2re/internal/playercontext"
	"github.com/witchcraze/party2re/internal/pvp"
)

type gvgLeaveStub struct {
	GvGService
	leave func(ctx context.Context, charID, roomID string) (gvg.LeaveResult, error)
}

func (s gvgLeaveStub) LeaveRoom(ctx context.Context, charID, roomID string) (gvg.LeaveResult, error) {
	return s.leave(ctx, charID, roomID)
}

type pvpLeaveStub struct {
	PvPService
	res pvp.LeaveResult
}

func (s pvpLeaveStub) LeaveRoom(context.Context, string, string) (pvp.LeaveResult, error) {
	return s.res, nil
}

func TestGvGLeaveGateway(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		res        gvg.LeaveResult
		err        error
		status     int
		calls      int
		notice     string
	}{
		{"leave", `{"action":"gvg_leave","params":{"room_id":"g1"}}`, gvg.LeaveResult{}, nil, 200, 1, ""},
		{"paid delivery", `{"action":"gvg_leave","params":{"room_id":"g1"}}`, gvg.LeaveResult{Delivery: &gvg.DeliveryOutcome{ItemName: "癒しの湧き水", Paid: true}}, nil, 200, 1, "酒場から癒しの湧き水が届きました！"},
		{"unpaid delivery", `{"action":"gvg_leave","params":{"room_id":"g1"}}`, gvg.LeaveResult{Delivery: &gvg.DeliveryOutcome{ItemName: "完熟トマトジュース"}}, nil, 200, 1, "酒場から完熟トマトジュースが届きましたが、お金が足りませんでした"},
		{"missing room", `{"action":"gvg_leave","params":{}}`, gvg.LeaveResult{}, nil, 400, 0, ""},
		{"unknown param", `{"action":"gvg_leave","params":{"room_id":"g1","team":"red"}}`, gvg.LeaveResult{}, nil, 400, 0, ""},
		{"room gone", `{"action":"gvg_leave","params":{"room_id":"g1"}}`, gvg.LeaveResult{}, gvg.ErrRoomNotFound, 404, 1, ""},
		{"not member", `{"action":"gvg_leave","params":{"room_id":"g1"}}`, gvg.LeaveResult{}, gvg.ErrCharacterNotInRoom, 403, 1, ""},
		{"store down", `{"action":"gvg_leave","params":{"room_id":"g1"}}`, gvg.LeaveResult{}, errors.New("down"), 500, 1, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newGatewayFixture(t)
			calls := 0
			stub := gvgLeaveStub{leave: func(_ context.Context, charID, roomID string) (gvg.LeaveResult, error) {
				calls++
				if charID != "hero" || roomID != "g1" {
					t.Errorf("LeaveRoom(%q, %q)", charID, roomID)
				}
				return tc.res, tc.err
			}}
			pc := playercontext.NewService(f, f, timer.NewService(nil), playercontext.WithActivities(func(context.Context, string, string) ([]playercontext.Activity, error) {
				return []playercontext.Activity{{Kind: "gvg", ID: "g1", Phase: gvg.StatusInProgress, Round: 2, Role: "member", Actions: []string{"gvg_leave"}}}, nil
			}))
			router := f.router("rescue_request", timer.NewService(nil), WithGvG(stub), WithPlayerContext(pc))
			status, raw := gatewayRequest(t, router, "hero", "session", "application/json", tc.body, nil)
			if status != tc.status || calls != tc.calls {
				t.Fatalf("status %d calls %d: %s", status, calls, raw)
			}
			if status == 200 {
				assertLeaveResult(t, raw, "戦闘から逃げ出しました", tc.notice)
			}
		})
	}
}

func TestPvPLeaveResult(t *testing.T) {
	for _, tc := range []struct {
		name            string
		res             pvp.LeaveResult
		message, notice string
	}{
		{"pre-start refund", pvp.LeaveResult{RefundedBet: 50}, "戦闘から逃げ出しました\n賭け金の 50 Gが返金されました", ""},
		{"after start with delivery", pvp.LeaveResult{Delivery: &pvp.DeliveryOutcome{ItemName: "癒しの湧き水", Paid: true}}, "戦闘から逃げ出しました", "酒場から癒しの湧き水が届きました！"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newGatewayFixture(t)
			pc := playercontext.NewService(f, f, timer.NewService(nil), playercontext.WithActivities(func(context.Context, string, string) ([]playercontext.Activity, error) {
				return []playercontext.Activity{{Kind: "pvp", ID: "r1", Phase: pvp.StatusRecruiting, Role: "member", Actions: []string{"pvp_leave"}}}, nil
			}))
			router := f.router("rescue_request", timer.NewService(nil), WithPvP(pvpLeaveStub{res: tc.res}), WithPlayerContext(pc))
			status, raw := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"pvp_leave","params":{"room_id":"r1"}}`, nil)
			if status != 200 {
				t.Fatalf("status %d %s", status, raw)
			}
			assertLeaveResult(t, raw, tc.message, tc.notice)
		})
	}
}

func assertLeaveResult(t *testing.T, raw map[string]json.RawMessage, message, notice string) {
	t.Helper()
	var result map[string]string
	if err := json.Unmarshal(raw["result"], &result); err != nil {
		t.Fatal(err)
	}
	got, has := result["delivery_notice"]
	if result["message"] != message || got != notice || has != (notice != "") {
		t.Fatalf("result = %v, want message %q notice %q", result, message, notice)
	}
}
