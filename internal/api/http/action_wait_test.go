package http

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/playercontext"
	vk "github.com/witchcraze/party2re/internal/valkey"
)

func actionWaitRouter(t *testing.T, destination string) (*bankGatewayFixture, timer.Service, http.Handler) {
	t.Helper()
	return actionWaitRouterWith(t, destination, timer.NewService(nil), playercontext.DefaultActionWait)
}

func actionWaitRouterWith(t *testing.T, destination string, timers timer.Service, wait time.Duration, opts ...Option) (*bankGatewayFixture, timer.Service, http.Handler) {
	t.Helper()
	f := newBankGatewayFixture(t)
	store := &gatewayNavigationStore{selection: playercontext.Selection{Destination: destination}}
	pc := playercontext.NewService(f.gatewayFixture, f.gatewayFixture, timers,
		playercontext.WithNavigation(store, playercontext.SceneDefinition{ID: "town"}, playercontext.SceneDefinition{ID: "bank", Parent: "town"}),
		playercontext.WithActionWait(timers, wait))
	h, err := NewHandler(f.gatewayFixture, f.gatewayFixture, &struct{ AdventureService }{}, &struct{ ShopService }{},
		append([]Option{WithBank(&bankSceneService{BankService: f, service: f.service}), WithPlayerContext(pc), func(h *Handler) { h.homes = f.gatewayFixture }}, opts...)...)
	if err != nil {
		t.Fatal(err)
	}
	return f, timers, h.Router()
}

func actionErrorCode(t *testing.T, raw map[string]json.RawMessage) string {
	t.Helper()
	var detail ErrorDetail
	if err := json.Unmarshal(raw["error"], &detail); err != nil {
		t.Fatal(err)
	}
	return detail.Code
}

func waitMS(t *testing.T, raw map[string]json.RawMessage) int64 {
	t.Helper()
	var observation PlayerContextResponse
	if err := json.Unmarshal(raw["context"], &observation); err != nil {
		t.Fatal(err)
	}
	return observation.Character.ActionWaitMS
}

const depositOne = `{"action":"bank_deposit","params":{"amount":1}}`

func TestSuccessfulCommandStartsActionWaitThatBlocksEveryAction(t *testing.T) {
	f, timers, router := actionWaitRouter(t, "bank")
	status, raw := gatewayRequest(t, router, "hero", "session", "application/json", depositOne, nil)
	if status != 200 {
		t.Fatalf("first deposit: %d %s", status, raw["error"])
	}
	if ms := waitMS(t, raw); ms <= 0 || ms > playercontext.DefaultActionWait.Milliseconds() {
		t.Fatalf("refreshed context action_wait_ms = %d", ms)
	}
	for _, body := range []string{depositOne, `{"action":"scene_back","params":{}}`} {
		status, raw = gatewayRequest(t, router, "hero", "session", "application/json", body, nil)
		if status != http.StatusConflict || actionErrorCode(t, raw) != "ACTION_WAIT" {
			t.Fatalf("%s during wait: %d %s", body, status, raw["error"])
		}
	}
	if f.depositCalls != 1 {
		t.Fatalf("a waiting command reached the service: %d calls", f.depositCalls)
	}
	if err := timers.ReleaseLock(context.Background(), timer.CategoryActionWait, "hero"); err != nil {
		t.Fatal(err)
	}
	if status, raw = gatewayRequest(t, router, "hero", "session", "application/json", depositOne, nil); status != 200 || f.depositCalls != 2 {
		t.Fatalf("after the wait: %d %s", status, raw["error"])
	}
}

