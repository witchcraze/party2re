package http

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/lottery"
	"github.com/witchcraze/party2re/internal/playercontext"
)

type lotteryFixture struct {
	*gatewayFixture
	tickets   int
	ticketErr error
	status    lottery.TakarakujiStatus
	statusErr error
	current   *lottery.TakarakujiTicket
	raffle    lottery.RaffleResult
	remaining int
}

func (f *lotteryFixture) GetRaffleTickets(context.Context, string) (int, error) {
	return f.tickets, f.ticketErr
}

func (f *lotteryFixture) PlayRaffle(ctx context.Context, actor string) (lottery.RaffleResult, int, corecharacter.Character, error) {
	_, err := f.execute(ctx, actor, 0)
	return f.raffle, f.remaining, f.char, err
}

func (f *lotteryFixture) GetTakarakujiStatus(context.Context) (lottery.TakarakujiStatus, error) {
	return f.status, f.statusErr
}

func (f *lotteryFixture) BuyTakarakujiTicket(ctx context.Context, actor string) (lottery.TakarakujiPurchaseResult, error) {
	_, err := f.execute(ctx, actor, 0)
	// 2026-11-20T15:00Z is 2026/11/21 00:00 JST.
	return lottery.TakarakujiPurchaseResult{Ticket: lottery.TakarakujiTicket{ID: "t1", CharacterID: actor}, RemainingGold: 70000,
		DrawDate: time.Date(2026, 11, 20, 15, 0, 0, 0, time.UTC)}, err
}

func (f *lotteryFixture) GetCharacterTakarakujiTicket(context.Context, string) (*lottery.TakarakujiTicket, []lottery.TakarakujiTicket, error) {
	return f.current, nil, nil
}

func (f *lotteryFixture) router(t *testing.T, destination string, options ...Option) (*gatewayNavigationStore, http.Handler) {
	t.Helper()
	store := &gatewayNavigationStore{selection: playercontext.Selection{Destination: destination}}
	pc := playercontext.NewService(f.gatewayFixture, f.gatewayFixture, timer.NewService(nil), playercontext.WithNavigation(store,
		playercontext.SceneDefinition{ID: "town", Pageable: true}, playercontext.SceneDefinition{ID: "bank", Parent: "town"},
		playercontext.SceneDefinition{ID: "lottery", Parent: "town"}, playercontext.SceneDefinition{ID: "takarakuji", Parent: "town"}))
	opts := append([]Option{WithLottery(f), WithPlayerContext(pc), func(h *Handler) { h.homes = f.gatewayFixture }}, options...)
	h, err := NewHandler(f.gatewayFixture, f.gatewayFixture, &struct{ AdventureService }{}, &struct{ ShopService }{}, opts...)
	if err != nil {
		t.Fatal(err)
	}
	return store, h.Router()
}

func newLotteryFixture(t *testing.T) *lotteryFixture {
	f := &lotteryFixture{gatewayFixture: newGatewayFixture(t), tickets: 7,
		status: lottery.TakarakujiStatus{RoundID: 3, TicketPrice: 30000, MaxTickets: 20, SoldCount: 8, RemainingTickets: 12,
			DrawDate: time.Date(2026, 11, 20, 15, 0, 0, 0, time.UTC), Prizes: []lottery.TakarakujiPrizeItem{}}}
	f.char.Name = "勇者"
	return f
}

