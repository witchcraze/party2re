package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"slices"
	"testing"

	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/depot"
	"github.com/witchcraze/party2re/internal/playercontext"
)

type depotSendMoneyFixture struct {
	*depotSaleFixture
	amount int
}

func (f *depotSendMoneyFixture) SendMoney(ctx context.Context, actor, recipient string, amount int) (depot.Depot, error) {
	f.recipient, f.amount = recipient, amount
	return f.Expand(ctx, actor)
}

type depotSendMoneyRig struct {
	*depotSendMoneyFixture
	store  *gatewayNavigationStore
	router http.Handler
	t      *testing.T
}

func (r depotSendMoneyRig) send(params string) (int, map[string]json.RawMessage) {
	return gatewayRequest(r.t, r.router, "hero", "session", "application/json", saleGatewayBody("depot_send_money", params), nil)
}

func depotSendMoneyRouter(t *testing.T) depotSendMoneyRig {
	t.Helper()
	f := &depotSendMoneyFixture{depotSaleFixture: &depotSaleFixture{depotExpansionFixture: &depotExpansionFixture{gatewayFixture: newGatewayFixture(t), dep: depot.Depot{CharacterID: "hero", Capacity: 10}}}}
	store := &gatewayNavigationStore{selection: playercontext.Selection{Destination: "depot"}}
	pc := playercontext.NewService(f.gatewayFixture, f.gatewayFixture, timer.NewService(nil), playercontext.WithNavigation(store,
		playercontext.SceneDefinition{ID: "town"}, playercontext.SceneDefinition{ID: "depot", Parent: "town", Pageable: true}, playercontext.SceneDefinition{ID: "bank", Parent: "town"}))
	h, err := NewHandler(f.gatewayFixture, f.gatewayFixture, &struct{ AdventureService }{}, &struct{ ShopService }{}, WithDepot(f), WithPlayerContext(pc), func(h *Handler) { h.homes = f.gatewayFixture })
	if err != nil {
		t.Fatal(err)
	}
	return depotSendMoneyRig{depotSendMoneyFixture: f, store: store, router: h.Router(), t: t}
}

func TestDepotSendMoneyGatewayRejectsInvalidInputsBeforeExecution(t *testing.T) {
	for _, params := range []string{"", `null`, `{}`, `[]`, `{"amount":1}`, `{"recipient_character_id":"r"}`,
		`{"amount":null,"recipient_character_id":"r"}`, `{"amount":"1","recipient_character_id":"r"}`, `{"amount":1.5,"recipient_character_id":"r"}`,
		`{"amount":1,"recipient_character_id":null}`, `{"amount":1,"recipient_character_id":1}`,
		`{"amount":1,"recipient_character_id":"r","character_id":"other"}`, `{"amount":1,"recipient_character_id":"r","player_id":"other"}`,
		`{"amount":1,"recipient_character_id":"r","item_id":"one"}`} {
		t.Run(params, func(t *testing.T) {
			f := depotSendMoneyRouter(t)
			status, got := f.send(params)
			if status != 400 || f.executions != 0 || f.queryCalls != 0 || f.store.writes != 0 {
				t.Fatalf("invalid params executed: %d %s", status, got)
			}
			assertGatewayError(t, got, "INVALID_ACTION_PARAMS")
		})
	}
}

