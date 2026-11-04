package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/auction"
	"github.com/witchcraze/party2re/internal/core/scheduling"
	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/playercontext"
)

type auctionGatewayFixture struct {
	*gatewayFixture
	AuctionService
	request auction.SendRequest
	result  auction.SendResult
}

func (f *auctionGatewayFixture) Send(ctx context.Context, req auction.SendRequest) (auction.SendResult, error) {
	f.checkContext(ctx)
	f.executions++
	f.request = req
	if f.afterExecute != nil {
		f.afterExecute()
	}
	return f.result, f.executionErr
}

func auctionGatewayRouter(t *testing.T) (*auctionGatewayFixture, timer.Service, http.Handler) {
	t.Helper()
	f := &auctionGatewayFixture{gatewayFixture: newGatewayFixture(t), result: auction.SendResult{
		SenderCharacterID: "hero", TargetCharacterID: "peer", TargetCharacterName: "相手", TransferredGold: 7, Message: "7 Gを 相手 に送りました",
	}}
	timers := timer.NewService(nil)
	h, err := NewHandler(f.gatewayFixture, f.gatewayFixture, &struct{ AdventureService }{}, &struct{ ShopService }{},
		WithAuction(f), WithPlayerContext(playercontext.NewService(f.gatewayFixture, f.gatewayFixture, timers)), func(h *Handler) { h.homes = f.gatewayFixture })
	if err != nil {
		t.Fatal(err)
	}
	return f, timers, h.Router()
}

func TestAuctionGatewaySendAndRecovery(t *testing.T) {
	for _, tc := range []struct {
		params string
		want   auction.SendRequest
	}{
		{`{"target_character_id":"peer","gold":7}`, auction.SendRequest{TargetCharacterID: "peer", Gold: 7}},
		{`{"target_character_name":"相手","slot":"weapon"}`, auction.SendRequest{TargetCharacterName: "相手", Slot: "weapon"}},
		{`{"target_character_id":"peer","instance_id":"held"}`, auction.SendRequest{TargetCharacterID: "peer", InstanceID: "held"}},
		{`{"target_character_id":"peer","target_character_name":"ignored","gold":7,"slot":"armor","instance_id":"held"}`, auction.SendRequest{TargetCharacterID: "peer", TargetCharacterName: "ignored", Gold: 7, Slot: "armor", InstanceID: "held"}},
	} {
		for _, failure := range []string{"", "query", "profile"} {
			t.Run(tc.params+failure, func(t *testing.T) {
				f, _, router := auctionGatewayRouter(t)
				f.expectedContext = context.WithValue(context.Background(), struct{}{}, "auction request")
				if tc.want.Gold == 0 {
					f.result.TransferredGold = 0
					f.result.TransferredItem = &auction.TransferredItemInfo{InstanceID: "delivered", DefinitionID: "weapon-01", ItemName: "武器", EnhancementLevel: 6}
					f.result.Message = "武器を相手に送りました"
				}
				f.afterExecute = func() {
					if failure == "query" {
						f.queryErr = errors.New("private query")
					}
					if failure == "profile" {
						f.profileErr = errors.New("private profile")
					}
				}
				status, got := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"auction_send","params":`+tc.params+`}`, f.expectedContext)
				want := tc.want
				want.SenderCharacterID = "hero"
				var result auction.SendResult
				if err := json.Unmarshal(got["result"], &result); err != nil {
					t.Fatal(err)
				}
				if status != 200 || f.executions != 1 || !reflect.DeepEqual(f.request, want) || !reflect.DeepEqual(result, f.result) {
					t.Fatalf("status=%d request=%+v result=%+v: %s", status, f.request, result, got)
				}
				if failure == "" {
					assertGatewayContext(t, got)
				} else {
					assertLoopRefreshFailure(t, got)
				}
				f.queryErr, f.profileErr, f.expectedContext = nil, nil, nil
				if status, _ := navigationGET(t, router); status != 200 || f.executions != 1 {
					t.Fatalf("GET recovery: %d calls=%d", status, f.executions)
				}
			})
		}
	}
}

func TestAuctionGatewayStrictParams(t *testing.T) {
	for _, params := range []string{`null`, `[]`, `1`, `{"sender_character_id":"other"}`, `{"character_id":"other"}`, `{"player_id":"other"}`, `{"extra":1}`, `{"gold":"7"}`, `{"gold":1.5}`, `{"gold":9223372036854775808}`, `{"gold":null}`, `{"slot":null}`, `{"instance_id":null}`, `{"target_character_id":null}`, `{"target_character_name":null}`, `{"target_character_id":7}`} {
		t.Run(params, func(t *testing.T) {
			f, _, router := auctionGatewayRouter(t)
			status, got := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"auction_send","params":`+params+`}`, nil)
			if status != 400 || f.executions != 0 || f.queryCalls != 0 {
				t.Fatalf("status=%d calls=%d: %s", status, f.executions, got)
			}
			assertGatewayError(t, got, "INVALID_ACTION_PARAMS")
		})
	}
}