func TestRaffleHallObservesCouponsAndPrizeTable(t *testing.T) {
	f := newLotteryFixture(t)
	store, router := f.router(t, "lottery", WithPlaceLog(&fakePlaceLog{}))
	status, observation := navigationGET(t, router)
	data := decodeShopScene[LotterySceneData](t, observation)
	if status != 200 || observation.Scene.Title != "福引所" || observation.Scene.Support.Observation != "details" ||
		data.Tickets != 7 || data.Special || data.Cost != 3 || len(data.Prizes) != 7 || data.Prizes[4].Name != "種系" || data.Prizes[6].Name != "福袋" {
		t.Fatalf("raffle hall: %d %+v", status, data)
	}
	for _, action := range []string{"lottery_raffle", "npc_talk", "npc_inspect", "place_say", "scene_back"} {
		if !offered(observation, action) {
			t.Fatalf("missing choice %s", action)
		}
	}
	f.tickets = 300
	_, observation = navigationGET(t, router)
	if data := decodeShopScene[LotterySceneData](t, observation); !data.Special || data.Cost != 300 || len(data.Prizes) != 1 {
		t.Fatalf("special raffle: %+v", data)
	}
	f.ticketErr = errors.New("private coupon read")
	if status, observation := navigationGET(t, router); status != 500 || observation.Scene.Data != nil {
		t.Fatalf("partial observation: %d", status)
	}
	if store.writes != 0 || f.executions != 0 {
		t.Fatalf("GET wrote: %d %d", store.writes, f.executions)
	}
}

func TestTakarakujiShopObservesTheRound(t *testing.T) {
	f := newLotteryFixture(t)
	f.current = &lottery.TakarakujiTicket{ID: "t1"}
	_, router := f.router(t, "takarakuji")
	status, observation := navigationGET(t, router)
	data := decodeShopScene[TakarakujiSceneData](t, observation)
	if status != 200 || observation.Scene.Title != "宝くじ屋" || data.RoundID != 3 || data.RemainingTickets != 12 ||
		data.DrawDateLabel != "2026/11/21" || !data.Purchased || !offered(observation, "takarakuji_buy") {
		t.Fatalf("takarakuji shop: %d %+v", status, data)
	}
	f.statusErr = errors.New("private round read")
	if status, observation := navigationGET(t, router); status != 500 || observation.Scene.Data != nil {
		t.Fatalf("partial observation: %d", status)
	}
}

func TestRaffleMessagesFollowLegacy(t *testing.T) {
	seed := lottery.EvaluateRaffleRoll(lottery.RaffleStandard, 20, 0)
	miss := lottery.EvaluateRaffleRoll(lottery.RaffleStandard, 999, 0)
	orb := lottery.EvaluateRaffleRoll(lottery.RaffleSpecial, 20, 0)
	for _, tc := range []struct {
		name      string
		res       lottery.RaffleResult
		remaining int
		want      string
	}{
		{"hand", lottery.RaffleResult{RaffleType: lottery.RaffleStandard, TicketsUsed: 3, Prize: seed}, 4,
			"プニョプニョプニョ…コロコロコロ…...,,,● 【桃スライム】おおっ、当たりで〜す！おめでとうございま〜す！４等が出ました〜！こちらが４等の不思議な木の実です！はい。どうぞ！"},
		{"depot", lottery.RaffleResult{RaffleType: lottery.RaffleSpecial, TicketsUsed: 300, Prize: orb, TransferredToDepot: true}, 0,
			"ガラガラガラ…コロコロコロ…...,,,● 【レッドオーブ】おおっ！レッドオーブが出ました〜！おめでとうございま〜す！こちらがレッドオーブになります！レッドオーブは、勇者さんの預かり所に送っておきますね"},
		{"miss again", lottery.RaffleResult{RaffleType: lottery.RaffleStandard, TicketsUsed: 3, Prize: miss}, 7,
			"プニョプニョプニョ…コロコロコロ…...,,,● 【白スライム】 ハズレです…あと2回まわせるよ"},
		{"miss last", lottery.RaffleResult{RaffleType: lottery.RaffleStandard, TicketsUsed: 3, Prize: miss}, 2,
			"プニョプニョプニョ…コロコロコロ…...,,,● 【白スライム】 ハズレです…また挑戦してね"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newLotteryFixture(t)
			f.raffle, f.remaining = tc.res, tc.remaining
			_, router := f.router(t, "lottery")
			status, raw := gatewayRequest(t, router, "hero", "session", "application/json", saleGatewayBody("lottery_raffle", `{}`), nil)
			if got := tavernResult[LotteryRaffleResult](t, raw); status != 200 || got.Message != tc.want || got.RemainingTickets != tc.remaining {
				t.Fatalf("raffle: %d %q", status, got.Message)
			}
		})
	}
}

