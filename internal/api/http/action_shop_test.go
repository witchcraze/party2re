package http

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/playercontext"
	"github.com/witchcraze/party2re/internal/shop"
)

func TestShopSellUsesObservedHandOffer(t *testing.T) {
	f, store, router := shopSceneRouter(t)
	weapon, _ := coreitem.NewInstance("weapon-01", 1)
	f.inv.Items = []coreitem.Instance{weapon}
	f.char.Money = 1
	store.selection.Destination = "shop_armor"
	if _, response := navigationGET(t, router); slices.ContainsFunc(response.AvailableActions, func(a ContextAction) bool { return a.Action == "shop_sell" }) {
		t.Fatal("armor shop offered to buy a weapon")
	}
	store.selection.Destination = "shop_weapon"
	_, response := navigationGET(t, router)
	data := decodeShopScene[ShopCatalogSceneData](t, response)
	i := slices.IndexFunc(response.AvailableActions, func(a ContextAction) bool { return a.Action == "shop_sell" })
	if data.SaleOffer == nil || *data.SaleOffer != (shop.SaleOffer{ItemInstanceID: weapon.ID, ItemDefinitionID: "weapon-01", Name: "ヒノキの棒", Payout: 5}) ||
		i < 0 || response.AvailableActions[i].ParamsTemplate["item_instance_id"] != weapon.ID || response.AvailableActions[i].ParamsTemplate["shop_type"] != "weapon" {
		t.Fatalf("sale offer: %+v %+v", data.SaleOffer, response.AvailableActions)
	}
	// The weapon shop's scene fixes shop_type, so another shop cannot be named from it.
	status, raw := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"shop_sell","params":{"shop_type":"item","item_instance_id":"`+weapon.ID+`"}}`, nil)
	if status != 409 || !strings.Contains(string(raw["error"]), "ACTION_UNAVAILABLE") || len(f.inv.Items) != 1 || f.char.Money != 1 {
		t.Fatalf("weapon shop sold through the item shop: %d %s", status, raw)
	}
	store.selection.Destination = "shop_item"
	status, raw = gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"shop_sell","params":{"shop_type":"item","item_instance_id":"`+weapon.ID+`"}}`, nil)
	if status != 409 || !strings.Contains(string(raw["error"]), "SHOP_ITEM_NOT_IN_HAND") || len(f.inv.Items) != 1 || f.char.Money != 1 {
		t.Fatalf("item shop bought a weapon: %d %s", status, raw)
	}
	store.selection.Destination = "shop_weapon"
	body, err := json.Marshal(map[string]any{"action": "shop_sell", "params": response.AvailableActions[i].ParamsTemplate})
	if err != nil {
		t.Fatal(err)
	}
	status, raw = gatewayRequest(t, router, "hero", "session", "application/json", string(body), nil)
	var result shopSaleResponse
	if err := json.Unmarshal(raw["result"], &result); err != nil {
		t.Fatal(err)
	}
	if status != 200 || len(f.inv.Items) != 0 || f.char.Money != 6 || result != (shopSaleResponse{ItemInstanceID: weapon.ID, Quantity: 1, TotalPayout: 5, NPCMessage: "ヒノキの棒 の買取代の 5 Gだ！"}) {
		t.Fatalf("sale: %d %s", status, raw)
	}
}

type shopCommandService struct {
	ShopService
	batch      []shop.BatchPurchaseItemRequest
	synthesize string
	err        error
}

func (s *shopCommandService) BatchPurchase(_ context.Context, _ string, kind shop.ShopType, items []shop.BatchPurchaseItemRequest) (shop.BatchPurchaseResult, error) {
	s.batch = items
	return shop.BatchPurchaseResult{TotalPrice: 40, Purchased: make([]coreitem.Instance, len(items))}, s.err
}

func (s *shopCommandService) Synthesize(_ context.Context, _ string, target string) (shop.SynthesisResult, error) {
	s.synthesize = target
	return shop.SynthesisResult{Recipe: shop.SynthesisRecipe{ProductName: "ルビーのうでわ"}, Success: true, UsedHiyaku: true}, s.err
}

