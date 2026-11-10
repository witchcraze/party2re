package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/witchcraze/party2re/internal/auction"
	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/playercontext"
)

type playerInspectFixture struct {
	*gatewayFixture
	actor, id, name string
	err             error
}

func (f *playerInspectFixture) InspectPlayer(ctx context.Context, actorID, targetID, targetName string) (PlayerInspection, error) {
	f.executions++
	f.actor, f.id, f.name = actorID, targetID, targetName
	return PlayerInspection{InspectResult: auction.InspectResult{CharacterID: "peer", Name: "相手"}, Gems: []string{}}, f.err
}

func playerInspectRouter(t *testing.T) (*playerInspectFixture, http.Handler) {
	t.Helper()
	f := &playerInspectFixture{gatewayFixture: newGatewayFixture(t)}
	h, err := NewHandler(f.gatewayFixture, f.gatewayFixture, &struct{ AdventureService }{}, &struct{ ShopService }{},
		WithPlayerInspect(f), WithPlayerContext(playercontext.NewService(f.gatewayFixture, f.gatewayFixture, timer.NewService(nil))), func(h *Handler) { h.homes = f.gatewayFixture })
	if err != nil {
		t.Fatal(err)
	}
	return f, h.Router()
}

func TestPlayerInspectGateway(t *testing.T) {
	for _, tc := range []struct{ params, id, name string }{
		{`{}`, "", ""},
		{`{"target_character_id":"peer"}`, "peer", ""},
		{`{"target_character_name":"相手"}`, "", "相手"},
	} {
		t.Run(tc.params, func(t *testing.T) {
			f, router := playerInspectRouter(t)
			status, got := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"player_inspect","params":`+tc.params+`}`, nil)
			var result PlayerInspection
			if err := json.Unmarshal(got["result"], &result); err != nil {
				t.Fatal(err)
			}
			if status != 200 || f.executions != 1 || f.actor != "hero" || f.id != tc.id || f.name != tc.name || result.Name != "相手" {
				t.Fatalf("status=%d calls=%d actor=%q id=%q name=%q: %s", status, f.executions, f.actor, f.id, f.name, got)
			}
		})
	}
}

func TestPlayerInspectGatewayStrictParamsAndFailures(t *testing.T) {
	for _, params := range []string{`null`, `[]`, `{"character_id":"other"}`, `{"extra":1}`, `{"target_character_id":null}`, `{"target_character_name":7}`} {
		f, router := playerInspectRouter(t)
		status, got := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"player_inspect","params":`+params+`}`, nil)
		if status != 400 || f.executions != 0 {
			t.Fatalf("%s: status=%d calls=%d: %s", params, status, f.executions, got)
		}
		assertGatewayError(t, got, "INVALID_ACTION_PARAMS")
	}
	// A storage failure is never reported as a missing target.
	f, router := playerInspectRouter(t)
	f.err = errors.New("private storage failure")
	status, got := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"player_inspect","params":{"target_character_id":"peer"}}`, nil)
	if status != 500 {
		t.Fatalf("status=%d: %s", status, got)
	}
	assertGatewayError(t, got, "EXECUTION_FAILED")
}
