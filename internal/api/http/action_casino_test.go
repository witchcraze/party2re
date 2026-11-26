package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/casino"
	"github.com/witchcraze/party2re/internal/character"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/core/scheduling"
	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/placelog"
	"github.com/witchcraze/party2re/internal/playercontext"
	"github.com/witchcraze/party2re/internal/testutil"
	vk "github.com/witchcraze/party2re/internal/valkey"
)

// casinoCounter replaces the counter operations that need the economy.
type casinoCounter struct {
	CasinoService
	spin  casino.SpinResult
	err   error
	calls []string
}

func (c *casinoCounter) SpinSlot(_ context.Context, actor string, bet int64) (casino.SpinResult, casino.Account, error) {
	c.calls = append(c.calls, "slot")
	c.spin.BetCoins = bet
	return c.spin, casino.Account{CharacterID: actor, Coins: 990}, c.err
}

func (c *casinoCounter) ExchangeGoldToCoins(_ context.Context, actor string, coins int64) (casino.Account, corecharacter.Character, error) {
	c.calls = append(c.calls, "exchange")
	return casino.Account{CharacterID: actor, Coins: coins}, corecharacter.Character{ID: actor, Money: 80}, c.err
}

func (c *casinoCounter) ExchangePrize(_ context.Context, _ string, cost int64, count int) (casino.PrizeExchangeResult, error) {
	c.calls = append(c.calls, "prize")
	prize, _ := casino.GetPrizeByCost(cost)
	return casino.PrizeExchangeResult{Prize: prize, Count: count, TotalCostCoins: cost * int64(count), TransferredToDepot: true}, c.err
}

func TestCasinoGatewayCounterCommands(t *testing.T) {
	for _, tc := range []struct {
		name, action, params string
		err                  error
		status               int
		code, field, want    string
	}{
		{"slot miss", "casino_slot", `{"bet":10}`, nil, 200, "", "message", "$10スロット\n【∞】【♪】【７】\nハズレ"},
		{"exchange", "casino_exchange", `{"coins":6}`, nil, 200, "", "npc_speech", "6枚のコインと両替しました"},
		{"prize", "casino_prize_exchange", `{"cost_coins":100,"count":2}`, nil, 200, "", "npc_speech", ""},
		{"insufficient coins", "casino_slot", `{"bet":10}`, casino.ErrInsufficientCoins, 422, "CASINO_INSUFFICIENT_COINS", "", ""},
		{"insufficient gold", "casino_exchange", `{"coins":6}`, casino.ErrInsufficientGold, 422, "CASINO_INSUFFICIENT_GOLD", "", ""},
		{"unknown prize", "casino_prize_exchange", `{"cost_coins":1,"count":1}`, casino.ErrPrizeNotFound, 400, "CASINO_INVALID_INPUT", "", ""},
		{"job 200", "casino_slot", `{"bet":200}`, casino.ErrJobNotEligibleForSlot200, 422, "CASINO_SLOT_JOB_REQUIRED", "", ""},
		{"unknown failure", "casino_exchange", `{"coins":6}`, errors.New("store down"), 500, "EXECUTION_FAILED", "", ""},
		{"reverse direction", "casino_exchange", `{"coins":6,"direction":"coins_to_gold"}`, nil, 400, "INVALID_ACTION_PARAMS", "", ""},
		{"missing bet", "casino_slot", `{}`, nil, 400, "INVALID_ACTION_PARAMS", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newCasinoSceneFixture(t)
			counter := &casinoCounter{CasinoService: f.service, spin: casino.SpinResult{Reels: [3]casino.SlotSymbol{casino.SymbolCherry, casino.SymbolNote, casino.SymbolSeven}}, err: tc.err}
			f.reads.CasinoService = counter
			status, raw := gatewayRequest(t, f.router(), "hero", "session", "application/json", `{"action":"`+tc.action+`","params":`+tc.params+`}`, nil)
			if status != tc.status {
				t.Fatalf("status %d: %s", status, raw)
			}
			if tc.code != "" {
				var e struct{ Error ErrorDetail }
				if err := json.Unmarshal(mustJSON(t, raw), &e); err != nil || e.Error.Code != tc.code {
					t.Fatalf("error %s: %v", raw, err)
				}
				if tc.code == "INVALID_ACTION_PARAMS" && len(counter.calls) != 0 {
					t.Fatal("invalid params reached the service")
				}
				return
			}
			var result map[string]any
			if err := json.Unmarshal(raw["result"], &result); err != nil {
				t.Fatal(err)
			}
			if tc.action == "casino_prize_exchange" {
				prize, _ := casino.GetPrizeByCost(100)
				tc.want = "コイン100枚の賞品2個と交換ですね！" + prize.ItemName + "は" + f.char.Name + "の預かり所に送っておきました"
			}
			if result[tc.field] != tc.want {
				t.Fatalf("%s = %q, want %q", tc.field, result[tc.field], tc.want)
			}
		})
	}
}

// Legacy ＄すろっと sets $m{wt} to a tenth of $act_time, which the next request
// truncates to no wait (lib/system.cgi:159), yet a running wait still rejects it.
func TestCasinoSlotStartsNoActionWait(t *testing.T) {
	f := newCasinoSceneFixture(t)
	waits := timer.NewService(nil)
	f.waits = &waits
	counter := &casinoCounter{CasinoService: f.service}
	f.reads.CasinoService = counter
	router := f.router()
	slot := `{"action":"casino_slot","params":{"bet":1}}`
	for range 2 {
		if status, raw := gatewayRequest(t, router, "hero", "session", "application/json", slot, nil); status != 200 || waitMS(t, raw) != 0 {
			t.Fatalf("slot: %d %s", status, raw)
		}
	}
	if status, raw := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"casino_exchange","params":{"coins":1}}`, nil); status != 200 || waitMS(t, raw) <= 0 {
		t.Fatalf("exchange must start the wait: %d %s", status, raw)
	}
	if status, raw := gatewayRequest(t, router, "hero", "session", "application/json", slot, nil); status != http.StatusConflict || actionErrorCode(t, raw) != "ACTION_WAIT" {
		t.Fatalf("slot during wait: %d %s", status, raw)
	}
	if !reflect.DeepEqual(counter.calls, []string{"slot", "slot", "exchange"}) {
		t.Fatalf("calls %v", counter.calls)
	}
}

// Legacy room commands wait the room speed and a kicked member waits 60 s
// (_casino.cgi:57,114,167); rejections start no wait.
func TestCasinoRoomCommandsWaitRoomSpeed(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct{ name, action, params string }{
		{"start", "casino_room_start", `{"room_id":"room"}`},
		{"kick", "casino_room_kick", `{"room_id":"room","target_character_id":"peer"}`},
		{"leave", "casino_room_leave", `{"room_id":"room"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newCasinoSceneFixture(t)
			waits := timer.NewService(nil)
			f.waits = &waits
			f.room("room", casino.GameTypeIndian, true)
			router := f.router()
			remaining := func(id string) time.Duration {
				t.Helper()
				d, err := waits.GetRemainingLock(ctx, timer.CategoryActionWait, id)
				if err != nil {
					t.Fatal(err)
				}
				return d
			}
			status, raw := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"casino_room_kick","params":{"room_id":"room","target_character_id":"hero"}}`, nil)
			if status != 403 || remaining("hero") != 0 {
				t.Fatalf("rejected kick: %d %s wait %v", status, raw, remaining("hero"))
			}
			status, raw = gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"`+tc.action+`","params":`+tc.params+`}`, nil)
			speed := time.Duration(casino.SpeedNormal) * time.Second
			if got := remaining("hero"); status != 200 || got <= playercontext.DefaultActionWait || got > speed {
				t.Fatalf("%s: %d %s wait %v, want room speed %v", tc.action, status, raw, got, speed)
			}
			if got := remaining("peer"); (tc.action == "casino_room_kick") != (got > 59*time.Second) {
				t.Fatalf("peer wait %v", got)
			}
		})
	}
}

