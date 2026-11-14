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
	"github.com/witchcraze/party2re/internal/depot"
	"github.com/witchcraze/party2re/internal/playercontext"
	"github.com/witchcraze/party2re/internal/store"
)

type oracleFixture struct {
	*gatewayFixture
	status      store.OracleStatus
	statusErr   error
	deliveredTo string
}

func (f *oracleFixture) GetOracleStatus(context.Context, string) (*store.OracleStatus, error) {
	if f.statusErr != nil {
		return nil, f.statusErr
	}
	return &f.status, nil
}

func (f *oracleFixture) BuyCostumeItem(ctx context.Context, actor string, itemNo int) (*store.CostumeBuyResult, error) {
	_, err := f.execute(ctx, actor, 0)
	return &store.CostumeBuyResult{ItemNo: itemNo, ItemName: "ネコミミメイド", Price: 300, DeliveredTo: f.deliveredTo}, err
}

func (f *oracleFixture) BuyHomeWallpaper(ctx context.Context, actor, wallpaper string) (*store.HomeWallpaperResult, error) {
	_, err := f.execute(ctx, actor, 0)
	return &store.HomeWallpaperResult{Wallpaper: wallpaper + ".gif"}, err
}

func (f *oracleFixture) router(t *testing.T, destination string, options ...Option) (*gatewayNavigationStore, http.Handler) {
	t.Helper()
	store := &gatewayNavigationStore{selection: playercontext.Selection{Destination: destination}}
	pc := playercontext.NewService(f.gatewayFixture, f.gatewayFixture, timer.NewService(nil), playercontext.WithNavigation(store,
		playercontext.SceneDefinition{ID: "town", Pageable: true}, playercontext.SceneDefinition{ID: "bank", Parent: "town"},
		playercontext.SceneDefinition{ID: "oracle", Parent: "town"},
		playercontext.SceneDefinition{ID: "blackmarket", Parent: "oracle", ParentOnly: true, CanEnter: func(c corecharacter.Character) bool { return c.JobLevel >= 15 }}))
	opts := append([]Option{WithOracleShop(f), WithPlayerContext(pc), func(h *Handler) { h.homes = f.gatewayFixture }}, options...)
	h, err := NewHandler(f.gatewayFixture, f.gatewayFixture, &struct{ AdventureService }{}, &struct{ ShopService }{}, opts...)
	if err != nil {
		t.Fatal(err)
	}
	return store, h.Router()
}

func newOracleFixture(t *testing.T) *oracleFixture {
	f := &oracleFixture{gatewayFixture: newGatewayFixture(t), deliveredTo: string(depot.DeliveredToInventory),
		status: store.OracleStatus{CharacterID: "hero", AvailableCostumes: store.AvailableCostumes(3), AvailableWallpapers: []store.WallpaperInfo{{Name: "goods", Price: 1000}}}}
	f.char.Name = "勇者"
	return f
}

func TestOracleSceneObservesCatalogAndGatesTheBlackMarket(t *testing.T) {
	f := newOracleFixture(t)
	store, router := f.router(t, "oracle", WithPlaceLog(&fakePlaceLog{}))
	status, observation := navigationGET(t, router)
	data := decodeShopScene[OracleSceneData](t, observation)
	if status != 200 || observation.Scene.Title != "オラクル屋" || observation.Scene.Support.Observation != "details" ||
		data.Parent != "town" || len(data.AvailableCostumes) != 5 || data.ActiveCostume != nil {
		t.Fatalf("oracle: %d %+v", status, data)
	}
	for _, action := range []string{"oracle_buy", "oracle_wallpaper", "npc_talk", "npc_inspect", "place_say", "scene_back"} {
		if !offered(observation, action) {
			t.Fatalf("missing choice %s", action)
		}
	}
	if offered(observation, "oracle_blackmarket") {
		t.Fatal("black market offered below job level 15")
	}
	f.char.JobLevel = 15
	if _, observation = navigationGET(t, router); !offered(observation, "oracle_blackmarket") {
		t.Fatal("black market hidden from a qualified character")
	}
	f.statusErr = errors.New("private costume read")
	if status, observation := navigationGET(t, router); status != 500 || observation.Scene.Data != nil {
		t.Fatalf("partial observation: %d", status)
	}
	if store.writes != 0 || f.executions != 0 {
		t.Fatalf("GET wrote: %d %d", store.writes, f.executions)
	}
}

func TestOracleTownListOmitsTheBlackMarket(t *testing.T) {
	f := newOracleFixture(t)
	f.char.JobLevel = 30
	_, router := f.router(t, "town")
	_, observation := navigationGET(t, router)
	data := decodeShopScene[TownSceneData](t, observation)
	for _, d := range data.Destinations {
		if d.ID == "blackmarket" {
			t.Fatal("the black market is listed in town; legacy reaches it only through ＠やみいちば")
		}
	}
}

func TestOraclePurchaseReplies(t *testing.T) {
	for _, tc := range []struct{ action, params, deliveredTo, want string }{
		{"oracle_buy", `{"item_no":47}`, string(depot.DeliveredToInventory), "ネコミミメイドだな。ほい、どうぞ"},
		{"oracle_buy", `{"item_no":47}`, string(depot.DeliveredToDepot), "ネコミミメイドは勇者の預かり所に送っておいたよん"},
		{"oracle_wallpaper", `{"wallpaper":"goods"}`, "", "勇者の家の壁紙を goods に、張り替えておいたよん"},
	} {
		t.Run(tc.want, func(t *testing.T) {
			f := newOracleFixture(t)
			f.deliveredTo = tc.deliveredTo
			_, router := f.router(t, "oracle")
			status, raw := gatewayRequest(t, router, "hero", "session", "application/json", saleGatewayBody(tc.action, tc.params), nil)
			got := tavernResult[struct{ Message string }](t, raw)
			if status != 200 || got.Message != tc.want {
				t.Fatalf("%s: %d %q", tc.action, status, got.Message)
			}
		})
	}
}