func TestShopBatchPurchaseAndSynthesisCommands(t *testing.T) {
	stub := &shopCommandService{}
	f, store, router := shopSceneRouter(t, func(h *Handler) {
		stub.ShopService = h.shops
		h.shops = stub
	})
	f.char.Name = "Hero"
	for _, tc := range []struct {
		scene, body, message string
	}{
		{"shop_item", `{"action":"shop_batch_purchase","params":{"shop_type":"item","items":[{"item_definition_id":"item-001","quantity":2}]}}`, "Heroニャンの預かり所の方に投げましたニャ！"},
		{"shop_accessory", `{"action":"shop_synthesize","params":{"recipe_target":"ruby"}}`, "合成の秘薬を使って、ルビーのうでわ が完成したわ。預り所にあるはずよ"},
	} {
		store.selection = playercontext.Selection{Destination: tc.scene}
		status, raw := gatewayRequest(t, router, "hero", "session", "application/json", tc.body, nil)
		var result struct {
			NPCMessage string `json:"npc_message"`
		}
		if err := json.Unmarshal(raw["result"], &result); err != nil {
			t.Fatal(err)
		}
		if status != 200 || result.NPCMessage != tc.message {
			t.Fatalf("%s: %d %s", tc.body, status, raw)
		}
	}
	if len(stub.batch) != 1 || stub.batch[0] != (shop.BatchPurchaseItemRequest{ItemDefinitionID: "item-001", Quantity: 2}) || stub.synthesize != "ruby" {
		t.Fatalf("inputs: %+v %q", stub.batch, stub.synthesize)
	}
}

func TestShopCommandsRejectInvalidInputsWithoutWrites(t *testing.T) {
	f, store, router := shopSceneRouter(t)
	f.char.Money = 1000
	for _, tc := range []struct {
		scene, body string
		status      int
		code        string
	}{
		{"shop_weapon", `{"action":"shop_purchase","params":{"shop_type":"weapon","item_definition_id":"weapon-01"}}`, 400, ""},
		{"shop_weapon", `{"action":"shop_purchase","params":{"shop_type":"weapon","item_definition_id":"weapon-01","quantity":1,"character_id":"other"}}`, 400, ""},
		{"shop_weapon", `{"action":"shop_purchase","params":{"shop_type":"secret","item_definition_id":"weapon-01","quantity":1}}`, 409, "ACTION_UNAVAILABLE"},
		{"town", `{"action":"shop_purchase","params":{"shop_type":"weapon","item_definition_id":"weapon-01","quantity":1}}`, 409, "ACTION_UNAVAILABLE"},
		{"shop_weapon", `{"action":"shop_purchase","params":{"shop_type":"weapon","item_definition_id":"weapon-01","quantity":0}}`, 400, "SHOP_INVALID_QUANTITY"},
		{"shop_weapon", `{"action":"shop_sell","params":{"shop_type":"weapon","item_instance_id":"missing"}}`, 404, "SHOP_ITEM_NOT_OWNED"},
		{"shop_accessory", `{"action":"shop_batch_purchase","params":{"shop_type":"accessory","items":[{"item_definition_id":"item-143","quantity":1}]}}`, 409, "ACTION_UNAVAILABLE"},
		{"shop_item", `{"action":"shop_batch_purchase","params":{"shop_type":"item","items":[]}}`, 400, "SHOP_EMPTY_PURCHASE_LIST"},
	} {
		store.selection = playercontext.Selection{Destination: tc.scene}
		status, raw := gatewayRequest(t, router, "hero", "session", "application/json", tc.body, nil)
		var failure struct {
			Error ErrorDetail `json:"error"`
		}
		if err := json.Unmarshal(raw["error"], &failure.Error); err != nil {
			t.Fatal(err)
		}
		if status != tc.status || (tc.code != "" && failure.Error.Code != tc.code) || f.writes != 0 || f.char.Money != 1000 {
			t.Fatalf("%s: %d %s", tc.body, status, raw)
		}
	}
}

func TestShopRejectionKeepsStorageFailuresUnexpected(t *testing.T) {
	if status, _ := shopActionRejection(corecharacter.ErrNotFound); status != 404 {
		t.Fatalf("missing character: %d", status)
	}
	// Unmapped errors take the Gateway's 500 EXECUTION_FAILED path (#2018).
	for _, err := range []error{errors.New("storage down"), context.Canceled} {
		if status, _ := shopActionRejection(err); status != 0 {
			t.Fatalf("%v mapped to known rejection %d", err, status)
		}
	}
}
