package http

import (
	"encoding/json"
	"testing"

	"github.com/witchcraze/party2re/internal/blacksmith"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/core/timer"
)

// echoCase is one command and the actor and NPC rows legacy logs for it
// (lib/system.cgi:660-676 and each facility's $npc_com).
type echoCase struct{ action, params, actor, reply string }

func assertEcho(t *testing.T, log *fakePlaceLog, status int, raw map[string]json.RawMessage, tc echoCase) {
	t.Helper()
	if status != 200 || len(log.rows) != 2 || log.rows[1].Text != tc.actor || log.rows[0].Text != tc.reply {
		t.Fatalf("%s: %d %s rows %+v", tc.action, status, raw["error"], log.rows)
	}
}

func TestBlackMarketCommandsEcho(t *testing.T) {
	for _, tc := range []echoCase{
		{"blackmarket_sacrifice", `{"item_instance_id":"owned-item"}`, "＠ささげる>はやぶさの剣", "…はやぶさの剣…か…。レアだな…。いいだろう…。お前のレアポイントを加算しておこう…"},
		{"blackmarket_trade", `{"prize_id":"bm_prize_087"}`, "＠とりひき>時の砂", "取引成立だ…。時の砂 はお前の預かり所に送っておいた…"},
	} {
		log := &fakePlaceLog{}
		f := &blackMarketGatewayFixture{gatewayFixture: newGatewayFixture(t)}
		status, raw := gatewayRequest(t, f.router(t, timer.NewService(nil), WithPlaceLog(log)), "hero", "session", "application/json", saleGatewayBody(tc.action, tc.params), nil)
		assertEcho(t, log, status, raw, tc)
	}
}

func TestFleaMarketCommandsEcho(t *testing.T) {
	// Go listings have no display number, so purchase and cancel name the item.
	for _, tc := range []echoCase{
		{"fleamarket_list", `{"item_id":"owned-item","price":300}`, "＠しゅっぴん>はやぶさの剣＠ねだん>300", "はやぶさの剣を300 Gで出品しとくぞ"},
		{"fleamarket_purchase", `{"listing_id":"listing-1"}`, "＠かう>はやぶさの剣", "まいどあり"},
		{"fleamarket_cancel", `{"listing_id":"listing-1"}`, "＠もどす>はやぶさの剣", "そら、はやぶさの剣だ"},
	} {
		log := &fakePlaceLog{}
		f := &fleaMarketGatewayFixture{gatewayFixture: newGatewayFixture(t)}
		status, raw := gatewayRequest(t, f.router(t, timer.NewService(nil), WithPlaceLog(log)), "hero", "session", "application/json", saleGatewayBody(tc.action, tc.params), nil)
		assertEcho(t, log, status, raw, tc)
	}
}

func TestGemStoreCommandsEcho(t *testing.T) {
	for _, tc := range []echoCase{
		{"gemstore_buy", `{"gem_id":"gem_atk_1"}`, "＠かう>攻撃の天珠Ⅰ", "攻撃の天珠Ⅰ を Heroさんの宝石箱に入れておきました"},
		{"gemstore_sell", `{"item_id":"owned-gem"}`, "＠うる>攻撃の天珠Ⅰ", "攻撃の天珠Ⅰ の代金250 Gです"},
		{"gemstore_send", `{"recipient_character_id":"friend","item_id":"owned-gem"}`, "＠おくる>攻撃の天珠Ⅰ＠あいて>Friend", "攻撃の天珠Ⅰ をFriendさんに送りました"},
		{"gemstore_synthesize", `{"recipe_id":"recipe-1"}`, "＠かこう>攻撃の天珠Ⅰ×天馬のたづな", "攻撃の天珠Ⅱ が完成しました。宝石箱に入れておきました"},
		{"gemstore_appraise", `{"item_id":"orb"}`, "＠かんてい>宝珠", "これは… 攻撃の天珠Ⅰですね。Heroさんの宝石箱に入れておきました"},
		{"gemstore_sort", `{}`, "＠せいとん", "お預かりしている宝石をせいとんしました"},
	} {
		log := &fakePlaceLog{}
		f := &gemStoreGatewayFixture{gatewayFixture: newGatewayFixture(t)}
		f.char.Name = "Hero"
		status, raw := gatewayRequest(t, f.router(t, timer.NewService(nil), WithPlaceLog(log)), "hero", "session", "application/json", saleGatewayBody(tc.action, tc.params), nil)
		assertEcho(t, log, status, raw, tc)
	}
}

