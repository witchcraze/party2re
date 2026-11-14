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

	"github.com/witchcraze/party2re/internal/altar"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/playercontext"
	"github.com/witchcraze/party2re/internal/wishingwell"
)

var faithInputs = []struct{ scene, action, params string }{
	{"altar", "altar_pray", `{}`},
	{"altar", "altar_wish", `{"item_id":"item-066"}`},
	{"wishingwell", "wishingwell_exchange", `{"stat":"mhp","sp":3}`},
}

type faithGatewayFixture struct {
	*gatewayFixture
	status    altar.AltarStatus
	statusErr error
	reads     int
	inputs    []any
}

func (f *faithGatewayFixture) GetStatus(_ context.Context, actor string) (altar.AltarStatus, error) {
	if actor != "hero" {
		f.t.Fatal("wrong altar observer")
	}
	f.reads++
	return f.status, f.statusErr
}

func (f *faithGatewayFixture) Pray(ctx context.Context, actor string) (altar.PrayResult, error) {
	f.inputs = []any{}
	_, err := f.execute(ctx, actor, 0)
	return altar.PrayResult{CharacterID: actor, Message: altar.MsgRamiaAwakening, RamiaAwakened: true}, err
}

func (f *faithGatewayFixture) Wish(ctx context.Context, actor, itemID string) (altar.WishResult, error) {
	f.inputs = []any{itemID}
	_, err := f.execute(ctx, actor, 0)
	return altar.WishResult{CharacterID: actor, ItemID: itemID, DeliveredTo: "depot"}, err
}

func (f *faithGatewayFixture) Exchange(ctx context.Context, req wishingwell.ExchangeRequest) (wishingwell.ExchangeResult, error) {
	f.inputs = []any{req.CharacterID, req.Stat, req.SP}
	_, err := f.execute(ctx, req.CharacterID, 0)
	return wishingwell.ExchangeResult{CharacterID: req.CharacterID, StatIncrease: 6}, err
}

func (f *faithGatewayFixture) router(t *testing.T, destination string, options ...Option) (*gatewayNavigationStore, http.Handler) {
	t.Helper()
	store := &gatewayNavigationStore{selection: playercontext.Selection{Destination: destination}}
	pc := playercontext.NewService(f.gatewayFixture, f.gatewayFixture, timer.NewService(nil), playercontext.WithNavigation(store,
		playercontext.SceneDefinition{ID: "town", Pageable: true}, playercontext.SceneDefinition{ID: "bank", Parent: "town"},
		playercontext.SceneDefinition{ID: "altar", Parent: "town"}, playercontext.SceneDefinition{ID: "wishingwell", Parent: "town"}))
	opts := append([]Option{WithAltar(f), WithWishingWell(f), WithPlayerContext(pc), func(h *Handler) { h.homes = f.gatewayFixture }}, options...)
	h, err := NewHandler(f.gatewayFixture, f.gatewayFixture, &struct{ AdventureService }{}, &struct{ ShopService }{}, opts...)
	if err != nil {
		t.Fatal(err)
	}
	return store, h.Router()
}

func newFaithFixture(t *testing.T) *faithGatewayFixture {
	return &faithGatewayFixture{gatewayFixture: newGatewayFixture(t), status: altar.AltarStatus{CharacterID: "hero", AvailableItems: []altar.WishItemInfo{}}}
}

func TestFaithGatewayInputs(t *testing.T) {
	invalid := map[string][]string{
		"altar_pray":           {`null`, `[]`, `{"extra":1}`, `{"character_id":"other"}`},
		"altar_wish":           {``, `{}`, `{"item_id":null}`, `{"item_id":1}`, `{"item_id":"x","extra":1}`, `{"item_id":"x","player_id":"other"}`},
		"wishingwell_exchange": {``, `{}`, `{"stat":"mhp"}`, `{"sp":1}`, `{"stat":"mhp","sp":"1"}`, `{"stat":null,"sp":1}`, `{"stat":"mhp","sp":1,"extra":1}`},
	}
	for _, input := range faithInputs {
		for _, params := range invalid[input.action] {
			t.Run(input.action+params, func(t *testing.T) {
				f := newFaithFixture(t)
				_, router := f.router(t, input.scene)
				status, got := gatewayRequest(t, router, "hero", "session", "application/json", saleGatewayBody(input.action, params), nil)
				if status != 400 || f.executions != 0 {
					t.Fatalf("invalid input executed: %d %s", status, got)
				}
				assertGatewayError(t, got, "INVALID_ACTION_PARAMS")
			})
		}
	}
}

