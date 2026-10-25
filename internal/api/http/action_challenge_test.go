package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/witchcraze/party2re/internal/challenge"
	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/playercontext"
)

type challengeGatewayFixture struct {
	*gatewayFixture
	ChallengeService
	facts               []playercontext.Activity
	receipt             *challenge.CommandResult
	exitErr, receiptErr error
}

func newChallengeGatewayFixture(t *testing.T) *challengeGatewayFixture {
	return &challengeGatewayFixture{gatewayFixture: newGatewayFixture(t), facts: []playercontext.Activity{{Kind: "challenge", ID: "run", Revision: 3, Phase: "active", Actions: []string{"challenge_retire"}}}}
}

func (f *challengeGatewayFixture) ExitSession(ctx context.Context, actor, run string, revision int) (*challenge.CommandResult, error) {
	f.checkContext(ctx)
	f.executions++
	if actor != "hero" || run != "run" || revision != 3 {
		f.t.Fatalf("wrong exit identity: %s %s %d", actor, run, revision)
	}
	if f.exitErr != nil && !errors.Is(f.exitErr, challenge.ErrRecoveryRequired) {
		return nil, f.exitErr
	}
	f.receipt = &challenge.CommandResult{ActorID: actor, Action: "exit", ExpectedRevision: revision, Session: challenge.ChallengeSession{ID: run, Revision: 4, Status: challenge.StatusClaimed}}
	f.facts = nil
	if f.afterExecute != nil {
		f.afterExecute()
	}
	return f.receipt, f.exitErr
}

func (f *challengeGatewayFixture) GetCommandResult(ctx context.Context, actor, run string, revision int) (*challenge.CommandResult, error) {
	f.checkContext(ctx)
	if f.receiptErr != nil {
		return nil, f.receiptErr
	}
	if f.receipt == nil || actor != f.receipt.ActorID || run != f.receipt.Session.ID || revision != f.receipt.ExpectedRevision {
		return nil, challenge.ErrForbidden
	}
	return f.receipt, nil
}

func (f *challengeGatewayFixture) router() http.Handler {
	pc := playercontext.NewService(f.gatewayFixture, f.gatewayFixture, timer.NewService(nil), playercontext.WithActivities(func(context.Context, string, string) ([]playercontext.Activity, error) { return f.facts, nil }))
	h, err := NewHandler(f.gatewayFixture, f.gatewayFixture, &struct{ AdventureService }{}, &struct{ ShopService }{}, WithChallenge(f), WithPlayerContext(pc), func(h *Handler) { h.homes = f.gatewayFixture })
	if err != nil {
		f.t.Fatal(err)
	}
	return h.Router()
}

func TestChallengeGatewayExitAndKnownResult(t *testing.T) {
	for _, pending := range []bool{false, true} {
		for _, refreshFailure := range []bool{false, true} {
			t.Run(fmt.Sprintf("pending=%t/refreshFailure=%t", pending, refreshFailure), func(t *testing.T) {
				f := newChallengeGatewayFixture(t)
				if pending {
					f.exitErr = errors.Join(challenge.ErrRecoveryRequired, errors.New("secret publication failure"))
				}
				if refreshFailure {
					f.afterExecute = func() { f.profileErr = errors.New("profile unavailable") }
				}
				router := f.router()
				status, observation := navigationGET(t, router)
				if status != 200 || len(observation.AvailableActions) != 1 || observation.AvailableActions[0].ParamsTemplate["expected_revision"] != float64(3) {
					t.Fatalf("missing typed exit: %d %+v", status, observation)
				}
				f.expectedContext = context.WithValue(context.Background(), struct{}{}, "request")
				status, got := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"challenge_retire","params":{"session_id":"run","expected_revision":3}}`, f.expectedContext)
				var result struct {
					challenge.CommandResult
					RecoveryPending bool `json:"recovery_pending"`
				}
				if err := json.Unmarshal(got["result"], &result); err != nil {
					t.Fatal(err)
				}
				if status != 200 || string(got["success"]) != "true" || f.executions != 1 || result.ActorID != "hero" || result.Action != "exit" || result.RecoveryPending != pending {
					t.Fatalf("lost known result: %d %s", status, got)
				}
				if refreshFailure && (string(got["context"]) != "null" || got["context_error"] == nil) {
					t.Fatalf("lost refresh error: %s", got)
				}
				f.profileErr = nil
				f.expectedContext = nil
				if status, _ := navigationGET(t, router); status != 200 || f.executions != 1 {
					t.Fatal("GET replayed exit")
				}
				status, got = gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"challenge_retire","params":{"session_id":"run","expected_revision":3}}`, f.expectedContext)
				if status != 200 || string(got["success"]) != "true" {
					t.Fatalf("departed actor cannot replay owned receipt: %d %s", status, got)
				}
			})
		}
	}
}