func TestTakarakujiPurchaseReplies(t *testing.T) {
	for _, tc := range []struct {
		name      string
		err       error
		purchased bool
		want      string
	}{
		{"bought", nil, true, "ありがとー。当たってたら 2026/11/21 に賞品が届くからね"},
		{"sold out", lottery.ErrSoldOut, false, "今回の宝くじは完売したよー"},
		{"one each", lottery.ErrAlreadyPurchased, false, "おひとりさまおひとつ！"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newLotteryFixture(t)
			f.char.Money = 0 // sold-out and one-per-person come before funds
			f.executionErr = tc.err
			_, router := f.router(t, "takarakuji")
			status, raw := gatewayRequest(t, router, "hero", "session", "application/json", saleGatewayBody("takarakuji_buy", `{}`), nil)
			got := tavernResult[TakarakujiBuyResult](t, raw)
			if status != 200 || f.executions != 1 || got.Purchased != tc.purchased || got.Message != tc.want || (got.Ticket != nil) != tc.purchased {
				t.Fatalf("buy: %d %+v", status, got)
			}
		})
	}
}

func TestLotteryCommandErrors(t *testing.T) {
	for _, tc := range []struct {
		scene, action string
		err           error
		code, text    string
		status        int
	}{
		{"lottery", "lottery_raffle", lottery.ErrInsufficientTickets, "LOTTERY_INSUFFICIENT_TICKETS", "福引券を 3 枚以上、持っていないようですねぇ…", 409},
		{"takarakuji", "takarakuji_buy", lottery.ErrInsufficientGold, "TAKARAKUJI_INSUFFICIENT_GOLD", "お金が足りないよ", 409},
		{"lottery", "lottery_raffle", errors.New("private storage failure"), "EXECUTION_FAILED", "", 500},
	} {
		t.Run(tc.code, func(t *testing.T) {
			f := newLotteryFixture(t)
			f.executionErr = fmt.Errorf("private wrapped: %w", tc.err)
			_, router := f.router(t, tc.scene)
			status, got := gatewayRequest(t, router, "hero", "session", "application/json", saleGatewayBody(tc.action, `{}`), nil)
			assertGatewayError(t, got, tc.code)
			if status != tc.status || strings.Contains(fmt.Sprint(got), "private") || !strings.Contains(string(got["error"]), tc.text) {
				t.Fatalf("error mapping: %d %s", status, got)
			}
		})
	}
}

func TestLotteryCommandsStayInTheirPlace(t *testing.T) {
	for _, tc := range []struct{ scene, action string }{{"takarakuji", "lottery_raffle"}, {"lottery", "takarakuji_buy"}, {"bank", "lottery_raffle"}} {
		f := newLotteryFixture(t)
		_, router := f.router(t, tc.scene)
		if status, got := gatewayRequest(t, router, "hero", "session", "application/json", saleGatewayBody(tc.action, `{}`), nil); status != 409 || f.executions != 0 {
			t.Fatalf("%s in %s: %d %s", tc.action, tc.scene, status, got)
		}
	}
}

func TestLotteryNPCWordsUseLiveFacts(t *testing.T) {
	f := newLotteryFixture(t)
	f.tickets = 300
	_, router := f.router(t, "lottery")
	h := &Handler{lottery: f}
	words, err := lotteryWords(h, context.Background(), corecharacter.Character{ID: "hero"})
	if err != nil || words[0] != "福引券300枚で１回まわすことができるよ" {
		t.Fatalf("raffle words: %v %v", words, err)
	}
	words, err = takarakujiWords(h, context.Background(), corecharacter.Character{})
	if err != nil || words[4] != "あと 12 人分の宝くじがあるよ" {
		t.Fatalf("takarakuji words: %v %v", words, err)
	}
	status, raw := gatewayRequest(t, router, "hero", "session", "application/json", saleGatewayBody("npc_inspect", `{}`), nil)
	if r := npcResult(t, raw); status != 200 || !r.Private || r.Dialogue != "なんと、勇者は福引券を見つけた！\n@フクスケ「ダメだよ！あげないよ！」" {
		t.Fatalf("inspect: %d %+v", status, r)
	}
}