func TestFaithGatewayForwardsExplicitInputs(t *testing.T) {
	want := map[string][]any{"altar_pray": {}, "altar_wish": {"item-066"}, "wishingwell_exchange": {"hero", "mhp", 3}}
	for _, input := range faithInputs {
		for _, refreshFailure := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/refresh=%t", input.action, refreshFailure), func(t *testing.T) {
				f := newFaithFixture(t)
				if refreshFailure {
					f.afterExecute = func() { f.profileErr = errors.New("private profile failure") }
				}
				_, router := f.router(t, input.scene)
				status, got := gatewayRequest(t, router, "hero", "session", "application/json", saleGatewayBody(input.action, input.params), nil)
				if status != 200 || f.executions != 1 || string(got["success"]) != "true" || fmt.Sprint(f.inputs) != fmt.Sprint(want[input.action]) {
					t.Fatalf("lost outcome or inputs: %d %s %v", status, got, f.inputs)
				}
				if refreshFailure {
					assertLoopRefreshFailure(t, got)
				} else {
					assertGatewayContext(t, got)
				}
				resolved := NewActionURLResolver().Resolve("hero", input.action, "", "")
				if resolved.Method != "POST" || resolved.URL != "/api/v1/characters/hero/actions" {
					t.Fatalf("obsolete resolver: %+v", resolved)
				}
			})
		}
	}
}

func TestFaithGatewayErrors(t *testing.T) {
	for _, tc := range []struct {
		action string
		err    error
		code   string
		status int
	}{
		{"altar_pray", altar.ErrInsufficientOrbs, "ALTAR_INSUFFICIENT_ORBS", 400},
		{"altar_pray", altar.ErrRamiaAwakened, "ALTAR_RAMIA_AWAKENED", 409},
		{"altar_wish", altar.ErrRamiaNotAwakened, "ALTAR_RAMIA_NOT_AWAKENED", 409},
		{"altar_wish", altar.ErrInvalidWishItem, "ALTAR_INVALID_WISH_ITEM", 400},
		{"altar_wish", corecharacter.ErrNotFound, "CHARACTER_NOT_FOUND", 404},
		{"altar_pray", errors.New("private storage failure"), "EXECUTION_FAILED", 500},
		{"wishingwell_exchange", wishingwell.ErrInvalidTargetStat, "WISHINGWELL_INVALID_STAT", 400},
		{"wishingwell_exchange", wishingwell.ErrInvalidSPAmount, "WISHINGWELL_INVALID_SP", 400},
		{"wishingwell_exchange", wishingwell.ErrInsufficientSP, "WISHINGWELL_INSUFFICIENT_SP", 400},
		{"wishingwell_exchange", wishingwell.ErrJobMemoryActive, "WISHINGWELL_JOB_MEMORY", 409},
		{"wishingwell_exchange", wishingwell.ErrOverLevelRestricted, "WISHINGWELL_OVER_LEVEL", 409},
		{"wishingwell_exchange", corecharacter.ErrNotFound, "CHARACTER_NOT_FOUND", 404},
	} {
		t.Run(tc.action+tc.code, func(t *testing.T) {
			input := faithInputs[slices.IndexFunc(faithInputs, func(i struct{ scene, action, params string }) bool { return i.action == tc.action })]
			f := newFaithFixture(t)
			f.executionErr = fmt.Errorf("private wrapped: %w", tc.err)
			_, router := f.router(t, input.scene)
			status, got := gatewayRequest(t, router, "hero", "session", "application/json", saleGatewayBody(tc.action, input.params), nil)
			assertGatewayError(t, got, tc.code)
			if status != tc.status || f.executions != 1 || strings.Contains(fmt.Sprint(got), "private") {
				t.Fatalf("error mapping: %d %s", status, got)
			}
		})
	}
}

func TestFaithGatewayGuards(t *testing.T) {
	for _, input := range faithInputs {
		for _, condition := range []string{"anonymous", "foreign", "location", "town", "wake"} {
			t.Run(input.action+condition, func(t *testing.T) {
				f := newFaithFixture(t)
				path, token, scene, want := "hero", "session", input.scene, 409
				switch condition {
				case "anonymous":
					token, want = "", 401
				case "foreign":
					path, want = "other", 403
				case "location":
					scene = "bank"
				case "town":
					scene = "town"
				case "wake":
					f.char.PendingWake = true
				}
				_, router := f.router(t, scene)
				if status, got := gatewayRequest(t, router, path, token, "application/json", saleGatewayBody(input.action, input.params), nil); status != want || f.executions != 0 {
					t.Fatalf("guard bypass: %d %s", status, got)
				}
			})
		}
	}
}