func mustJSON(t *testing.T, raw map[string]json.RawMessage) []byte {
	t.Helper()
	b, err := json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestCasinoSlotMessage(t *testing.T) {
	seven, cherry, note := casino.SymbolSeven, casino.SymbolCherry, casino.SymbolNote
	for _, tc := range []struct {
		spin casino.SpinResult
		want string
	}{
		{casino.EvaluateSpin(1, [3]casino.SlotSymbol{seven, seven, seven}), "$1スロット\n【７】【７】【７】\nなんと!! ７ が3つそろいました!!\nおめでとうございます!!\n***** コイン 100 枚 GET !! *****"},
		{casino.EvaluateSpin(10, [3]casino.SlotSymbol{cherry, cherry, note}), "$10スロット\n【∞】【∞】【♪】\nチェリーが2つそろいました♪\nコイン 30 枚Up♪"},
		{casino.SpinResult{BetCoins: 10, Reels: [3]casino.SlotSymbol{cherry, cherry, note}, IsWin: true, PayoutCoins: 45, BonusCoins: 15}, "$10スロット\n【∞】【∞】【♪】\nチェリーが2つそろいました♪\nコイン 30 枚Up♪\nコインがちょっとおまけされた！"},
	} {
		if got := casinoSlotMessage(tc.spin); got != tc.want {
			t.Errorf("got %q, want %q", got, tc.want)
		}
	}
}

// TestCasinoGatewayRoomGame plays a Doppel round as leader through the Gateway
// while the peer acts through the service.
func TestCasinoGatewayRoomGame(t *testing.T) {
	ctx := context.Background()
	f := newCasinoSceneFixture(t)
	router := f.router()
	post := func(action, params string, want int) map[string]json.RawMessage {
		t.Helper()
		status, raw := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"`+action+`","params":`+params+`}`, nil)
		if status != want {
			t.Fatalf("%s: %d %s", action, status, raw)
		}
		return raw
	}
	offered := func() ([]string, *CasinoRoomSceneData) {
		t.Helper()
		status, response := navigationGET(t, router)
		if status != 200 {
			t.Fatalf("GET %d", status)
		}
		var ids []string
		for _, a := range response.AvailableActions {
			ids = append(ids, a.Action)
		}
		if response.Scene.Kind != "activity" {
			return ids, nil
		}
		return ids, decodeSceneData[ActivitySceneData](t, response).Casino
	}
	if ids, _ := offered(); !slices.Contains(ids, "casino_room_create") || !slices.Contains(ids, "npc_talk") {
		t.Fatalf("lobby offers %v", ids)
	}
	raw := post("casino_room_create", `{"name":"cards","game_type":"doppel","speed":18,"max_players":2,"rate":10,"allow_spectators":true}`, 200)
	var created map[string]string
	if err := json.Unmarshal(raw["result"], &created); err != nil || len(created) != 1 || created["room_id"] == "" {
		t.Fatalf("create result must carry only the room: %s %v", raw["result"], err)
	}
	room := created["room_id"]
	if _, err := f.service.JoinRoom(ctx, room, "peer", "", 0); err != nil {
		t.Fatal(err)
	}
	ids, data := offered()
	if data == nil || !reflect.DeepEqual(data.Controls.KickTargets, []string{"peer"}) || !slices.Contains(ids, "casino_room_kick") || !slices.Contains(ids, "casino_room_start") {
		t.Fatalf("waiting room: %v %+v", ids, data)
	}
	post("casino_room_kick", `{"room_id":"`+room+`","target_character_id":"hero"}`, 403)
	post("casino_room_start", `{"room_id":"`+room+`"}`, 200)
	ids, data = offered()
	if data == nil || len(data.Controls.Actions) != 3 || !slices.Contains(ids, "casino_room_action") || slices.Contains(ids, "casino_room_kick") {
		t.Fatalf("started room: %v %+v", ids, data)
	}
	mine, theirs := data.Controls.Actions[0], data.Controls.Actions[1]
	post("casino_room_action", `{"room_id":"`+room+`","action":"`+mine+`","mark":"0"}`, 400)
	raw = post("casino_room_action", `{"room_id":"`+room+`","action":"`+mine+`"}`, 200)
	if string(raw["result"]) != `{"room_id":"`+room+`"}` {
		t.Fatalf("action result leaked room state: %s", raw["result"])
	}
	// Doppel reselection before every mark is in takes no second wager.
	post("casino_room_action", `{"room_id":"`+room+`","action":"`+mine+`"}`, 200)
	if _, err := f.service.PlayRoomAction(ctx, room, "peer", theirs); err != nil {
		t.Fatal(err)
	}
	// No match: the leader takes the pot of both wagers.
	if f.accounts.coins["hero"] != 1010 || f.accounts.coins["peer"] != 990 {
		t.Fatalf("coins = %v", f.accounts.coins)
	}
	post("casino_room_leave", `{"room_id":"`+room+`"}`, 200)
	if ids, data := offered(); data != nil || !slices.Contains(ids, "casino_room_create") || f.char.Tired != 1 {
		t.Fatalf("left room: %v %+v tired=%d", ids, data, f.char.Tired)
	}
}

func TestCasinoGatewayEntryRejections(t *testing.T) {
	for _, tc := range []struct {
		name, action, params string
		status               int
		code                 string
	}{
		{"wrong password", "casino_room_join", `{"room_id":"room","password":"guess"}`, 403, "CASINO_INVALID_PASSWORD"},
		{"spectate wrong password", "casino_room_spectate", `{"room_id":"room","password":"guess"}`, 403, "CASINO_INVALID_PASSWORD"},
		{"other room", "casino_room_join", `{"room_id":"other","password":"secret"}`, 409, "ACTION_UNAVAILABLE"},
		{"spectate", "casino_room_spectate", `{"room_id":"room","password":"secret"}`, 200, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newCasinoSceneFixture(t)
			f.room("room", casino.GameTypeIndian, false)
			f.store.selection.Subject = playercontext.Subject{Kind: "room", ID: "room"}
			status, raw := gatewayRequest(t, f.router(), "hero", "session", "application/json", `{"action":"`+tc.action+`","params":`+tc.params+`}`, nil)
			var e struct{ Error ErrorDetail }
			if err := json.Unmarshal(mustJSON(t, raw), &e); err != nil || status != tc.status || e.Error.Code != tc.code {
				t.Fatalf("%d %s %v", status, raw, err)
			}
		})
	}
}

type faultyKickWaitService struct {
	timer.Service
	target     string
	failExtend atomic.Bool
}

func (w *faultyKickWaitService) ImposeLockUntil(ctx context.Context, category, targetID string, until time.Time) error {
	if category == timer.CategoryActionWait && targetID == w.target && w.failExtend.Load() {
		return errors.New("injected target wait write failure")
	}
	return w.Service.ImposeLockUntil(ctx, category, targetID, until)
}

type multiActorGateway struct {
	*gatewayFixture
	peer corecharacter.Character
}

func (g *multiActorGateway) Authenticate(ctx context.Context, token string) (coreplayer.Player, error) {
	if token == "peer-session" {
		return coreplayer.Player{ID: g.peer.PlayerID}, nil
	}
	return g.gatewayFixture.Authenticate(ctx, token)
}

func (g *multiActorGateway) Get(ctx context.Context, id string) (corecharacter.Character, error) {
	if id == g.peer.ID {
		return g.peer, nil
	}
	return g.gatewayFixture.Get(ctx, id)
}

func (g *multiActorGateway) FindByID(ctx context.Context, id string) (corecharacter.Character, error) {
	if id == g.peer.ID {
		return g.peer, nil
	}
	return g.gatewayFixture.FindByID(ctx, id)
}

func (g *multiActorGateway) FindPendingByActorID(ctx context.Context, id string) ([]scheduling.ScheduledAction, error) {
	if id == g.peer.ID {
		return nil, nil
	}
	return g.gatewayFixture.FindPendingByActorID(ctx, id)
}

type multiActorCasinoChars struct {
	*casinoSceneCharacters
	peer  corecharacter.Character
	peer2 corecharacter.Character
}

func (r *multiActorCasinoChars) FindByID(ctx context.Context, id string) (corecharacter.Character, error) {
	if id == r.peer.ID {
		return r.peer, nil
	}
	if id == r.peer2.ID {
		return r.peer2, nil
	}
	return r.casinoSceneCharacters.FindByID(ctx, id)
}

func (r *multiActorCasinoChars) FindByIDForUpdate(ctx context.Context, id string) (corecharacter.Character, error) {
	return r.FindByID(ctx, id)
}

func (r *multiActorCasinoChars) Update(_ context.Context, c corecharacter.Character) error {
	switch c.ID {
	case r.peer.ID:
		r.peer = c
	case r.peer2.ID:
		r.peer2 = c
	default:
		r.f.char = c
	}
	return nil
}

func newMultiActorKickFixture(t *testing.T) (f *casinoSceneFixture, waits *faultyKickWaitService, departures *casino.MemoryDepartureRepository, router http.Handler) {
	t.Helper()
	f = newCasinoSceneFixture(t)
	departures = casino.NewMemoryDepartureRepository()
	peer := corecharacter.Character{ID: "peer", PlayerID: "peer-owner", Name: "peer", Money: 100, Stats: corecharacter.Stats{HP: 50}}
	peer2 := corecharacter.Character{ID: "peer2", PlayerID: "peer-owner", Name: "peer2", Money: 100, Stats: corecharacter.Stats{HP: 50}}
	chars := &multiActorCasinoChars{casinoSceneCharacters: f.characters, peer: peer, peer2: peer2}
	var err error
	f.service, err = casino.NewService(f.accounts, casino.WithDepartureRepository(departures), casino.WithRoomRepository(f.rooms), casino.WithCharacterRepository(chars))
	if err != nil {
		t.Fatal(err)
	}
	f.reads = &casinoSceneReads{CasinoService: f.service, f: f.gatewayFixture}
	waits = &faultyKickWaitService{Service: timer.NewService(nil), target: "peer"}
	gw := &multiActorGateway{gatewayFixture: f.gatewayFixture, peer: peer}
	scene := playercontext.SceneDefinition{ID: "casino", Parent: "town", Pageable: true, SubjectKind: "room", SubjectAvailable: func(ctx context.Context, _, id string) (bool, error) {
		rows, err := f.service.ListRooms(ctx)
		return slices.ContainsFunc(rows, func(r casino.RoomSummary) bool { return r.ID == id }), err
	}}
	pc := playercontext.NewService(gw, gw, waits, playercontext.WithNavigation(f.store, playercontext.SceneDefinition{ID: "town"}, scene), playercontext.WithActivities(f.readActivities), playercontext.WithActionWait(waits, playercontext.DefaultActionWait))
	f.service.SetActionWait(pc)
	rescueCmd := withActionCommand("rescue_request", func(ctx context.Context, id string, p rescueActionParams) (any, error) {
		return f.execute(ctx, id, 0)
	}, gatewayRejection)
	h, err := NewHandler(gw, gw, &struct{ AdventureService }{}, &struct{ ShopService }{}, WithPlayerContext(pc), rescueCmd, func(h *Handler) { h.homes = f }, WithCasino(f.reads))
	if err != nil {
		t.Fatal(err)
	}
	f.room("room", casino.GameTypeIndian, true)
	return f, waits, departures, h.Router()
}

func contextGET(t *testing.T, router http.Handler, actor, token string) (int, PlayerContextResponse) {
	t.Helper()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/characters/"+actor+"/context", nil)
	r.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, r)
	var response PlayerContextResponse
	if w.Code == http.StatusOK {
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
	}
	return w.Code, response
}

func TestCasinoKickWaitFailureAndRecovery(t *testing.T) {
	ctx := context.Background()
	remaining := func(t *testing.T, waits timer.Service, id string) time.Duration {
		t.Helper()
		d, err := waits.GetRemainingLock(ctx, timer.CategoryActionWait, id)
		if err != nil {
			t.Fatal(err)
		}
		return d
	}

	t.Run("healthy kick enforces peer 60s wait and rejects peer actions", func(t *testing.T) {
		f, waits, _, router := newMultiActorKickFixture(t)
		status, raw := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"casino_room_kick","params":{"room_id":"room","target_character_id":"peer"}}`, nil)
		if status != http.StatusOK || string(raw["success"]) != "true" {
			t.Fatalf("kick failed: %d %s", status, raw)
		}
		if got := remaining(t, waits, "hero"); got <= playercontext.DefaultActionWait || got > time.Duration(casino.SpeedNormal)*time.Second {
			t.Fatalf("hero wait=%v, want room speed", got)
		}
		if got := remaining(t, waits, "peer"); got <= 58*time.Second || got > 60*time.Second {
			t.Fatalf("peer wait=%v, want ~60s", got)
		}
		if _, err := f.rooms.GetMember(ctx, "room", "peer"); !errors.Is(err, casino.ErrMemberNotFound) {
			t.Fatalf("peer remained in room: %v", err)
		}
		// Peer is rejected by ACTION_WAIT on subsequent Gateway commands.
		status, raw = gatewayRequest(t, router, "peer", "peer-session", "application/json", `{"action":"scene_enter","params":{"destination":"town"}}`, nil)
		if status != http.StatusConflict || actionErrorCode(t, raw) != "ACTION_WAIT" {
			t.Fatalf("peer action during kick wait: %d %s", status, raw)
		}
	})

	t.Run("wait write failure keeps pending evidence and recovers on hero retry", func(t *testing.T) {
		_, waits, departures, router := newMultiActorKickFixture(t)
		waits.failExtend.Store(true)
		status, raw := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"casino_room_kick","params":{"room_id":"room","target_character_id":"peer"}}`, nil)
		if status != http.StatusInternalServerError {
			t.Fatalf("expected 500 on wait write failure, got %d %s", status, raw)
		}
		if got := remaining(t, waits, "hero"); got != 0 {
			t.Fatalf("failed kick kept hero claim: %v", got)
		}
		if got := remaining(t, waits, "peer"); got != 0 {
			t.Fatalf("unexpected peer wait while write failed: %v", got)
		}
		d, err := departures.FindDeparture(ctx, "room")
		if err != nil || !d.Pending || !d.Kicked || d.Member.CharacterID != "peer" || d.WaitUntil.IsZero() {
			t.Fatalf("missing pending kick evidence: %+v %v", d, err)
		}

		// GET must stay read-only and never repair wait or membership even after fault heals.
		waits.failExtend.Store(false)
		for _, actor := range []struct{ id, token string }{{"hero", "session"}, {"peer", "peer-session"}} {
			if getStatus, _ := contextGET(t, router, actor.id, actor.token); getStatus != http.StatusInternalServerError {
				t.Fatalf("GET %s hid pending recovery: %d", actor.id, getStatus)
			}
		}
		if got := remaining(t, waits, "peer"); got != 0 {
			t.Fatalf("GET wrote wait during pending recovery: %v", got)
		}

		// Hero retries the kick: recovery completes peer's 60s wait and returns 200 idempotently.
		status, raw = gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"casino_room_kick","params":{"room_id":"room","target_character_id":"peer"}}`, nil)
		if status != http.StatusOK || string(raw["success"]) != "true" {
			t.Fatalf("retry kick recovery failed: %d %s", status, raw)
		}
		if got := remaining(t, waits, "hero"); got <= playercontext.DefaultActionWait || got > time.Duration(casino.SpeedNormal)*time.Second {
			t.Fatalf("hero wait after retry=%v", got)
		}
		if got := remaining(t, waits, "peer"); got <= 58*time.Second || got > 60*time.Second {
			t.Fatalf("peer wait after retry=%v, want ~60s", got)
		}
		status, raw = gatewayRequest(t, router, "peer", "peer-session", "application/json", `{"action":"scene_enter","params":{"destination":"town"}}`, nil)
		if status != http.StatusConflict || actionErrorCode(t, raw) != "ACTION_WAIT" {
			t.Fatalf("peer not blocked after retry recovery: %d %s", status, raw)
		}
	})

	t.Run("kick claimed after peer preflight recovers instead of failing preflight", func(t *testing.T) {
		f, waits, departures, router := newMultiActorKickFixture(t)
		waits.failExtend.Store(true)
		if status, _ := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"casino_room_kick","params":{"room_id":"room","target_character_id":"peer"}}`, nil); status != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d", status)
		}
		waits.failExtend.Store(false)
		f.reads.missedRecoveries = 1

		status, raw := gatewayRequest(t, router, "peer", "peer-session", "application/json", `{"action":"casino_room_leave","params":{"room_id":"room"}}`, nil)
		if status != http.StatusConflict || actionErrorCode(t, raw) != "ACTION_WAIT" {
			t.Fatalf("expected ACTION_WAIT after recovering the late kick, got %d %s", status, raw)
		}
		if claim, err := departures.FindPendingDeparture(ctx, "peer"); err != nil || claim != "" {
			t.Fatalf("pending claim remained: %q %v", claim, err)
		}
	})

	t.Run("peer mutation recovers pending kick wait and rejects with ACTION_WAIT", func(t *testing.T) {
		_, waits, departures, router := newMultiActorKickFixture(t)
		waits.failExtend.Store(true)
		status, _ := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"casino_room_kick","params":{"room_id":"room","target_character_id":"peer"}}`, nil)
		if status != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d", status)
		}
		waits.failExtend.Store(false)

		// Without hero retrying, peer's next Gateway mutation recovers the pending kick and blocks peer.
		status, raw := gatewayRequest(t, router, "peer", "peer-session", "application/json", `{"action":"casino_room_join","params":{"room_id":"room","password":"secret"}}`, nil)
		if status != http.StatusConflict || actionErrorCode(t, raw) != "ACTION_WAIT" {
			t.Fatalf("expected ACTION_WAIT after peer recovery, got %d %s", status, raw)
		}
		if got := remaining(t, waits, "peer"); got <= 58*time.Second || got > 60*time.Second {
			t.Fatalf("peer wait=%v, want ~60s", got)
		}
		if claim, err := departures.FindPendingDeparture(ctx, "peer"); err != nil || claim != "" {
			t.Fatalf("pending claim remained: %q %v", claim, err)
		}
		if getStatus, obs := contextGET(t, router, "hero", "session"); getStatus != http.StatusOK || obs.Scene.Kind != "activity" {
			t.Fatalf("hero context after peer recovery: %d %+v", getStatus, obs)
		}
	})

	t.Run("lost response retry does not reset wait and voluntary leave rejects kick", func(t *testing.T) {
		f, waits, _, router := newMultiActorKickFixture(t)
		if status, _ := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"casino_room_kick","params":{"room_id":"room","target_character_id":"peer"}}`, nil); status != http.StatusOK {
			t.Fatalf("kick status=%d", status)
		}
		// Simulate hero's room-speed wait elapsing while peer still has >40s kick wait.
		if err := waits.ReleaseLock(ctx, timer.CategoryActionWait, "hero"); err != nil {
			t.Fatal(err)
		}
		if err := waits.SetLock(ctx, timer.CategoryActionWait, "peer", 40*time.Second); err != nil {
			t.Fatal(err)
		}
		status, raw := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"casino_room_kick","params":{"room_id":"room","target_character_id":"peer"}}`, nil)
		if status != http.StatusOK {
			t.Fatalf("idempotent retry status=%d %s", status, raw)
		}
		if got := remaining(t, waits, "peer"); got > 41*time.Second {
			t.Fatalf("idempotent retry reset peer wait to %v", got)
		}

		// A non-kicked member who left voluntarily must not be treated as a kick replay.
		if err := f.rooms.AddMember(ctx, casino.RoomMember{RoomID: "room", CharacterID: "peer2", JoinedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}); err != nil {
			t.Fatal(err)
		}
		if err := f.service.LeaveRoom(ctx, "room", "peer2"); err != nil {
			t.Fatal(err)
		}
		if err := waits.ReleaseLock(ctx, timer.CategoryActionWait, "hero"); err != nil {
			t.Fatal(err)
		}
		status, raw = gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"casino_room_kick","params":{"room_id":"room","target_character_id":"peer2"}}`, nil)
		if status != http.StatusNotFound || actionErrorCode(t, raw) != "CASINO_MEMBER_NOT_FOUND" {
			t.Fatalf("kick after voluntary leave: %d %s", status, raw)
		}
		if got := remaining(t, waits, "peer2"); got != 0 {
			t.Fatalf("rejected kick created wait for peer2: %v", got)
		}
	})
}

type pausedLeaveCasinoService struct {
	CasinoService
	target       string
	leaveEntered chan struct{}
	leaveProceed chan struct{}
	pausedOnce   atomic.Bool
}

func (s *pausedLeaveCasinoService) LeaveRoom(ctx context.Context, roomID, characterID string) error {
	if (s.target == "" || characterID == s.target) && s.pausedOnce.CompareAndSwap(false, true) {
		close(s.leaveEntered)
		select {
		case <-s.leaveProceed:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return s.CasinoService.LeaveRoom(ctx, roomID, characterID)
}

type raceSafeGatewayChars struct {
	CharacterService
	mu       sync.Mutex
	chars    map[string]corecharacter.Character
	accounts *casinoSceneAccounts
}

func (r *raceSafeGatewayChars) Authenticate(_ context.Context, token string) (coreplayer.Player, error) {
	switch token {
	case "session":
		return coreplayer.Player{ID: "owner"}, nil
	case "peer-session":
		return coreplayer.Player{ID: "peer-owner"}, nil
	default:
		return coreplayer.Player{}, errors.New("invalid session")
	}
}

func (r *raceSafeGatewayChars) Get(_ context.Context, id string) (corecharacter.Character, error) {
	return r.FindByID(context.Background(), id)
}

func (r *raceSafeGatewayChars) GetProfile(_ context.Context, id string) (character.ProfileView, error) {
	c, err := r.FindByID(context.Background(), id)
	if err != nil {
		return character.ProfileView{}, err
	}
	return character.ProfileView{Character: c}, nil
}

func (r *raceSafeGatewayChars) FindByID(_ context.Context, id string) (corecharacter.Character, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.chars[id]
	if !ok {
		return corecharacter.Character{}, corecharacter.ErrNotFound
	}
	return c, nil
}

func (r *raceSafeGatewayChars) FindByIDForUpdate(ctx context.Context, id string) (corecharacter.Character, error) {
	return r.FindByID(ctx, id)
}

func (r *raceSafeGatewayChars) Update(_ context.Context, c corecharacter.Character) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.chars[c.ID] = c
	return nil
}

func (r *raceSafeGatewayChars) FindPendingByActorID(context.Context, string) ([]scheduling.ScheduledAction, error) {
	return nil, nil
}

func (r *raceSafeGatewayChars) GetAccount(_ context.Context, id string) (casino.Account, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.accounts.GetAccount(context.Background(), id)
}

func (r *raceSafeGatewayChars) GetAccountForUpdate(ctx context.Context, id string) (casino.Account, error) {
	return r.GetAccount(ctx, id)
}

func (r *raceSafeGatewayChars) AdjustCoins(_ context.Context, id string, delta int64) (casino.Account, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.accounts.AdjustCoins(context.Background(), id, delta)
}

func (r *raceSafeGatewayChars) DeductBetAndCreditPayout(ctx context.Context, id string, bet, payout int64) (casino.Account, error) {
	return r.AdjustCoins(ctx, id, payout-bet)
}

func (r *raceSafeGatewayChars) ListPlayers(context.Context, string) ([]coreplayer.Player, error) {
	return nil, nil
}
func (r *raceSafeGatewayChars) BanPlayer(context.Context, string) error { return nil }
func (r *raceSafeGatewayChars) Register(context.Context, string, string) (coreplayer.Player, error) {
	return coreplayer.Player{}, nil
}
func (r *raceSafeGatewayChars) Login(context.Context, string, string, ...string) (coreplayer.Session, error) {
	return coreplayer.Session{}, nil
}
func (r *raceSafeGatewayChars) Logout(context.Context, string) error { return nil }
func (r *raceSafeGatewayChars) DeleteAccount(context.Context, string, string) error {
	return nil
}
func (r *raceSafeGatewayChars) CreateAPIToken(context.Context, string, string, *time.Time) (coreplayer.APIToken, string, error) {
	return coreplayer.APIToken{}, "", nil
}
func (r *raceSafeGatewayChars) ListAPITokens(context.Context, string) ([]coreplayer.APIToken, error) {
	return nil, nil
}
func (r *raceSafeGatewayChars) RevokeAPIToken(context.Context, string, string) error { return nil }

func newRaceSafeKickLeaveSetup(t *testing.T, useValkey bool, wrap func(CasinoService) CasinoService) (router http.Handler, waits timer.Service, chars *raceSafeGatewayChars, heroID, peerID, roomID string) {
	t.Helper()
	ctx := context.Background()
	suffix := id.New()[:8]
	heroID = "hero-" + suffix
	peerID = "peer-" + suffix
	roomID = "room-" + suffix

	chars = &raceSafeGatewayChars{
		accounts: &casinoSceneAccounts{},
		chars: map[string]corecharacter.Character{
			heroID: {ID: heroID, PlayerID: "owner", Name: "hero", Money: 100, Stats: corecharacter.Stats{HP: 50}},
			peerID: {ID: peerID, PlayerID: "peer-owner", Name: "peer", Money: 100, Stats: corecharacter.Stats{HP: 50}},
		},
	}

	var roomRepo casino.RoomRepository
	if useValkey {
		if os.Getenv("PARTY2_VALKEY_ADDR") == "" {
			t.Skip("PARTY2_VALKEY_ADDR is not configured")
		}
		client, err := vk.NewClientWithConfig(vk.ConfigFromEnvironment())
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(client.Close)
		t.Cleanup(func() {
			bg := context.Background()
			_ = client.Do(bg, client.B().Del().Key(casino.DefaultRoomKeyPrefix+roomID).Build())
			_ = client.Do(bg, client.B().Zrem().Key(casino.DefaultRoomsActiveIndexKey).Member(roomID).Build())
			for _, actor := range []string{heroID, peerID} {
				_ = client.Do(bg, client.B().Del().Key(casino.DefaultCharacterKeyPrefix+actor).Build())
				_ = client.Do(bg, client.B().Del().Key(timer.PrefixTimer+timer.CategoryActionWait+":"+actor).Build())
			}
		})
		roomRepo, err = casino.NewValkeyRoomRepository(client, casino.WithValkeyCharacterRepository(chars))
		if err != nil {
			t.Fatal(err)
		}
		waits = timer.NewService(client)
	} else {
		roomRepo = casino.NewMemoryRoomRepository()
		waits = timer.NewService(nil)
	}

	svc, err := casino.NewService(chars, casino.WithDepartureRepository(casino.NewMemoryDepartureRepository()), casino.WithRoomRepository(roomRepo), casino.WithCharacterRepository(chars))
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	if err := roomRepo.CreateRoom(ctx, casino.Room{
		ID: roomID, Name: "Race-" + suffix, GameType: casino.GameTypeIndian, Status: casino.RoomStatusWaiting,
		LeaderCharacterID: heroID, Rate: 10, MaxPlayers: 4, Speed: casino.SpeedNormal, AllowSpectators: true, CreatedAt: now, UpdatedAt: now,
	}, casino.RoomMember{RoomID: roomID, CharacterID: heroID, Action: "待機中", Card: 2, JoinedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := roomRepo.AddMember(ctx, casino.RoomMember{RoomID: roomID, CharacterID: peerID, Action: "待機中", Card: 5, JoinedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}

	readActivities := func(ctx context.Context, owner, actor string) ([]playercontext.Activity, error) {
		v, err := svc.GetCharacterRoomView(ctx, owner, actor)
		if err != nil || v == nil {
			return []playercontext.Activity{}, err
		}
		acc, err := svc.GetAccount(ctx, actor)
		if err != nil {
			return nil, err
		}
		c, err := v.Controls(actor, acc.Coins)
		if err != nil {
			return nil, err
		}
		a := playercontext.Activity{Kind: "casino", ID: v.Room.ID, Role: c.Role, Phase: string(v.Room.Status), Round: v.Room.Round, Actions: []string{"casino_room_leave"}, Pace: time.Duration(v.Room.Speed) * time.Second}
		if c.CanStart {
			a.Actions = append(a.Actions, "casino_room_start", "casino_room_kick")
		}
		return []playercontext.Activity{a}, nil
	}
	store := &gatewayNavigationStore{selection: playercontext.Selection{Destination: "casino"}}
	scene := playercontext.SceneDefinition{ID: "casino", Parent: "town"}
	pc := playercontext.NewService(chars, chars, waits, playercontext.WithNavigation(store, playercontext.SceneDefinition{ID: "town"}, scene), playercontext.WithActivities(readActivities), playercontext.WithActionWait(waits, playercontext.DefaultActionWait))
	svc.SetActionWait(pc)

	var casinoHandler CasinoService = svc
	if wrap != nil {
		casinoHandler = wrap(svc)
	}
	h, err := NewHandler(chars, chars, &struct{ AdventureService }{}, &struct{ ShopService }{}, WithPlayerContext(pc), WithCasino(casinoHandler))
	if err != nil {
		t.Fatal(err)
	}
	return h.Router(), waits, chars, heroID, peerID, roomID
}

func TestCasinoKickWaitSurvivesConcurrentLeaveClaimRelease(t *testing.T) {
	for _, useValkey := range []bool{false, true} {
		name := "memory"
		if useValkey {
			name = "valkey"
		}
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			leaveEntered := make(chan struct{})
			leaveProceed := make(chan struct{})
			router, waits, _, heroID, peerID, roomID := newRaceSafeKickLeaveSetup(t, useValkey, func(base CasinoService) CasinoService {
				return &pausedLeaveCasinoService{CasinoService: base, leaveEntered: leaveEntered, leaveProceed: leaveProceed}
			})
			// Match any non-leader leave target by routing peerID dynamically via target == "" check or setting target after creation.
			_ = peerID

			var leaveStatus int
			var leaveRaw map[string]json.RawMessage
			leaveDone := make(chan struct{})
			go func() {
				defer close(leaveDone)
				leaveStatus, leaveRaw = gatewayRequest(t, router, peerID, "peer-session", "application/json", `{"action":"casino_room_leave","params":{"room_id":"`+roomID+`"}}`, ctx)
			}()

			select {
			case <-leaveEntered:
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}

			// Leader kicks peer while peer holds the 18s ActionWaitClaim.
			kickStatus, kickRaw := gatewayRequest(t, router, heroID, "session", "application/json", `{"action":"casino_room_kick","params":{"room_id":"`+roomID+`","target_character_id":"`+peerID+`"}}`, ctx)
			if kickStatus != http.StatusOK {
				close(leaveProceed)
				<-leaveDone
				t.Fatalf("kick failed: %d %s", kickStatus, kickRaw)
			}

			close(leaveProceed)
			<-leaveDone
			if leaveStatus != http.StatusNotFound || actionErrorCode(t, leaveRaw) != "CASINO_MEMBER_NOT_FOUND" {
				t.Fatalf("expected failed leave after kick, got %d %s", leaveStatus, leaveRaw)
			}

			// Failed leave's claim.Release() must NOT delete the 60s kick wait.
			rem, err := waits.GetRemainingLock(ctx, timer.CategoryActionWait, peerID)
			if err != nil || rem <= 58*time.Second || rem > casino.KickActionWait {
				t.Fatalf("kick wait lost after leave claim release: rem=%v err=%v", rem, err)
			}
			status, raw := gatewayRequest(t, router, peerID, "peer-session", "application/json", `{"action":"scene_enter","params":{"destination":"town"}}`, ctx)
			if status != http.StatusConflict || actionErrorCode(t, raw) != "ACTION_WAIT" {
				t.Fatalf("peer allowed to act during kick wait: %d %s", status, raw)
			}
		})
	}
}

func TestCasinoKickAndLeaveGatewayRace(t *testing.T) {
	for _, useValkey := range []bool{false, true} {
		name := "memory"
		if useValkey {
			name = "valkey"
		}
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			router, waits, chars, heroID, peerID, roomID := newRaceSafeKickLeaveSetup(t, useValkey, nil)

			var kickStatus, leaveStatus int
			var kickRaw, leaveRaw map[string]json.RawMessage
			_, _ = testutil.RunRace2(
				func() error {
					kickStatus, kickRaw = gatewayRequest(t, router, heroID, "session", "application/json", `{"action":"casino_room_kick","params":{"room_id":"`+roomID+`","target_character_id":"`+peerID+`"}}`, ctx)
					return nil
				},
				func() error {
					leaveStatus, leaveRaw = gatewayRequest(t, router, peerID, "peer-session", "application/json", `{"action":"casino_room_leave","params":{"room_id":"`+roomID+`"}}`, ctx)
					return nil
				},
			)

			peerChar, err := chars.FindByID(ctx, peerID)
			if err != nil {
				t.Fatal(err)
			}
			rem, err := waits.GetRemainingLock(ctx, timer.CategoryActionWait, peerID)
			if err != nil {
				t.Fatal(err)
			}
			switch {
			case kickStatus == http.StatusOK && (leaveStatus == http.StatusNotFound || leaveStatus == http.StatusConflict):
				if peerChar.Tired != 0 || rem <= 58*time.Second {
					t.Fatalf("kick won but wait/tired wrong: tired=%d rem=%v kick=%s leave=%s", peerChar.Tired, rem, kickRaw, leaveRaw)
				}
				status, raw := gatewayRequest(t, router, peerID, "peer-session", "application/json", `{"action":"scene_enter","params":{"destination":"town"}}`, ctx)
				if status != http.StatusConflict || actionErrorCode(t, raw) != "ACTION_WAIT" {
					t.Fatalf("peer allowed after kick won: %d %s", status, raw)
				}
			case leaveStatus == http.StatusOK && kickStatus == http.StatusNotFound:
				if peerChar.Tired != 1 || rem <= 0 || rem > time.Duration(casino.SpeedNormal)*time.Second {
					t.Fatalf("leave won but wait/tired wrong: tired=%d rem=%v kick=%s leave=%s", peerChar.Tired, rem, kickRaw, leaveRaw)
				}
			default:
				t.Fatalf("unexpected concurrent kick/leave outcome: kick=(%d,%s) leave=(%d,%s)", kickStatus, kickRaw, leaveStatus, leaveRaw)
			}
		})
	}
}

func TestCasinoRoomRuleRejections(t *testing.T) {
	for _, err := range []error{casino.ErrHighLowCallUnavailable, casino.ErrLowNotAllowedTwo} {
		if status, d := casinoActionRejection(err); status != 400 || d.Code != "CASINO_INVALID_INPUT" {
			t.Fatalf("%v: %d %+v", err, status, d)
		}
	}
	if status, d := casinoActionRejection(casino.ErrAlreadyActed); status != 409 || d.Code != "CASINO_ALREADY_ACTED" {
		t.Fatalf("waiting entrant rejection: %d %+v", status, d)
	}
}

type casinoKickLogGateway struct {
	*multiActorGateway
	other corecharacter.Character
}

func (g *casinoKickLogGateway) Authenticate(ctx context.Context, token string) (coreplayer.Player, error) {
	if token == "other-session" {
		return coreplayer.Player{ID: g.other.PlayerID}, nil
	}
	return g.multiActorGateway.Authenticate(ctx, token)
}

func (g *casinoKickLogGateway) Get(ctx context.Context, id string) (corecharacter.Character, error) {
	if id == g.other.ID {
		return g.other, nil
	}
	return g.multiActorGateway.Get(ctx, id)
}

func (g *casinoKickLogGateway) FindByID(ctx context.Context, id string) (corecharacter.Character, error) {
	if id == g.other.ID {
		return g.other, nil
	}
	return g.multiActorGateway.FindByID(ctx, id)
}

func (g *casinoKickLogGateway) GetProfile(ctx context.Context, id string) (character.ProfileView, error) {
	c, err := g.Get(ctx, id)
	if err != nil {
		return character.ProfileView{}, err
	}
	return character.ProfileView{Character: c}, nil
}

func (g *casinoKickLogGateway) FindPendingByActorID(ctx context.Context, id string) ([]scheduling.ScheduledAction, error) {
	if id == g.other.ID {
		return nil, nil
	}
	return g.multiActorGateway.FindPendingByActorID(ctx, id)
}

func TestCasinoKickPlaceLog(t *testing.T) {
	ctx := context.Background()
	setup := func(t *testing.T) (*casinoSceneFixture, *faultyKickWaitService, *fakePlaceLog, http.Handler, string) {
		t.Helper()
		f := newCasinoSceneFixture(t)
		f.char.Name = "Hero"
		departures := casino.NewMemoryDepartureRepository()
		peer := corecharacter.Character{ID: "peer", PlayerID: "peer-owner", Name: "Peer", Money: 100, Stats: corecharacter.Stats{HP: 50}}
		peer2 := corecharacter.Character{ID: "peer2", PlayerID: "peer-owner", Name: "Peer2", Money: 100, Stats: corecharacter.Stats{HP: 50}}
		other := corecharacter.Character{ID: "other", PlayerID: "peer-owner", Name: "Other", Money: 100, Stats: corecharacter.Stats{HP: 50}}
		chars := &multiActorCasinoChars{casinoSceneCharacters: f.characters, peer: peer, peer2: peer2}
		var err error
		f.service, err = casino.NewService(f.accounts, casino.WithDepartureRepository(departures), casino.WithRoomRepository(f.rooms), casino.WithCharacterRepository(chars))
		if err != nil {
			t.Fatal(err)
		}
		waits := &faultyKickWaitService{Service: timer.NewService(nil), target: "peer"}
		gw := &casinoKickLogGateway{multiActorGateway: &multiActorGateway{gatewayFixture: f.gatewayFixture, peer: peer}, other: other}
		scene := playercontext.SceneDefinition{ID: "casino", Parent: "town", Pageable: true, SubjectKind: "room", SubjectAvailable: func(ctx context.Context, _, id string) (bool, error) {
			rows, err := f.service.ListRooms(ctx)
			return slices.ContainsFunc(rows, func(r casino.RoomSummary) bool { return r.ID == id }), err
		}}
		pc := playercontext.NewService(gw, gw, waits, playercontext.WithNavigation(f.store, playercontext.SceneDefinition{ID: "town"}, scene), playercontext.WithActivities(f.readActivities), playercontext.WithActionWait(waits, playercontext.DefaultActionWait))
		f.service.SetActionWait(pc)
		placeLog := &fakePlaceLog{byPlace: map[placelog.Place][]placelog.Entry{}}
		h, err := NewHandler(gw, gw, &struct{ AdventureService }{}, &struct{ ShopService }{}, WithPlayerContext(pc), WithCasino(f.service), WithPlaceLog(placeLog))
		if err != nil {
			t.Fatal(err)
		}
		room, err := f.service.CreateRoom(ctx, "hero", casino.CreateRoomRequest{
			Name: "CasinoHall", GameType: casino.GameTypeIndian, Speed: casino.SpeedNormal, MaxPlayers: 4, Rate: 10,
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.service.JoinRoom(ctx, room.Room.ID, "peer", "", 0); err != nil {
			t.Fatal(err)
		}
		return f, waits, placeLog, h.Router(), room.Room.ID
	}

	casinoPlace := placelog.Place{Kind: placelog.KindFacility, ID: "casino"}

	t.Run("posts private notice to casino lobby only and ignores append failures", func(t *testing.T) {
		f, waits, placeLog, router, roomID := setup(t)
		roomPlace := placelog.Place{Kind: placeKindCasinoRoom, ID: roomID}

		// Rejected self-kick writes no log row.
		if status, _ := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"casino_room_kick","params":{"room_id":"`+roomID+`","target_character_id":"hero"}}`, nil); status != http.StatusForbidden {
			t.Fatalf("expected 403 self-kick, got %d", status)
		}
		if len(placeLog.byPlace) != 0 {
			t.Fatalf("rejected kick wrote log %+v", placeLog.byPlace)
		}

		// Valid kick appends private NPC notice to the casino lobby place log only.
		status, raw := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"casino_room_kick","params":{"room_id":"`+roomID+`","target_character_id":"peer"}}`, nil)
		if status != http.StatusOK {
			t.Fatalf("kick failed: %d %s", status, raw)
		}
		peerStatus, peerCtx := contextGET(t, router, "peer", "peer-session")
		if peerStatus != http.StatusOK || len(peerCtx.Scene.PlaceLog) != 1 ||
			!peerCtx.Scene.PlaceLog[0].Private ||
			peerCtx.Scene.PlaceLog[0].SpeakerKind != string(placelog.SpeakerNPC) ||
			peerCtx.Scene.PlaceLog[0].Name != "＠" ||
			peerCtx.Scene.PlaceLog[0].Color != npcColor ||
			peerCtx.Scene.PlaceLog[0].Text != "CasinoHallのリーダーHeroからきっくされました" {
			t.Fatalf("kicked peer place_log = (%d) %+v", peerStatus, peerCtx.Scene.PlaceLog)
		}
		if otherStatus, otherCtx := contextGET(t, router, "other", "other-session"); otherStatus != http.StatusOK || len(otherCtx.Scene.PlaceLog) != 0 {
			t.Fatalf("kick notice leaked to third-party viewer: (%d) %+v", otherStatus, otherCtx.Scene.PlaceLog)
		}
		if heroStatus, heroCtx := contextGET(t, router, "hero", "session"); heroStatus != http.StatusOK || len(heroCtx.Scene.PlaceLog) != 0 {
			t.Fatalf("kick notice leaked into room place_log: (%d) %+v", heroStatus, heroCtx.Scene.PlaceLog)
		}
		if len(placeLog.byPlace[roomPlace]) != 0 || len(placeLog.byPlace[casinoPlace]) != 1 {
			t.Fatalf("unexpected placeLog buckets: %+v", placeLog.byPlace)
		}

		// Acknowledged lost-response retry does not duplicate the log row.
		if err := waits.ReleaseLock(ctx, timer.CategoryActionWait, "hero"); err != nil {
			t.Fatal(err)
		}
		if status, raw := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"casino_room_kick","params":{"room_id":"`+roomID+`","target_character_id":"peer"}}`, nil); status != http.StatusOK {
			t.Fatalf("idempotent retry failed: %d %s", status, raw)
		}
		if len(placeLog.byPlace[casinoPlace]) != 1 {
			t.Fatalf("idempotent retry duplicated notice: %+v", placeLog.byPlace[casinoPlace])
		}

		// Best-effort append failure does not fail a committed kick.
		if _, err := f.service.JoinRoom(ctx, roomID, "peer2", "", 0); err != nil {
			t.Fatal(err)
		}
		if err := waits.ReleaseLock(ctx, timer.CategoryActionWait, "hero"); err != nil {
			t.Fatal(err)
		}
		placeLog.err = errors.New("valkey log write failed")
		if status, raw := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"casino_room_kick","params":{"room_id":"`+roomID+`","target_character_id":"peer2"}}`, nil); status != http.StatusOK {
			t.Fatalf("kick should succeed despite log append failure: %d %s", status, raw)
		}
	})

	t.Run("failed wait write writes no log row", func(t *testing.T) {
		_, waits, placeLog, router, roomID := setup(t)
		waits.failExtend.Store(true)
		if status, _ := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"casino_room_kick","params":{"room_id":"`+roomID+`","target_character_id":"peer"}}`, nil); status != http.StatusInternalServerError {
			t.Fatalf("expected 500 on wait failure, got %d", status)
		}
		if len(placeLog.byPlace) != 0 {
			t.Fatalf("failed kick wrote log: %+v", placeLog.byPlace)
		}
	})
}