func TestAuctionGatewayRejections(t *testing.T) {
	for _, tc := range []struct {
		err    error
		status int
		code   string
	}{
		{auction.ErrCannotSendToSelf, 400, "AUCTION_SELF_TRANSFER"},
		{auction.ErrInvalidSendTarget, 400, "AUCTION_INVALID_TARGET"},
		{auction.ErrNothingToSend, 400, "AUCTION_NOTHING_TO_SEND"},
		{auction.ErrInvalidSendAmount, 400, "AUCTION_INVALID_AMOUNT"},
		{auction.ErrInsufficientMoney, 400, "AUCTION_INSUFFICIENT_FUNDS"},
		{auction.ErrDepotFull, 400, "AUCTION_DEPOT_FULL"},
		{auction.ErrTabooItem, 400, "AUCTION_TABOO_ITEM"},
		{auction.ErrItemNotEquipped, 400, "AUCTION_ITEM_NOT_EQUIPPED"},
		{auction.ErrInvalidSlot, 400, "AUCTION_INVALID_SLOT"},
		{auction.ErrTargetNotFound, 404, "AUCTION_TARGET_NOT_FOUND"},
		{auction.ErrItemNotFound, 404, "AUCTION_ITEM_NOT_FOUND"},
		{errors.New("private storage failure"), 500, "EXECUTION_FAILED"},
		{context.Canceled, 500, "EXECUTION_FAILED"},
	} {
		for _, failRefresh := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/%v/%t", tc.code, tc.err, failRefresh), func(t *testing.T) {
				f, _, router := auctionGatewayRouter(t)
				f.executionErr = fmt.Errorf("wrapped: %w", tc.err)
				if failRefresh {
					f.afterExecute = func() { f.profileErr = errors.New("private profile") }
				}
				status, got := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"auction_send","params":{}}`, nil)
				if status != tc.status || f.executions != 1 {
					t.Fatalf("status=%d: %s", status, got)
				}
				assertGatewayError(t, got, tc.code)
				if strings.Contains(fmt.Sprint(got), "private") {
					t.Fatalf("private error disclosed: %s", got)
				}
				if tc.status == 500 {
					if _, ok := got["context"]; ok {
						t.Fatalf("unknown outcome refreshed: %s", got)
					}
				} else if failRefresh {
					assertLoopRefreshFailure(t, got)
				} else {
					assertGatewayContext(t, got)
				}
			})
		}
	}
}

func TestAuctionGatewayAdmission(t *testing.T) {
	for _, condition := range []string{"auth", "owner", "sleep", "wake", "shared sleep", "shared wake", "work", "overdue", "processing", "query error", "sleep error", "unconfigured"} {
		t.Run(condition, func(t *testing.T) {
			f, timers, router := auctionGatewayRouter(t)
			actor, token, want, code := "hero", "session", 409, "ACTION_UNAVAILABLE"
			switch condition {
			case "auth":
				token, want = "", 401
			case "owner":
				actor, want = "other", 403
			case "sleep", "wake":
				category := timer.CategorySleep
				if condition == "wake" {
					category = timer.CategoryAsleep
				}
				if err := timers.SetLock(context.Background(), category, "hero", time.Hour); err != nil {
					t.Fatal(err)
				}
			case "shared sleep":
				f.sleep.Sleeping = true
			case "shared wake":
				f.sleep.CanWake = true
			case "work", "overdue", "processing":
				a := scheduling.ScheduledAction{ID: "work", State: scheduling.StatePending, ExecuteAt: time.Now().Add(time.Hour)}
				if condition != "work" {
					a.ExecuteAt = time.Now().Add(-time.Hour)
				}
				if condition == "processing" {
					a.State = scheduling.StateProcessing
				}
				f.actions = []scheduling.ScheduledAction{a}
			case "query error":
				f.queryErr, want, code = errors.New("private query"), 500, "ACTION_PREFLIGHT_FAILED"
			case "sleep error":
				f.sleepErr, want, code = errors.New("private sleep"), 500, "ACTION_PREFLIGHT_FAILED"
			case "unconfigured":
				h, err := NewHandler(f.gatewayFixture, f.gatewayFixture, &struct{ AdventureService }{}, &struct{ ShopService }{}, WithAuction(nil))
				if err != nil {
					t.Fatal(err)
				}
				router, want, code = h.Router(), 501, "ACTION_NOT_IMPLEMENTED"
			}
			status, got := gatewayRequest(t, router, actor, token, "application/json", `{"action":"auction_send","params":{"target_character_id":"peer","gold":7}}`, nil)
			if status != want || f.executions != 0 {
				t.Fatalf("status=%d calls=%d: %s", status, f.executions, got)
			}
			if condition != "auth" && condition != "owner" {
				assertGatewayError(t, got, code)
			}
		})
	}
}

func TestAuctionSendRetiredRoutes(t *testing.T) {
	f, _, router := auctionGatewayRouter(t)
	for _, path := range []string{"/characters/hero/auction/send", "/auction/send"} {
		r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"sender_character_id":"hero","target_character_id":"peer","gold":7}`))
		r.Header.Set("Authorization", "Bearer session")
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		if w.Code != 404 || f.executions != 0 {
			t.Fatalf("%s: status=%d calls=%d", path, w.Code, f.executions)
		}
	}
}