func TestFaithScenesObserveWithoutWrites(t *testing.T) {
	f := newFaithFixture(t)
	f.status = altar.AltarStatus{CharacterID: "hero", Orbs: "G", RamiaAwakened: true, RamiaPresent: true, AwakenedBy: "Friend",
		AvailableItems: []altar.WishItemInfo{{ID: "item-066", Name: "真実の鏡"}}}
	store, router := f.router(t, "altar", WithPlaceLog(&fakePlaceLog{}))
	status, observation := navigationGET(t, router)
	data := decodeShopScene[AltarSceneData](t, observation)
	if status != 200 || observation.Scene.Title != "復活の祭壇" || observation.Scene.Support.Observation != "details" ||
		data.Parent != "town" || !data.RamiaAwakened || data.AwakenedBy != "Friend" || len(data.AvailableItems) != 1 {
		t.Fatalf("altar projection: %d %+v", status, observation)
	}
	if want := []PlaceMember{{Kind: "npc", Name: altar.NPCName, Color: npcColor}, {Kind: "npc", Name: altar.RamiaMemberName, Color: npcColor}}; !slices.Equal(observation.Scene.Members, want) {
		t.Fatalf("members %+v, want the shrine maiden and Ramia", observation.Scene.Members)
	}
	for _, action := range []string{"altar_pray", "altar_wish", "npc_talk", "npc_inspect", "place_say", "scene_back"} {
		if !slices.ContainsFunc(observation.AvailableActions, func(a ContextAction) bool { return a.Action == action }) {
			t.Fatalf("missing connected choice %s: %+v", action, observation.AvailableActions)
		}
	}

	store.selection.Destination = "wishingwell"
	f.char.OverLevel = true
	status, observation = navigationGET(t, router)
	well := decodeShopScene[WishingWellSceneData](t, observation)
	if status != 200 || observation.Scene.Title != "願いの泉" || well.Parent != "town" || !well.OverLevel || well.CanExchange ||
		!slices.ContainsFunc(observation.AvailableActions, func(a ContextAction) bool { return a.Action == "wishingwell_exchange" }) {
		t.Fatalf("wishing well projection: %d %+v", status, observation)
	}
	if store.writes != 0 || f.executions != 0 {
		t.Fatalf("GET wrote: %d %d", store.writes, f.executions)
	}
}

func TestAltarSceneRequiredReadFailure(t *testing.T) {
	f := newFaithFixture(t)
	f.statusErr = errors.New("private awakening read failure")
	store, router := f.router(t, "altar")
	if status, observation := navigationGET(t, router); status != 500 || observation.Scene.Data != nil || store.writes != 0 {
		t.Fatalf("partial observation: %d %+v", status, observation)
	}
}

func TestFaithNPCWordsFollowTheSpeaker(t *testing.T) {
	for _, tc := range []struct {
		scene, orb string
		words      []string
	}{
		{"altar", "", mikoWords(corecharacter.Character{})},
		{"altar", "srbgyp", mikoWords(corecharacter.Character{Orb: "srbgyp"})},
		{"altar", "G", mikoWords(corecharacter.Character{Orb: "G"})},
		{"wishingwell", "", nil},
	} {
		t.Run(tc.scene+tc.orb, func(t *testing.T) {
			f := newFaithFixture(t)
			f.char.Name, f.char.Orb, f.char.SP = "Hero", tc.orb, 42
			_, router := f.router(t, tc.scene)
			for range 20 {
				status, raw := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"npc_talk","params":{}}`, nil)
				r := npcResult(t, raw)
				if status != 200 || strings.ContainsAny(r.Dialogue, "{}") {
					t.Fatalf("npc_talk: %d %+v", status, r)
				}
				if tc.words != nil && (r.NPCName != altar.NPCName || !slices.Contains(tc.words, r.Dialogue)) {
					t.Fatalf("line %q not in the %q words", r.Dialogue, tc.orb)
				}
				if tc.words == nil && r.NPCName != wishingwell.NPCName {
					t.Fatalf("speaker %+v", r)
				}
				if strings.Contains(r.Dialogue, "スキルポイントは現在") && r.Dialogue != "Heroのスキルポイントは現在 42 ポイントです" {
					t.Fatalf("SP line %q", r.Dialogue)
				}
			}
		})
	}
}

// The detail lists must stay non-null so clients need no absent-array branch.
func TestAltarSceneDataKeepsEmptyWishItems(t *testing.T) {
	raw, err := json.Marshal(AltarSceneData{Parent: "town", AltarStatus: altar.AltarStatus{AvailableItems: []altar.WishItemInfo{}}})
	if err != nil || !strings.Contains(string(raw), `"available_items":[]`) {
		t.Fatalf("%s %v", raw, err)
	}
}

func TestMikoWordsFollowOrbState(t *testing.T) {
	for _, tc := range []struct {
		orb  string
		want []string
	}{{"", mikoGuardingWords}, {"srbgyp", mikoReadyWords}, {"G", mikoAwakenedWords}} {
		if got := mikoWords(corecharacter.Character{Orb: tc.orb}); !slices.Equal(got, tc.want) {
			t.Errorf("orb %q: got %q", tc.orb, got)
		}
	}
}