func TestRejectedCommandAndNavigationDoNotStartActionWait(t *testing.T) {
	f, timers, router := actionWaitRouter(t, "town")
	if status, raw := gatewayRequest(t, router, "hero", "session", "application/json", depositOne, nil); status != 409 || actionErrorCode(t, raw) != "ACTION_UNAVAILABLE" || waitMS(t, raw) != 0 || f.depositCalls != 0 {
		t.Fatalf("deposit from town: %d %s (calls %d)", status, raw["error"], f.depositCalls)
	}
	if status, raw := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"scene_enter","params":{"destination":"bank"}}`, nil); status != 200 || waitMS(t, raw) != 0 {
		t.Fatalf("scene_enter: %d %s", status, raw["error"])
	}
	status, raw := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"bank_deposit","params":{"amount":999999}}`, nil)
	if status < 400 || status >= 500 || actionErrorCode(t, raw) == "ACTION_WAIT" {
		t.Fatalf("expected a known service rejection, got %d %s", status, raw["error"])
	}
	if remaining, err := timers.GetRemainingLock(context.Background(), timer.CategoryActionWait, "hero"); err != nil || remaining != 0 {
		t.Fatalf("rejection started a wait: %v %v", remaining, err)
	}
	if status, raw = gatewayRequest(t, router, "hero", "session", "application/json", depositOne, nil); status != 200 || f.depositCalls != 2 {
		t.Fatalf("deposit after rejection: %d %s (calls %d)", status, raw["error"], f.depositCalls)
	}
}

func TestActionWaitIsReadOnlyForGET(t *testing.T) {
	_, timers, router := actionWaitRouter(t, "bank")
	if err := timers.SetLock(context.Background(), timer.CategoryActionWait, "hero", time.Minute); err != nil {
		t.Fatal(err)
	}
	status, observation := navigationGET(t, router)
	if status != 200 || observation.Character.ActionWaitMS <= 0 {
		t.Fatalf("GET during wait: %d %+v", status, observation.Character)
	}
	for _, a := range observation.AvailableActions {
		if a.Action == "bank_deposit" || a.Action == "scene_back" {
			t.Fatalf("%s offered during the wait", a.Action)
		}
	}
	if remaining, _ := timers.GetRemainingLock(context.Background(), timer.CategoryActionWait, "hero"); remaining <= 30*time.Second {
		t.Fatalf("GET changed the wait: %v", remaining)
	}
}

// Real Valkey: SET NX EX claim, rejection during the wait, release on a
// rejected command and expiry, across sequential commands.
func TestActionWaitWithRealValkey(t *testing.T) {
	if os.Getenv("PARTY2_VALKEY_ADDR") == "" {
		t.Skip("PARTY2_VALKEY_ADDR is not configured")
	}
	client, err := vk.NewClient()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Close)
	timers := timer.NewService(client)
	ctx := context.Background()
	_ = timers.ReleaseLock(ctx, timer.CategoryActionWait, "hero")
	t.Cleanup(func() { _ = timers.ReleaseLock(context.Background(), timer.CategoryActionWait, "hero") })
	f, _, router := actionWaitRouterWith(t, "bank", timers, time.Second)

	status, raw := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"bank_deposit","params":{"amount":999999}}`, nil)
	if status < 400 || status >= 500 || actionErrorCode(t, raw) == "ACTION_WAIT" {
		t.Fatalf("expected a service rejection: %d %s", status, raw["error"])
	}
	if status, raw = gatewayRequest(t, router, "hero", "session", "application/json", depositOne, nil); status != 200 {
		t.Fatalf("a rejected command must release its claim: %d %s", status, raw["error"])
	}
	if status, raw = gatewayRequest(t, router, "hero", "session", "application/json", depositOne, nil); status != http.StatusConflict || actionErrorCode(t, raw) != "ACTION_WAIT" {
		t.Fatalf("during the wait: %d %s", status, raw["error"])
	}
	time.Sleep(1200 * time.Millisecond)
	if status, raw = gatewayRequest(t, router, "hero", "session", "application/json", depositOne, nil); status != 200 || f.depositCalls != 3 {
		t.Fatalf("after expiry: %d %s (calls %d)", status, raw["error"], f.depositCalls)
	}
}