func TestOracleCommandErrors(t *testing.T) {
	for _, tc := range []struct {
		action, params string
		err            error
		code, text     string
		status         int
	}{
		{"oracle_buy", `{"item_no":47}`, store.ErrInsufficientFunds, "ORACLE_INSUFFICIENT_GOLD", "勇者よ…ゴールドが足らん", 409},
		{"oracle_wallpaper", `{"wallpaper":"goods"}`, store.ErrInsufficientFunds, "ORACLE_INSUFFICIENT_GOLD", "勇者よ…ゴールドが足らん", 409},
		{"oracle_buy", `{"item_no":141}`, store.ErrCostumeNotAvailable, "ORACLE_COSTUME_UNAVAILABLE", "", 400},
		{"oracle_buy", `{"item_no":47}`, store.ErrItemUnavailableInHelperQuest, "ORACLE_COSTUME_UNAVAILABLE", "", 400},
		{"oracle_wallpaper", `{"wallpaper":"x"}`, store.ErrInvalidWallpaper, "ORACLE_INVALID_WALLPAPER", "", 400},
		{"oracle_buy", `{"item_no":47}`, errors.New("private storage failure"), "EXECUTION_FAILED", "", 500},
	} {
		t.Run(tc.action+tc.code, func(t *testing.T) {
			f := newOracleFixture(t)
			f.executionErr = fmt.Errorf("private wrapped: %w", tc.err)
			_, router := f.router(t, "oracle")
			status, got := gatewayRequest(t, router, "hero", "session", "application/json", saleGatewayBody(tc.action, tc.params), nil)
			assertGatewayError(t, got, tc.code)
			if status != tc.status || strings.Contains(fmt.Sprint(got), "private") || !strings.Contains(string(got["error"]), tc.text) {
				t.Fatalf("error mapping: %d %s", status, got)
			}
		})
	}
}

func TestOracleBlackMarketMovesQualifiedCharacters(t *testing.T) {
	f := newOracleFixture(t)
	f.char.JobLevel = 15
	store, router := f.router(t, "oracle")
	status, raw := gatewayRequest(t, router, "hero", "session", "application/json", saleGatewayBody("oracle_blackmarket", `{}`), nil)
	if got := tavernResult[OracleBlackMarketResult](t, raw); status != 200 || got.Message != "闇市場を見つけました！" ||
		got.Selection.Destination != "blackmarket" || store.selection.Destination != "blackmarket" {
		t.Fatalf("black market move: %d %+v", status, got)
	}

	f.char.JobLevel = 14
	store.selection = playercontext.Selection{Destination: "oracle"}
	if status, _ := gatewayRequest(t, router, "hero", "session", "application/json", saleGatewayBody("oracle_blackmarket", `{}`), nil); status != 403 || store.selection.Destination != "oracle" {
		t.Fatalf("unqualified move: %d %+v", status, store.selection)
	}

	f.char.JobLevel = 30
	for _, scene := range []string{"bank", "town"} {
		store.selection = playercontext.Selection{Destination: scene}
		if status, _ := gatewayRequest(t, router, "hero", "session", "application/json", saleGatewayBody("oracle_blackmarket", `{}`), nil); status != 409 || store.selection.Destination != scene {
			t.Fatalf("move from %s: %d %+v", scene, status, store.selection)
		}
		status, raw := gatewayRequest(t, router, "hero", "session", "application/json", saleGatewayBody("scene_enter", `{"destination":"blackmarket"}`), nil)
		if status != 403 || store.selection.Destination != scene {
			t.Fatalf("direct entry from %s: %d %+v", scene, status, store.selection)
		}
		assertGatewayError(t, raw, "SCENE_ACCESS_DENIED")
	}
}

// The offered scene_enter destinations are the ones Enter admits from the observed scene.
func TestBlackMarketEntryDiscoveryMatchesExecution(t *testing.T) {
	for _, tc := range []struct {
		scene    string
		jobLevel int
		offered  bool
	}{{"town", 30, false}, {"oracle", 14, false}, {"oracle", 15, true}} {
		f := newOracleFixture(t)
		f.char.JobLevel = tc.jobLevel
		store, router := f.router(t, tc.scene)
		_, observation := navigationGET(t, router)
		enter := observation.AvailableActions[slices.IndexFunc(observation.AvailableActions, func(a ContextAction) bool { return a.Action == "scene_enter" })]
		var schema struct {
			Properties map[string]struct{ Enum []string }
		}
		if err := json.Unmarshal(enter.ParamsSchema, &schema); err != nil {
			t.Fatal(err)
		}
		status, _ := gatewayRequest(t, router, "hero", "session", "application/json", saleGatewayBody("scene_enter", `{"destination":"blackmarket"}`), nil)
		if offered := slices.Contains(schema.Properties["destination"].Enum, "blackmarket"); offered != tc.offered || (status == 200) != tc.offered {
			t.Fatalf("%s job %d: offered=%v status=%d", tc.scene, tc.jobLevel, offered, status)
		}
		if !tc.offered {
			continue
		}
		if status, _ := gatewayRequest(t, router, "hero", "session", "application/json", saleGatewayBody("scene_back", `{}`), nil); status != 200 || store.selection.Destination != "oracle" {
			t.Fatalf("back: %d %+v", status, store.selection)
		}
	}
}
