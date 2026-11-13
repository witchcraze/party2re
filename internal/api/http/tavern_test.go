package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/playercontext"
	"github.com/witchcraze/party2re/internal/tavern"
)

var tavernTestMenu = []tavern.MenuItem{
	{ID: "tavern_water", Name: "癒しの湧き水", Price: 20, HPHeal: 40, Tickets: 1},
	{ID: "tavern_course", Name: "フルコース料理", Price: 3000, HPHeal: 999, MPHeal: 999, Tickets: 15},
}

type tavernFixture struct {
	*gatewayFixture
	status    tavern.TavernStatus
	statusErr error
	inputs    []string
}

func (f *tavernFixture) GetMenu() []tavern.MenuItem { return tavernTestMenu }

func (f *tavernFixture) GetStatus(_ context.Context, actor string) (tavern.TavernStatus, error) {
	if actor != "hero" {
		f.t.Fatal("wrong tavern observer")
	}
	return f.status, f.statusErr
}

func (f *tavernFixture) OrderMeal(ctx context.Context, actor, itemID string) (tavern.OrderResult, error) {
	f.inputs = append(f.inputs, itemID)
	_, err := f.execute(ctx, actor, 0)
	item := tavernTestMenu[slices.IndexFunc(tavernTestMenu, func(m tavern.MenuItem) bool { return m.ID == itemID })]
	return tavern.OrderResult{CharacterID: actor, Item: item, HPHealed: 10, TicketsAwarded: item.Tickets, TotalTickets: 20}, err
}

func (f *tavernFixture) ReserveDelivery(ctx context.Context, actor, itemID string) (tavern.DeliveryReservation, error) {
	f.inputs = append(f.inputs, itemID)
	_, err := f.execute(ctx, actor, 0)
	return tavern.DeliveryReservation{CharacterID: actor, ItemID: itemID, ItemName: "フルコース料理"}, err
}

func (f *tavernFixture) CancelDelivery(ctx context.Context, actor string) error {
	_, err := f.execute(ctx, actor, 0)
	return err
}

func (f *tavernFixture) router(t *testing.T, destination string, options ...Option) (*gatewayNavigationStore, http.Handler) {
	t.Helper()
	store := &gatewayNavigationStore{selection: playercontext.Selection{Destination: destination}}
	pc := playercontext.NewService(f.gatewayFixture, f.gatewayFixture, timer.NewService(nil), playercontext.WithNavigation(store,
		playercontext.SceneDefinition{ID: "town", Pageable: true}, playercontext.SceneDefinition{ID: "bank", Parent: "town"},
		playercontext.SceneDefinition{ID: "tavern", Parent: "town"}))
	opts := append([]Option{WithTavern(f), WithPlayerContext(pc), func(h *Handler) { h.homes = f.gatewayFixture }}, options...)
	h, err := NewHandler(f.gatewayFixture, f.gatewayFixture, &struct{ AdventureService }{}, &struct{ ShopService }{}, opts...)
	if err != nil {
		t.Fatal(err)
	}
	return store, h.Router()
}

func newTavernFixture(t *testing.T) *tavernFixture {
	f := &tavernFixture{gatewayFixture: newGatewayFixture(t), status: tavern.TavernStatus{CharacterID: "hero"}}
	f.char.Name = "勇者"
	return f
}

func offered(observation PlayerContextResponse, action string) bool {
	return slices.ContainsFunc(observation.AvailableActions, func(a ContextAction) bool { return a.Action == action })
}

func TestTavernSceneObservesMenuFullnessAndDelivery(t *testing.T) {
	f := newTavernFixture(t)
	f.bio = map[string]string{"like_food": "ラーメン"}
	store, router := f.router(t, "tavern", WithPlaceLog(&fakePlaceLog{}))
	status, observation := navigationGET(t, router)
	data := decodeShopScene[TavernSceneData](t, observation)
	if status != 200 || observation.Scene.Title != "冒険者の酒場" || observation.Scene.Support.Observation != "details" ||
		data.Parent != "town" || data.IsFull || data.Delivery != nil || len(data.Menu) != 2 ||
		data.Menu[0].OrderName != "癒しの湧き水" || data.Menu[1].OrderName != "ラーメン" || data.Menu[1].Name != "フルコース料理" {
		t.Fatalf("tavern projection: %d %+v %+v", status, observation, data)
	}
	if want := []PlaceMember{{Kind: "npc", Name: tavern.NPCName, Color: npcColor}}; !slices.Equal(observation.Scene.Members, want) {
		t.Fatalf("members %+v", observation.Scene.Members)
	}
	for _, action := range []string{"tavern_order", "tavern_delivery_reserve", "npc_talk", "npc_inspect", "place_say", "scene_back"} {
		if !offered(observation, action) {
			t.Fatalf("missing choice %s: %+v", action, observation.AvailableActions)
		}
	}
	if offered(observation, "tavern_delivery_cancel") {
		t.Fatal("cancel offered without a delivery order")
	}

	f.bio = nil
	f.status = tavern.TavernStatus{CharacterID: "hero", IsFull: true, DeliveryReservation: &tavern.DeliveryReservation{ItemID: "tavern_course", ItemName: "フルコース料理"}}
	status, observation = navigationGET(t, router)
	data = decodeShopScene[TavernSceneData](t, observation)
	if status != 200 || !data.IsFull || data.Delivery == nil || data.Menu[1].OrderName != "フルコース料理" || !offered(observation, "tavern_delivery_cancel") {
		t.Fatalf("reserved projection: %d %+v", status, data)
	}
	if store.writes != 0 || f.executions != 0 {
		t.Fatalf("GET wrote: %d %d", store.writes, f.executions)
	}
}

