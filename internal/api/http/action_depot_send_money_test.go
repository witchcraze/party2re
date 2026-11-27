package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/depot"
	"github.com/witchcraze/party2re/internal/playercontext"
)

type depotSendMoneyFixture struct {
	*depotSaleFixture
	amount int
}

func (f *depotSendMoneyFixture) SendMoney(ctx context.Context, actor, recipient string, amount int) (depot.Receipt, error) {
	f.recipient, f.amount = recipient, amount
	return f.receipt(ctx, actor)
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

func depotSendMoneyRouter(t *testing.T, opts ...Option) depotSendMoneyRig {
	t.Helper()
	return depotSendMoneyRouterWith(t, nil, opts...)
}

func depotSendMoneyRouterWith(t *testing.T, pcOpts []func(*playercontext.Service), opts ...Option) depotSendMoneyRig {
	t.Helper()
	f := &depotSendMoneyFixture{depotSaleFixture: &depotSaleFixture{depotExpansionFixture: &depotExpansionFixture{gatewayFixture: newGatewayFixture(t), dep: depot.Depot{CharacterID: "hero", Capacity: 10}}}}
	store := &gatewayNavigationStore{selection: playercontext.Selection{Destination: "depot"}}
	timers := timer.NewService(nil)
	pc := playercontext.NewService(f.gatewayFixture, f.gatewayFixture, timers, append([]func(*playercontext.Service){playercontext.WithNavigation(store,
		playercontext.SceneDefinition{ID: "town"}, playercontext.SceneDefinition{ID: "depot", Parent: "town", Pageable: true}, playercontext.SceneDefinition{ID: "bank", Parent: "town"}, playercontext.SceneDefinition{ID: "auction", Parent: "town"})}, pcOpts...)...)
	h, err := NewHandler(f.gatewayFixture, f.gatewayFixture, &struct{ AdventureService }{}, &struct{ ShopService }{}, append([]Option{WithDepot(f), WithPlayerContext(pc), func(h *Handler) { h.homes = f.gatewayFixture }}, opts...)...)
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
	// Auction okuru is the same routine, so the venue offers the same command.
	f.store.selection.Destination = "auction"
	if status, got := f.send(`{"amount":1,"recipient_character_id":"r"}`); status != 200 || f.executions != 1 {
		t.Fatalf("auction venue: %d %s", status, got)
	}
	f.store.selection.Destination = "depot"
	f.char.PendingWake = true
	if status, got := f.send(`{"amount":1,"recipient_character_id":"r"}`); status != 409 || f.executions != 1 {
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
	f.char.Money = 0
	if _, got := navigationGET(t, f.router); slices.ContainsFunc(got.AvailableActions, func(a ContextAction) bool { return a.Action == "depot_send_money" }) {
		t.Fatal("empty wallet offered a remittance")
	}
	if f.executions != 1 || f.store.writes != 0 {
		t.Fatal("GET mutated state")
	}
}

// Each Depot success logs its legacy ＠verb>target and $npc_com (depot.cgi:105-542).
func TestDepotCommandsEchoIntoThePlaceLog(t *testing.T) {
	nbsp := strings.Repeat(" ", 12)
	for _, tc := range []struct {
		action, params string
		names          depot.Receipt
		actor, reply   string
	}{
		{"depot_deposit", `{"item_id":"one"}`, depot.Receipt{ItemName: "どうのつるぎ"}, "＠あずける>どうのつるぎ", "どうのつるぎをお預かりしました"},
		{"depot_withdraw", `{"item_id":"one"}`, depot.Receipt{ItemName: "やくそう"}, "＠ひきだす>やくそう", "やくそうをお返しします"},
		{"depot_withdraw", `{"item_id":"one"}`, depot.Receipt{ItemName: "やくそう", ReturnedName: "どくけし"}, "＠ひきだす>やくそう", "どくけしをお預かりしました。やくそうをお返しします"},
		{"depot_send_item", `{"item_id":"one","recipient_character_id":"r"}`, depot.Receipt{ItemName: "やくそう", RecipientName: "Ann"}, "＠おくる>やくそう＠あいて>Ann", "やくそうをAnnに送りました"},
		{"depot_send_money", `{"amount":300,"recipient_character_id":"r"}`, depot.Receipt{RecipientName: "Ann"}, "＠おくる>300G＠あいて>Ann", "300 Gを Ann に送りました"},
		// A name that looks like a placeholder is shown verbatim.
		{"depot_send_money", `{"amount":300,"recipient_character_id":"r"}`, depot.Receipt{RecipientName: "{name}"}, "＠おくる>300G＠あいて>{name}", "300 Gを {name} に送りました"},
		{"depot_sort", "", depot.Receipt{}, "＠せいとん", "預かっているアイテムをせいとんした"},
		{"depot_expand", "", depot.Receipt{}, "＠かくちょう>はい", "Heroの倉庫を拡張しました"},
		{"depot_sell", `{"item_id":"one"}`, depot.Receipt{Sales: []depot.Sale{{Name: "やくそう", Price: 150}}}, "＠うる>やくそう", "やくそう の買取代の 150 Gです！"},
		{"depot_sell_batch", `{"item_ids":["one","two"]}`, depot.Receipt{Sales: []depot.Sale{{Name: "やくそう", Price: 50}, {Name: "どくけし", Price: 100}}}, "＠まとめてうる>やくそう<>どくけし<>",
			"買取代の 150 Gです！\n-------買取リスト-------\n" + nbsp + "やくそう 50 G\n" + nbsp + "どくけし 100 G\n---------------------"},
	} {
		t.Run(tc.action+"/"+tc.actor, func(t *testing.T) {
			log := &fakePlaceLog{}
			f := depotSendMoneyRouter(t, WithPlaceLog(log))
			f.char.Name, f.char.Money, f.names = "Hero", 1000, tc.names
			status, raw := gatewayRequest(t, f.router, "hero", "session", "application/json", saleGatewayBody(tc.action, tc.params), nil)
			if status != 200 || log.writes != 1 || len(log.rows) != 2 {
				t.Fatalf("%d %s rows %+v", status, raw["error"], log.rows)
			}
			if npc, actor := log.rows[0], log.rows[1]; npc.Name != depotFacility.npc.name || npc.Text != tc.reply || actor.Text != tc.actor || actor.Name != "Hero" {
				t.Fatalf("rows: %+v", log.rows)
			}
		})
	}
}

// Collecting held gold is a private $mes: no rows and no wait (depot.cgi:211-215, lib/system.cgi:651).
func TestDepotWithdrawCollectionWritesNoLogAndStartsNoWait(t *testing.T) {
	log := &fakePlaceLog{}
	timers := timer.NewService(nil)
	f := depotSendMoneyRouterWith(t, []func(*playercontext.Service){playercontext.WithActionWait(timers, playercontext.DefaultActionWait)}, WithPlaceLog(log))
	f.collected = []depot.Remittance{{ID: "held", CharacterID: "hero", Amount: 150, Source: depot.RemittanceDepotTransfer, SenderName: "Ann"}}
	for range 2 {
		if status, raw := gatewayRequest(t, f.router, "hero", "session", "application/json", saleGatewayBody("depot_withdraw", ""), nil); status != 200 {
			t.Fatalf("collection: %d %s", status, raw["error"])
		}
	}
	if log.writes != 0 {
		t.Fatalf("collection logged: %+v", log.rows)
	}
	if status, raw := gatewayRequest(t, f.router, "hero", "session", "application/json", saleGatewayBody("depot_sort", ""), nil); status != 200 {
		t.Fatalf("sort after collection: %d %s", status, raw["error"])
	}
	if status, raw := gatewayRequest(t, f.router, "hero", "session", "application/json", saleGatewayBody("depot_sort", ""), nil); status != 409 || !strings.Contains(string(raw["error"]), "ACTION_WAIT") {
		t.Fatalf("logged command did not start the wait: %d %s", status, raw["error"])
	}
}

// The Auction venue's おくる is the same routine answered by its own NPC (auction.cgi:76).
func TestAuctionSendMoneyEchoesWithTheAuctionNPC(t *testing.T) {
	log := &fakePlaceLog{}
	f := depotSendMoneyRouter(t, WithPlaceLog(log))
	f.store.selection.Destination, f.char.Money, f.names = "auction", 1000, depot.Receipt{RecipientName: "Ann"}
	status, raw := f.send(`{"amount":300,"recipient_character_id":"r"}`)
	if status != 200 || len(log.rows) != 2 || log.rows[0].Name != auctionFacility.npc.name || log.rows[0].Text != "300 Gを Ann に送りました" {
		t.Fatalf("%d %s rows %+v", status, raw["error"], log.rows)
	}
}