func TestChallengeGatewayBoundary(t *testing.T) {
	for _, params := range []string{`{}`, `null`, `{"session_id":"run"}`, `{"session_id":null,"expected_revision":3}`, `{"session_id":"run","expected_revision":null}`, `{"session_id":1,"expected_revision":3}`, `{"session_id":"run","expected_revision":"3"}`, `{"session_id":"run","expected_revision":1.5}`, `{"session_id":"run","expected_revision":3,"character_id":"other"}`} {
		f := newChallengeGatewayFixture(t)
		status, _ := gatewayRequest(t, f.router(), "hero", "session", "application/json", `{"action":"challenge_retire","params":`+params+`}`, nil)
		if status != 400 || f.executions != 0 {
			t.Fatalf("invalid params reached exit: %s status=%d", params, status)
		}
	}
	f := newChallengeGatewayFixture(t)
	if status, _ := gatewayRequest(t, f.router(), "other", "session", "application/json", `{"action":"challenge_retire","params":{"session_id":"run","expected_revision":3}}`, nil); status != 403 || f.executions != 0 {
		t.Fatal("unowned actor reached exit")
	}
	f.facts = nil
	if status, _ := gatewayRequest(t, f.router(), "hero", "session", "application/json", `{"action":"challenge_retire","params":{"session_id":"run","expected_revision":3}}`, nil); status != 409 || f.executions != 0 {
		t.Fatal("missing receipt bypassed eligibility")
	}
}

func TestChallengeGatewayRejectionsAndUnknownOutcome(t *testing.T) {
	for _, tc := range []struct {
		err    error
		status int
		code   string
	}{
		{challenge.ErrRevisionConflict, 409, "CHALLENGE_REVISION_CONFLICT"},
		{challenge.ErrForbidden, 403, "CHALLENGE_FORBIDDEN"},
		{challenge.ErrSessionNotFound, 404, "CHALLENGE_NOT_FOUND"},
		{challenge.ErrSessionNotActive, 422, "CHALLENGE_NOT_ACTIVE"},
		{challenge.ErrOutcomeUnknown, 500, "EXECUTION_FAILED"},
		{errors.New("secret SQL failure"), 500, "EXECUTION_FAILED"},
	} {
		t.Run(tc.code, func(t *testing.T) {
			f := newChallengeGatewayFixture(t)
			f.exitErr = fmt.Errorf("wrapped: %w", tc.err)
			status, got := gatewayRequest(t, f.router(), "hero", "session", "application/json", `{"action":"challenge_retire","params":{"session_id":"run","expected_revision":3}}`, nil)
			if status != tc.status || f.executions != 1 {
				t.Fatalf("status=%d %s", status, got)
			}
			assertGatewayError(t, got, tc.code)
			if tc.status == 500 {
				if _, ok := got["context"]; ok {
					t.Fatal("unknown outcome exposed context")
				}
			} else {
				assertGatewayContext(t, got)
			}
		})
	}
	f := newChallengeGatewayFixture(t)
	f.facts = nil
	f.receiptErr = errors.New("secret receipt store failure")
	status, got := gatewayRequest(t, f.router(), "hero", "session", "application/json", `{"action":"challenge_retire","params":{"session_id":"run","expected_revision":3}}`, nil)
	if status != 500 || f.executions != 0 {
		t.Fatalf("receipt error permitted execution: %d %s", status, got)
	}
	assertGatewayError(t, got, "ACTION_PREFLIGHT_FAILED")
}

func TestChallengeGatewayExitRecoveryAndInvalidIdentity(t *testing.T) {
	f := newChallengeGatewayFixture(t)
	f.char.Stats.HP, f.char.Tired, f.char.PendingWake = 0, 100, true
	status, got := gatewayRequest(t, f.router(), "hero", "session", "application/json", `{"action":"challenge_retire","params":{"session_id":"run","expected_revision":3}}`, nil)
	if status != 200 || f.executions != 1 || !f.char.PendingWake {
		t.Fatalf("entry gates blocked personal exit or woke actor: %d %s", status, got)
	}
	for _, params := range []string{`{"session_id":"","expected_revision":3}`, `{"session_id":"run","expected_revision":0}`, `{"session_id":"run","expected_revision":-1}`} {
		f := newChallengeGatewayFixture(t)
		status, got := gatewayRequest(t, f.router(), "hero", "session", "application/json", `{"action":"challenge_retire","params":`+params+`}`, nil)
		if status != 409 || f.executions != 0 {
			t.Fatalf("invalid identity reached domain: %d %s", status, got)
		}
		assertGatewayError(t, got, "CHALLENGE_REVISION_CONFLICT")
	}
}