func TestTavernSceneRequiredReadFailures(t *testing.T) {
	for _, fail := range []string{"status", "profile"} {
		t.Run(fail, func(t *testing.T) {
			f := newTavernFixture(t)
			_, router := f.router(t, "tavern")
			if fail == "status" {
				f.statusErr = errors.New("private reservation read failure")
			} else {
				f.profileErr = errors.New("private profile failure")
			}
			if status, observation := navigationGET(t, router); status != 500 || observation.Scene.Data != nil {
				t.Fatalf("partial observation: %d %+v", status, observation)
			}
		})
	}
}

func tavernResult[T any](t *testing.T, raw map[string]json.RawMessage) T {
	t.Helper()
	var out T
	if err := json.Unmarshal(raw["result"], &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestTavernOrderRepliesLikeLegacy(t *testing.T) {
	f := newTavernFixture(t)
	f.bio = map[string]string{"like_food": "ラーメン"}
	_, router := f.router(t, "tavern")
	status, raw := gatewayRequest(t, router, "hero", "session", "application/json", saleGatewayBody("tavern_order", `{"item_id":"tavern_course"}`), nil)
	got := tavernResult[TavernOrderResult](t, raw)
	if status != 200 || got.Full || got.ItemName != "ラーメン" || got.TicketsAwarded != 15 ||
		got.Message != "おまたせ、ラーメンよ♪勇者のＨＰが回復した！勇者のＭＰが回復した！福引券を15枚もらった！" {
		t.Fatalf("order: %d %+v", status, got)
	}
	status, raw = gatewayRequest(t, router, "hero", "session", "application/json", saleGatewayBody("tavern_order", `{"item_id":"tavern_water"}`), nil)
	if got := tavernResult[TavernOrderResult](t, raw); status != 200 || got.Message != "おまたせ、癒しの湧き水よ♪勇者のＨＰが回復した！福引券を1枚もらった！" {
		t.Fatalf("drink order: %d %+v", status, got)
	}

	f.executionErr = fmt.Errorf("wrapped: %w", tavern.ErrAlreadyFull)
	status, raw = gatewayRequest(t, router, "hero", "session", "application/json", saleGatewayBody("tavern_order", `{"item_id":"tavern_water"}`), nil)
	if got := tavernResult[TavernOrderResult](t, raw); status != 200 || !got.Full || got.Message != "そんなに飲み食いするとお腹がこわれちゃうわよ？" {
		t.Fatalf("already full: %d %+v", status, got)
	}
}

// Legacy chuumon answers fullness before funds, and an empty wallet still reaches the barkeep.
func TestTavernOrderWithEmptyWallet(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
	}{{"hungry", tavern.ErrInsufficientFunds}, {"full", tavern.ErrAlreadyFull}} {
		t.Run(tc.name, func(t *testing.T) {
			f := newTavernFixture(t)
			f.char.Money = 0
			f.executionErr = tc.err
			_, router := f.router(t, "tavern")
			status, raw := gatewayRequest(t, router, "hero", "session", "application/json", saleGatewayBody("tavern_order", `{"item_id":"tavern_water"}`), nil)
			if f.executions != 1 {
				t.Fatalf("order did not reach the service at 0G: %d %s", status, raw)
			}
			if tc.name == "full" {
				if got := tavernResult[TavernOrderResult](t, raw); status != 200 || !got.Full || got.Message != "そんなに飲み食いするとお腹がこわれちゃうわよ？" {
					t.Fatalf("full at 0G: %d %+v", status, got)
				}
				return
			}
			assertGatewayError(t, raw, "TAVERN_INSUFFICIENT_FUNDS")
			if status != 409 || !strings.Contains(string(raw["error"]), "お金が足りないみたいね") {
				t.Fatalf("hungry at 0G: %d %s", status, raw)
			}
		})
	}
}

func TestTavernOrderRejectsBeforeCommitWhenProfileFails(t *testing.T) {
	f := newTavernFixture(t)
	f.profileErr = errors.New("private profile failure")
	_, router := f.router(t, "tavern")
	// Context resolution also reads the profile, so the request fails before the order runs.
	if status, _ := gatewayRequest(t, router, "hero", "session", "application/json", saleGatewayBody("tavern_order", `{"item_id":"tavern_water"}`), nil); status < 400 || f.executions != 0 {
		t.Fatalf("order ran without a profile: %d %d", status, f.executions)
	}
}

func TestTavernDeliveryCommands(t *testing.T) {
	f := newTavernFixture(t)
	_, router := f.router(t, "tavern")
	status, raw := gatewayRequest(t, router, "hero", "session", "application/json", saleGatewayBody("tavern_delivery_reserve", `{"item_id":"tavern_course"}`), nil)
	if got := tavernResult[TavernDeliveryResult](t, raw); status != 200 || got.Delivery == nil || got.Message != "冒険が終わったらフルコース料理が届くようにしたわ" {
		t.Fatalf("reserve: %d %+v", status, got)
	}
	status, raw = gatewayRequest(t, router, "hero", "session", "application/json", saleGatewayBody("tavern_delivery_cancel", `{}`), nil)
	if got := tavernResult[TavernDeliveryResult](t, raw); status != 200 || got.Delivery != nil || got.Message != "でりばりーをキャンセルしたわ" {
		t.Fatalf("cancel: %d %+v", status, got)
	}
	if fmt.Sprint(f.inputs) != "[tavern_course]" {
		t.Fatalf("inputs %v", f.inputs)
	}
}

func TestTavernCommandErrors(t *testing.T) {
	for _, tc := range []struct {
		action, params string
		err            error
		code           string
		status         int
	}{
		{"tavern_order", `{"item_id":"tavern_water"}`, tavern.ErrInsufficientFunds, "TAVERN_INSUFFICIENT_FUNDS", 409},
		{"tavern_order", `{"item_id":"tavern_water"}`, tavern.ErrMenuItemNotFound, "TAVERN_ITEM_NOT_FOUND", 400},
		{"tavern_delivery_reserve", `{"item_id":"tavern_water"}`, tavern.ErrMenuItemNotFound, "TAVERN_ITEM_NOT_FOUND", 400},
		{"tavern_delivery_cancel", `{}`, tavern.ErrNoActiveDelivery, "TAVERN_NO_DELIVERY", 409},
		{"tavern_order", `{"item_id":"tavern_water"}`, errors.New("private storage failure"), "EXECUTION_FAILED", 500},
	} {
		t.Run(tc.action+tc.code, func(t *testing.T) {
			f := newTavernFixture(t)
			f.executionErr = fmt.Errorf("private wrapped: %w", tc.err)
			_, router := f.router(t, "tavern")
			status, got := gatewayRequest(t, router, "hero", "session", "application/json", saleGatewayBody(tc.action, tc.params), nil)
			assertGatewayError(t, got, tc.code)
			if status != tc.status || strings.Contains(fmt.Sprint(got), "private") {
				t.Fatalf("error mapping: %d %s", status, got)
			}
		})
	}
}

func TestTavernCommandsRequireTheTavern(t *testing.T) {
	for _, action := range []string{"tavern_order", "tavern_delivery_reserve", "tavern_delivery_cancel"} {
		f := newTavernFixture(t)
		_, router := f.router(t, "bank")
		params := `{"item_id":"tavern_water"}`
		if action == "tavern_delivery_cancel" {
			params = `{}`
		}
		if status, got := gatewayRequest(t, router, "hero", "session", "application/json", saleGatewayBody(action, params), nil); status != 409 || f.executions != 0 {
			t.Fatalf("%s outside the tavern: %d %s", action, status, got)
		}
	}
}

func TestTavernWordsNameACatalogDish(t *testing.T) {
	words, _ := tavernWords(nil, context.Background(), corecharacter.Character{})
	line := words[1]
	if !slices.ContainsFunc(tavernDishes, func(d string) bool { return line == "{name}さんは"+d+"は好きかしら？" }) {
		t.Fatalf("dish line %q names no catalog dish", line)
	}
}