func TestBlacksmithCommandsEcho(t *testing.T) {
	for _, tc := range []echoCase{
		{"blacksmith_seal", `{"seal_id":2}`, "＠こくいん>" + blacksmith.Seals[1].Name, "よっしゃ！武器に【" + blacksmith.Seals[1].Name + "】を刻印したぜ！"},
		{"blacksmith_name", `{"target":"weapon","name":"名剣"}`, "＠なづける>武器＠なまえ>名剣", "武器を名剣と名づけたぜ"},
		{"blacksmith_name", `{"target":"armor","name":"名剣"}`, "＠なづける>防具＠なまえ>名剣", "防具を名剣と名づけたぜ"},
		{"blacksmith_deposit", `{}`, "＠あずける>武器", "名剣をお預かっておくぜ"},
		{"blacksmith_withdraw", `{"slot":3}`, "＠ひきだす>古剣", "古剣を返すぜ"},
	} {
		log := &fakePlaceLog{}
		f := &blacksmithGatewayFixture{gatewayFixture: newGatewayFixture(t)}
		status, raw := gatewayRequest(t, f.router(t, timer.NewService(nil), WithPlaceLog(log)), "hero", "session", "application/json", saleGatewayBody(tc.action, tc.params), nil)
		assertEcho(t, log, status, raw, tc)
	}
	// A held weapon is a logged reply that changes nothing (blacksmith.cgi:206-209).
	log := &fakePlaceLog{}
	f := &blacksmithGatewayFixture{gatewayFixture: newGatewayFixture(t)}
	f.executionErr = blacksmith.ErrWeaponSlotOccupied
	status, raw := gatewayRequest(t, f.router(t, timer.NewService(nil), WithPlaceLog(log)), "hero", "session", "application/json", saleGatewayBody("blacksmith_withdraw", `{"slot":3}`), nil)
	assertEcho(t, log, status, raw, echoCase{"blacksmith_withdraw", "", "＠ひきだす>古剣", "おっと、武器を外してからにしてくれ！"})
}

func TestSecretShopPurchaseEchoes(t *testing.T) {
	log := &fakePlaceLog{}
	f := newSecretShopGatewayFixture(t)
	f.inv.Items = []coreitem.Instance{{ID: "held", DefinitionID: "item-015", Quantity: 1}}
	status, raw := f.request(f.router(timer.NewService(nil), WithPlaceLog(log)), `{"item_id":"secret_item_herbal_root","quantity":1}`)
	def, err := f.catalog.FindByID("item-010")
	if err != nil {
		t.Fatal(err)
	}
	name := def.Name
	assertEcho(t, log, status, raw, echoCase{"secretshop_purchase", "", "＠かう>" + name, name + "は" + f.char.Name + "メェ〜の預かり所の方に投げましたメェ〜"})
}

func TestShopCommandsEcho(t *testing.T) {
	log := &fakePlaceLog{}
	f, store, router := shopSceneRouter(t, WithPlaceLog(log))
	weapon, _ := coreitem.NewInstance("weapon-01", 1)
	f.inv.Items = []coreitem.Instance{weapon}
	store.selection.Destination = "shop_weapon"
	status, raw := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"shop_sell","params":{"shop_type":"weapon","item_instance_id":"`+weapon.ID+`"}}`, nil)
	assertEcho(t, log, status, raw, echoCase{"shop_sell", "", "＠うる>ヒノキの棒", "ヒノキの棒 の買取代の 5 Gだ！"})
	log.rows, f.char.Money = nil, 1000
	status, raw = gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"shop_purchase","params":{"shop_type":"weapon","item_definition_id":"weapon-01","quantity":1}}`, nil)
	assertEcho(t, log, status, raw, echoCase{"shop_purchase", "", "＠かう>ヒノキの棒", "まいど！ヒノキの棒だ！受けとってくれ！"})
}