func TestDepotSendMoneyGatewayDelegatesExplicitIntent(t *testing.T) {
	for _, failure := range []string{"", "query"} {
		t.Run("success/"+failure, func(t *testing.T) {
			f := depotSendMoneyRouter(t)
			f.afterExecute = func() {
				if failure == "query" {
					f.queryErr = errors.New("private query")
				}
			}
			status, got := f.send(`{"amount":40,"recipient_character_id":" recipient "}`)
			var result sendMoneyResponse
			if err := json.Unmarshal(got["result"], &result); err != nil {
				t.Fatal(err)
			}
			want := sendMoneyResponse{Depot: toDepotResponse(f.dep), RecipientCharacterID: "recipient", Amount: 40}
			if status != 200 || f.executions != 1 || f.recipient != "recipient" || f.amount != 40 || !reflect.DeepEqual(result, want) {
				t.Fatalf("lost intent: %d %s recipient=%q amount=%d", status, got, f.recipient, f.amount)
			}
			if failure == "" {
				assertGatewayContext(t, got)
			} else {
				assertLoopRefreshFailure(t, got)
			}
			f.queryErr = nil
			if status, _ := navigationGET(t, f.router); status != 200 || f.executions != 1 || f.store.writes != 0 {
				t.Fatal("GET replayed the remittance")
			}
		})
	}
	// Typed nonpositive or excessive amounts reach service validation.
	for _, tc := range []struct {
		params string
		err    error
		status int
		code   string
	}{
		{`{"amount":0,"recipient_character_id":"r"}`, depot.ErrInvalidAmount, 400, "DEPOT_INVALID_AMOUNT"},
		{`{"amount":-1,"recipient_character_id":"r"}`, depot.ErrInvalidAmount, 400, "DEPOT_INVALID_AMOUNT"},
		{`{"amount":999999999,"recipient_character_id":"r"}`, depot.ErrInsufficientFunds, 400, "DEPOT_INSUFFICIENT_FUNDS"},
		{`{"amount":1,"recipient_character_id":"hero"}`, depot.ErrSelfTransferNotAllowed, 400, "DEPOT_SELF_TRANSFER"},
		{`{"amount":1,"recipient_character_id":"ghost"}`, depot.ErrRecipientNotFound, 404, "DEPOT_RECIPIENT_NOT_FOUND"},
		{`{"amount":1,"recipient_character_id":"full"}`, depot.ErrRecipientDepotFull, 400, "DEPOT_RECIPIENT_FULL"},
		{`{"amount":1,"recipient_character_id":"r"}`, errors.New("private storage"), 500, "EXECUTION_FAILED"},
	} {
		t.Run(tc.code+tc.params, func(t *testing.T) {
			f := depotSendMoneyRouter(t)
			f.executionErr = fmt.Errorf("wrapped: %w", tc.err)
			status, got := f.send(tc.params)
			if status != tc.status || f.executions != 1 {
				t.Fatalf("error mapping: %d %s", status, got)
			}
			assertGatewayError(t, got, tc.code)
		})
	}
}

func TestDepotSendMoneyGatewayGuardsAndDiscovery(t *testing.T) {
	f := depotSendMoneyRouter(t)
	f.store.selection.Destination = "bank"
	if status, got := f.send(`{"amount":1,"recipient_character_id":"r"}`); status != 409 || f.executions != 0 {
		t.Fatalf("outside Depot: %d %s", status, got)
	}
	f.store.selection.Destination = "depot"
	f.char.PendingWake = true
	if status, got := f.send(`{"amount":1,"recipient_character_id":"r"}`); status != 409 || f.executions != 0 {
		t.Fatalf("pending wake: %d %s", status, got)
	}
	f.char.PendingWake = false
	status, got := navigationGET(t, f.router)
	idx := slices.IndexFunc(got.AvailableActions, func(a ContextAction) bool { return a.Action == "depot_send_money" })
	if status != 200 || idx < 0 {
		t.Fatalf("missing discovery: %d %+v", status, got.AvailableActions)
	}
	a := got.AvailableActions[idx]
	var schema struct {
		AdditionalProperties bool                             `json:"additionalProperties"`
		Properties           map[string]struct{ Type string } `json:"properties"`
		Required             []string                         `json:"required"`
	}
	if err := json.Unmarshal(a.ParamsSchema, &schema); err != nil {
		t.Fatal(err)
	}
	fields := []string{"amount", "recipient_character_id"}
	if !slices.Equal(a.RequiredParams, fields) || !slices.Equal(schema.Required, fields) || schema.AdditionalProperties || len(schema.Properties) != 2 ||
		schema.Properties["amount"].Type != "integer" || schema.Properties["recipient_character_id"].Type != "string" || len(a.ParamsTemplate) != 0 {
		t.Fatalf("incorrect params: %+v %s", a, a.ParamsSchema)
	}
	if resolved := NewActionURLResolver().Resolve("hero", "depot_send_money", "", ""); resolved.Method != "POST" || resolved.URL != "/api/v1/characters/hero/actions" {
		t.Fatalf("link: %+v", resolved)
	}
	f.char.Money = 0
	if _, got := navigationGET(t, f.router); slices.ContainsFunc(got.AvailableActions, func(a ContextAction) bool { return a.Action == "depot_send_money" }) {
		t.Fatal("empty wallet offered a remittance")
	}
	if f.executions != 0 || f.store.writes != 0 {
		t.Fatal("GET mutated state")
	}
}
