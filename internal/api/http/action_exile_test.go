package http

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/exile"
	"github.com/witchcraze/party2re/internal/playercontext"
)

type exileGatewayFixture struct {
	*gatewayFixture
	calls []string
}

func (f *exileGatewayFixture) GetBoard(_ context.Context, actor string) (exile.Board, error) {
	if actor != f.char.ID {
		f.t.Fatal("wrong board reader")
	}
	return exile.Board{Eligible: true, Petitions: []exile.Petition{{ID: "p1", TargetName: "Bad", Votes: []exile.Vote{}}}}, nil
}

func (f *exileGatewayFixture) Petition(_ context.Context, actor, target, reason string, now time.Time) (exile.Petition, error) {
	f.executions++
	f.calls = append(f.calls, fmt.Sprintf("petition %s %s %s %v", actor, target, reason, now.Location() == time.UTC))
	return exile.Petition{ID: "p2", TargetName: target, Reason: reason, Votes: []exile.Vote{}}, f.executionErr
}

func (f *exileGatewayFixture) Vote(_ context.Context, actor, target string, approve bool, _ time.Time) (exile.VoteResult, error) {
	f.executions++
	f.calls = append(f.calls, fmt.Sprintf("vote %s %s %v", actor, target, approve))
	return exile.VoteResult{Withdrawn: []exile.Petition{}, Approvals: 2}, f.executionErr
}

func exileGatewayRouter(t *testing.T, destination string) (*exileGatewayFixture, http.Handler) {
	t.Helper()
	f := &exileGatewayFixture{gatewayFixture: newGatewayFixture(t)}
	store := &gatewayNavigationStore{selection: playercontext.Selection{Destination: destination}}
	pc := playercontext.NewService(f.gatewayFixture, f.gatewayFixture, timer.NewService(nil), playercontext.WithNavigation(store,
		playercontext.SceneDefinition{ID: "town"}, playercontext.SceneDefinition{ID: "exile", Parent: "town"}, playercontext.SceneDefinition{ID: "bank", Parent: "town"}))
	h, err := NewHandler(f.gatewayFixture, f.gatewayFixture, &struct{ AdventureService }{}, &struct{ ShopService }{},
		WithExile(f), WithPlayerContext(pc), func(h *Handler) { h.homes = f.gatewayFixture })
	if err != nil {
		t.Fatal(err)
	}
	return f, h.Router()
}

func TestExileGatewayObservationAndCommands(t *testing.T) {
	f, router := exileGatewayRouter(t, "exile")
	status, observed := navigationGET(t, router)
	data := decodeShopScene[ExileSceneData](t, observed)
	if status != 200 || data.Parent != "town" || !data.Board.Eligible || len(data.Board.Petitions) != 1 || observed.Scene.Support.Observation != "details" || f.executions != 0 {
		t.Fatalf("missing exile board: %d %+v", status, observed)
	}
	for _, id := range []string{"exile_petition", "exile_approve", "exile_oppose"} {
		if !slices.ContainsFunc(observed.AvailableActions, func(a ContextAction) bool { return a.Action == id }) {
			t.Fatalf("missing %s: %+v", id, observed.AvailableActions)
		}
	}
	for _, body := range []string{
		`{"action":"exile_petition","params":{"target_name":"Bad","reason":"暴言"}}`,
		`{"action":"exile_approve","params":{"target_name":"Bad"}}`,
		`{"action":"exile_oppose","params":{"target_name":"Bad"}}`,
	} {
		if status, got := gatewayRequest(t, router, "hero", "session", "application/json", body, nil); status != 200 {
			t.Fatalf("%s: %d %s", body, status, got)
		}
	}
	want := []string{"petition hero Bad 暴言 true", "vote hero Bad true", "vote hero Bad false"}
	if !slices.Equal(f.calls, want) {
		t.Fatalf("calls %v", f.calls)
	}
}

func TestExileGatewayRejections(t *testing.T) {
	for _, tc := range []struct {
		err    error
		status int
		code   string
	}{
		{exile.ErrCancelLocked, 409, "EXILE_WITHDRAWAL_LOCKED"},
		{exile.ErrJobLevelTooLow, 409, "EXILE_JOB_LEVEL_TOO_LOW"},
		{exile.ErrPetitionLimit, 409, "EXILE_PETITION_LIMIT"},
		{exile.ErrPetitionNotFound, 404, "EXILE_PETITION_NOT_FOUND"},
		{fmt.Errorf("%w: too long", exile.ErrInvalidReason), 422, "EXILE_INVALID_REASON"},
	} {
		f, router := exileGatewayRouter(t, "exile")
		f.executionErr = tc.err
		status, got := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"exile_oppose","params":{"target_name":"Bad"}}`, nil)
		if status != tc.status {
			t.Fatalf("%v: %d %s", tc.err, status, got)
		}
		assertGatewayError(t, got, tc.code)
	}
	for _, params := range []string{`{}`, `{"target_name":1}`, `{"target_name":"Bad","reason":"x"}`} {
		f, router := exileGatewayRouter(t, "exile")
		status, got := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"exile_approve","params":`+params+`}`, nil)
		if status != 400 || f.executions != 0 {
			t.Fatalf("invalid params %s: %d %s", params, status, got)
		}
		assertGatewayError(t, got, "INVALID_ACTION_PARAMS")
	}
	f, router := exileGatewayRouter(t, "bank")
	if status, got := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"exile_approve","params":{"target_name":"Bad"}}`, nil); status != 409 || f.executions != 0 {
		t.Fatalf("voted outside the board: %d %s", status, got)
	}
}
