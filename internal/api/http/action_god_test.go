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
	"github.com/witchcraze/party2re/internal/god"
	"github.com/witchcraze/party2re/internal/playercontext"
)

type godGatewayFixture struct {
	*gatewayFixture
	wishes   []god.Wish
	wishErr  error
	realm    god.Realm
	wishID   string
	grant    god.WishResult
	forfeits int
}

func (f *godGatewayFixture) GetWishes(_ context.Context, actor string, realm god.Realm) ([]god.Wish, error) {
	if actor != "hero" {
		f.t.Fatal("wrong wish observer")
	}
	f.realm = realm
	return f.wishes, f.wishErr
}

func (f *godGatewayFixture) GrantWish(ctx context.Context, actor, wishID string) (god.WishResult, error) {
	f.wishID = wishID
	_, err := f.execute(ctx, actor, 0)
	return f.grant, err
}

func (f *godGatewayFixture) router(t *testing.T, options ...Option) (*gatewayNavigationStore, http.Handler) {
	t.Helper()
	store := &gatewayNavigationStore{selection: playercontext.Selection{Destination: "town"}}
	visit := func(realm string) func(corecharacter.Character) bool {
		return func(c corecharacter.Character) bool { return c.GodVisit == realm }
	}
	pc := playercontext.NewService(f.gatewayFixture, f.gatewayFixture, timer.NewService(nil), playercontext.WithNavigation(store,
		playercontext.SceneDefinition{ID: "town", Pageable: true}, playercontext.SceneDefinition{ID: "bank", Parent: "town"},
		playercontext.SceneDefinition{ID: "god", Parent: "town", CanEnter: visit(corecharacter.GodVisitHeaven)},
		playercontext.SceneDefinition{ID: "u_god", Parent: "town", CanEnter: visit(corecharacter.GodVisitUnderworld)}),
		playercontext.WithGodVisitForfeit(func(context.Context, string) error {
			f.forfeits++
			f.char.GodVisit = ""
			return nil
		}))
	opts := append([]Option{WithGod(f), WithPlayerContext(pc), WithPlaceLog(&fakePlaceLog{}), func(h *Handler) { h.homes = f.gatewayFixture }}, options...)
	h, err := NewHandler(f.gatewayFixture, f.gatewayFixture, &struct{ AdventureService }{}, &struct{ ShopService }{}, opts...)
	if err != nil {
		t.Fatal(err)
	}
	return store, h.Router()
}

func newGodFixture(t *testing.T, visit string) *godGatewayFixture {
	f := &godGatewayFixture{gatewayFixture: newGatewayFixture(t)}
	f.char.Name, f.char.GodVisit = "Hero", visit
	return f
}

func TestGodSceneFollowsTheRecordedVisit(t *testing.T) {
	for _, tc := range []struct{ visit, scene, title, npc string }{
		{corecharacter.GodVisitHeaven, "god", "天界", "@神"},
		{corecharacter.GodVisitUnderworld, "u_god", "天界?", "@神?"},
	} {
		t.Run(tc.scene, func(t *testing.T) {
			f := newGodFixture(t, tc.visit)
			f.wishes = []god.Wish{{ID: god.WishMoney, Name: "お金がほしい", Available: true}}
			store, router := f.router(t)
			status, observation := navigationGET(t, router)
			data := decodeShopScene[GodSceneData](t, observation)
			if status != 200 || observation.Scene.LocationID != tc.scene || observation.Scene.Title != tc.title || observation.Scene.Support.Observation != "details" ||
				data.Realm != god.Realm(tc.visit) || f.realm != god.Realm(tc.visit) || len(data.Wishes) != 1 || data.Parent != "town" {
				t.Fatalf("projection: %d %+v", status, observation)
			}
			for _, action := range []string{"god_wish", "npc_talk", "npc_inspect", "place_say", "scene_back"} {
				if !slices.ContainsFunc(observation.AvailableActions, func(a ContextAction) bool { return a.Action == action }) {
					t.Fatalf("missing %s: %+v", action, observation.AvailableActions)
				}
			}
			if !slices.Contains(observation.Scene.Members, PlaceMember{Kind: "npc", Name: tc.npc, Color: npcColor}) || store.writes != 0 || f.executions != 0 {
				t.Fatalf("members %+v writes %d executions %d", observation.Scene.Members, store.writes, f.executions)
			}
		})
	}
	// Without a visit the saved selection applies and god_wish is not offered.
	f := newGodFixture(t, "")
	_, router := f.router(t)
	if status, observation := navigationGET(t, router); status != 200 || observation.Scene.LocationID != "town" ||
		slices.ContainsFunc(observation.AvailableActions, func(a ContextAction) bool { return a.Action == "god_wish" }) ||
		slices.ContainsFunc(decodeShopScene[TownSceneData](t, observation).Destinations, func(d SceneDestination) bool { return strings.Contains(d.ID, "god") }) {
		t.Fatalf("no visit: %d %+v", status, observation)
	}
}

func TestGodSceneRequiredReadFailure(t *testing.T) {
	f := newGodFixture(t, corecharacter.GodVisitHeaven)
	f.wishErr = errors.New("private guild failure")
	_, router := f.router(t)
	if status, observation := navigationGET(t, router); status != 500 || observation.Scene.Data != nil {
		t.Fatalf("partial observation: %d %+v", status, observation)
	}
}

func TestGodWishCommand(t *testing.T) {
	for _, params := range []string{``, `{}`, `{"wish_id":null}`, `{"wish_id":1}`, `{"wish_id":"x","realm":"heaven"}`, `{"wish_id":"x","character_id":"other"}`} {
		f := newGodFixture(t, corecharacter.GodVisitHeaven)
		_, router := f.router(t)
		if status, raw := gatewayRequest(t, router, "hero", "session", "application/json", saleGatewayBody("god_wish", params), nil); status != 400 || f.executions != 0 {
			t.Fatalf("%s executed: %d %s", params, status, raw)
		}
	}
	for _, tc := range []struct {
		visit, wish, speech string
	}{
		{corecharacter.GodVisitHeaven, god.WishMoney, "ふむ。Heroの願いは「お金がほしい」だな。\nHeroの願いを叶えたぞ…。機会があればまたあえるだろう…。さらばだ…"},
		{corecharacter.GodVisitUnderworld, god.WishExpandDepot, "ふむ。Heroの願いは「お金がほしい」だな。\nHeroの願いを叶えたぞ…。機会があればまたあえるだろう…。さらばだ…"},
		{corecharacter.GodVisitHeaven, god.WishLover, "それは無理な願いだ…。アドバイスとしては積極的にアピールするのだ…"},
	} {
		t.Run(tc.visit+tc.wish, func(t *testing.T) {
			f := newGodFixture(t, tc.visit)
			f.grant = god.WishResult{Character: corecharacter.Character{ID: "hero", Name: "Hero"}, Wish: god.Wish{ID: tc.wish, Name: "お金がほしい"}, Message: "granted"}
			_, router := f.router(t)
			status, raw := gatewayRequest(t, router, "hero", "session", "application/json", saleGatewayBody("god_wish", fmt.Sprintf(`{"wish_id":%q}`, tc.wish)), nil)
			var result struct {
				Message   string   `json:"message"`
				NPCSpeech string   `json:"npc_speech"`
				Wish      god.Wish `json:"wish"`
			}
			if err := json.Unmarshal(raw["result"], &result); err != nil || status != 200 || f.wishID != tc.wish || result.NPCSpeech != tc.speech || result.Message != "granted" || result.Wish.ID != tc.wish {
				t.Fatalf("%d %s %v", status, raw, err)
			}
			assertGatewayContext(t, raw)
		})
	}
}

func TestGodWishRejections(t *testing.T) {
	for _, tc := range []struct {
		err    error
		code   string
		status int
	}{
		{god.ErrNotAdmitted, "GOD_NOT_ADMITTED", 409},
		{god.ErrWishNotFound, "GOD_WISH_NOT_FOUND", 400},
		{god.ErrLimitBreakMaxed, "GOD_LIMIT_BREAK_MAXED", 409},
		{god.ErrWishRequirement, "GOD_WISH_REQUIREMENT", 409},
		{god.ErrCharacterNotFound, "CHARACTER_NOT_FOUND", 404},
		{errors.New("private storage failure"), "EXECUTION_FAILED", 500},
	} {
		t.Run(tc.code, func(t *testing.T) {
			f := newGodFixture(t, corecharacter.GodVisitHeaven)
			f.executionErr = fmt.Errorf("private wrapped: %w", tc.err)
			_, router := f.router(t)
			status, raw := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"god_wish","params":{"wish_id":"wish_money"}}`, nil)
			assertGatewayError(t, raw, tc.code)
			if status != tc.status || strings.Contains(fmt.Sprint(raw), "private") {
				t.Fatalf("%d %s", status, raw)
			}
		})
	}
}

func TestLeavingGodThroughNavigationForfeitsTheVisit(t *testing.T) {
	f := newGodFixture(t, corecharacter.GodVisitHeaven)
	store, router := f.router(t)
	store.selection.Destination = "bank" // the saved selection is irrelevant while visiting
	status, raw := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"scene_back","params":{}}`, nil)
	if status != 200 || f.forfeits != 1 || store.selection.Destination != "town" {
		t.Fatalf("leave: %d %s forfeits=%d", status, raw, f.forfeits)
	}
	if _, observation := navigationGET(t, router); observation.Scene.LocationID != "town" {
		t.Fatalf("still in god: %+v", observation.Scene)
	}
}

func TestGodNPCReplies(t *testing.T) {
	f := newGodFixture(t, corecharacter.GodVisitHeaven)
	_, router := f.router(t)
	_, raw := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"npc_talk","params":{}}`, nil)
	if r := npcResult(t, raw); r != (NPCResult{NPCName: "@神", Dialogue: "Heroよ。よくぞここまできた。Heroの願いを一つだけ叶えてやろう"}) {
		t.Fatalf("talk %+v", r)
	}
	_, raw = gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"npc_inspect","params":{}}`, nil)
	if r := npcResult(t, raw); r.Dialogue != "@神「本当の願いは自分の力で叶えるのだ…」" {
		t.Fatalf("heaven inspect %+v", r)
	}
	f = newGodFixture(t, corecharacter.GodVisitUnderworld)
	_, router = f.router(t)
	_, raw = gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"npc_inspect","params":{}}`, nil)
	if r := npcResult(t, raw); r != (NPCResult{NPCName: "@神?", Private: true}) {
		t.Fatalf("underworld inspect must print nothing: %+v", r)
	}
}
