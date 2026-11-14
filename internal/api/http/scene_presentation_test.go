package http

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"testing"

	"github.com/witchcraze/party2re/internal/altar"
	"github.com/witchcraze/party2re/internal/casino"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/home"
	"github.com/witchcraze/party2re/internal/playercontext"
	"github.com/witchcraze/party2re/internal/shop"
	"github.com/witchcraze/party2re/internal/store"
	"github.com/witchcraze/party2re/internal/tavern"
)

// These vectors pin the presentation adjustments independently of facility reads.
func TestFacilityScenePresentation(t *testing.T) {
	cursor := "next-cursor"
	next := &playercontext.PageParams{Offset: 20, Limit: 20}
	page := ScenePage{Next: next}
	for _, tc := range []struct {
		name, destination, title string
		data                     any
		hidden                   []string
		template                 map[string]any
		command                  string
		controls, ramia, denied  bool
	}{
		{name: "town", destination: "town", data: TownSceneData{Page: page}, controls: true, command: "scene_page", template: map[string]any{"destination": "", "offset": 20, "limit": 20}},
		{name: "alchemy", destination: "alchemy", data: AlchemySceneData{Page: page}, command: "scene_page", template: map[string]any{"destination": "", "offset": 20, "limit": 20}},
		{name: "blackmarket", destination: "blackmarket", data: BlackMarketSceneData{Page: page}, command: "scene_page", template: map[string]any{"destination": "", "offset": 20, "limit": 20}},
		{name: "fleamarket", destination: "fleamarket", data: FleaMarketSceneData{Page: page}, command: "scene_page", template: map[string]any{"destination": "", "offset": 20, "limit": 20}},
		{name: "depot", destination: "depot", data: DepotSceneData{Page: page}, command: "scene_page", template: map[string]any{"destination": "", "offset": 20, "limit": 20}},
		{name: "casino lobby", destination: "casino", data: CasinoLobbySceneData{Page: page}, command: "scene_page", template: map[string]any{"destination": "", "offset": 20, "limit": 20}},
		{name: "casino closed", destination: "casino", data: CasinoSelectedSceneData{Room: casino.RoomSummary{Name: "cards"}}, title: "cards", hidden: []string{"casino_room_spectate", "scene_page"}},
		{name: "casino open", destination: "casino", data: CasinoSelectedSceneData{Room: casino.RoomSummary{Name: "cards"}, SpectateParams: &CasinoRoomParams{}}, title: "cards", hidden: []string{"scene_page"}},
		{name: "shop catalog", destination: "shop_weapon", data: ShopCatalogSceneData{ShopSceneInfo: ShopSceneInfo{Title: "weapons"}, Page: page}, title: "weapons", hidden: []string{"shop_sell"}, command: "scene_page", template: map[string]any{"destination": "", "offset": 20, "limit": 20}},
		{name: "shop product", destination: "shop_weapon", data: ShopProductSceneData{ShopSceneInfo: ShopSceneInfo{Title: "weapon", SaleOffer: &shop.SaleOffer{ItemInstanceID: "sale-item"}}}, title: "weapon", hidden: []string{"scene_page"}, command: "shop_sell", template: map[string]any{"shop_type": "weapon", "item_instance_id": "sale-item"}},
		{name: "secret catalog", destination: "secretshop", data: SecretShopCatalogSceneData{SecretShopSceneInfo: SecretShopSceneInfo{Title: "secret"}, Page: page}, title: "secret", command: "scene_page", template: map[string]any{"destination": "", "offset": 20, "limit": 20}},
		{name: "secret product", destination: "secretshop", data: SecretShopProductSceneData{SecretShopSceneInfo: SecretShopSceneInfo{Title: "secret item"}}, title: "secret item", hidden: []string{"scene_page"}},
		{name: "store visitor", destination: "town1", data: StoreSceneData{Details: store.StoreDetails{Store: store.Store{StoreName: "store"}}}, title: "store", hidden: []string{"scene_page"}},
		{name: "store owner", destination: "town1", data: StoreSceneData{Details: store.StoreDetails{Store: store.Store{StoreName: "store"}}, IsOwner: true}, title: "store", hidden: []string{"scene_page"}},
		{name: "district", destination: "town1", data: TownDistrictSceneData{}, hidden: []string{"scene_page"}},
		{name: "home", destination: "home", data: HomeSceneData{View: home.HomeView{Owner: home.HomeOwner{Name: "hero"}}}, title: "heroの家", hidden: []string{"scene_page"}},
		{name: "oracle permitted", destination: "oracle", data: OracleSceneData{}, hidden: []string{"scene_page"}},
		{name: "oracle denied", destination: "oracle", data: OracleSceneData{}, hidden: []string{"oracle_blackmarket", "scene_page"}, denied: true},
		{name: "tavern no delivery", destination: "tavern", data: TavernSceneData{}, hidden: []string{"tavern_delivery_cancel", "scene_page"}},
		{name: "tavern delivery", destination: "tavern", data: TavernSceneData{Delivery: &tavern.DeliveryReservation{}}, hidden: []string{"scene_page"}},
		{name: "altar present", destination: "altar", data: AltarSceneData{AltarStatus: altar.AltarStatus{RamiaPresent: true}}, hidden: []string{"scene_page"}, ramia: true},
		{name: "altar absent", destination: "altar", data: AltarSceneData{}, hidden: []string{"scene_page"}},
		{name: "mail offset", destination: "home_inbox", data: HomeMailboxSceneData{Page: page}, command: "scene_page", template: map[string]any{"destination": "", "offset": 20, "limit": 20}},
		{name: "mail cursor", destination: "home_outbox", data: HomeMailboxSceneData{Page: ScenePage{Next: &playercontext.PageParams{Destination: "home_outbox", Limit: 20, Cursor: &cursor}}}, command: "scene_page", template: map[string]any{"destination": "home_outbox", "cursor": cursor, "limit": 20}},
		{name: "mail end", destination: "home_inbox", data: HomeMailboxSceneData{}, hidden: []string{"scene_page"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := facilities[tc.destination]
			choices := slices.Concat(f.commands, f.subjectCommands, f.ownedSubjectCommands, []string{"scene_page"})
			result := playercontext.Result{Snapshot: playercontext.Snapshot{SceneActions: choices}, AvailableActions: choices,
				Navigation: &playercontext.NavigationObservation{Selection: playercontext.Selection{Destination: tc.destination}}}
			service := playercontext.NewService(nil, nil, nil, playercontext.WithNavigation(&gatewayNavigationStore{},
				playercontext.SceneDefinition{ID: "town"}, playercontext.SceneDefinition{ID: "blackmarket", Parent: "town", CanEnter: func(corecharacter.Character) bool { return !tc.denied }}))
			h := &Handler{playerContext: service, actionCommands: map[string]actionCommand{}, sceneAdapters: map[string]sceneAdapter{
				tc.destination: {facility: f, read: func(context.Context, playercontext.Result) (any, error) { return tc.data, nil }},
			}}
			// Connection is independent of registration and entry eligibility.
			for _, id := range choices {
				if contextParamsSchemas[id] != nil {
					h.actionCommands[id] = actionCommand{prepare: func(json.RawMessage) (func(context.Context, string, string) (any, error), error) { return nil, nil }}
				}
			}
			var response PlayerContextResponse
			if err := h.composeSelectedScene(context.Background(), result, &response); err != nil {
				t.Fatal(err)
			}
			wantTitle := tc.title
			if wantTitle == "" {
				wantTitle = f.title
			}
			wantObservation := "details"
			if tc.controls {
				wantObservation = "controls"
			}
			if response.Scene.Title != wantTitle || response.Scene.Support.Observation != wantObservation || !reflect.DeepEqual(response.Scene.Data, tc.data) {
				t.Fatalf("scene: %+v", response.Scene)
			}
			for _, id := range choices {
				want := !slices.Contains(tc.hidden, id)
				connected := contextParamsSchemas[id] != nil
				if slices.Contains(actionIDs(response.AvailableActions), id) != (want && connected) || slices.ContainsFunc(response.Scene.Support.Actions, func(a SceneActionSupport) bool { return a.Action == id && a.EntryEligible && a.Connected == connected }) != want {
					t.Fatalf("%s visibility: %+v", id, response)
				}
			}
			if tc.command != "" {
				i := slices.IndexFunc(response.AvailableActions, func(a ContextAction) bool { return a.Action == tc.command })
				if i < 0 || !reflect.DeepEqual(response.AvailableActions[i].ParamsTemplate, tc.template) {
					t.Fatalf("template: %+v", response.AvailableActions)
				}
				var schema struct {
					Properties map[string]struct{ Const any }
				}
				if err := json.Unmarshal(response.AvailableActions[i].ParamsSchema, &schema); err != nil {
					t.Fatal(err)
				}
				for key, want := range tc.template {
					got, _ := json.Marshal(schema.Properties[key].Const)
					expected, _ := json.Marshal(want)
					if string(got) != string(expected) {
						t.Fatalf("%s schema constant: %s, want %s", key, got, expected)
					}
				}
			}
			wantMembers := []PlaceMember{}
			if f.npc != nil && f.placeLog {
				wantMembers = append(wantMembers, PlaceMember{Kind: "npc", Name: f.npc.name, Color: npcColor})
			}
			if tc.ramia {
				wantMembers = append(wantMembers, PlaceMember{Kind: "npc", Name: altar.RamiaMemberName, Color: npcColor})
			}
			if !slices.Equal(response.Scene.Members, wantMembers) {
				t.Fatalf("members: %+v, want %+v", response.Scene.Members, wantMembers)
			}
		})
	}
}

func TestScenePresentationCannotAdmitCommands(t *testing.T) {
	for _, tc := range []struct {
		name                          string
		controls, unavailable, noRead bool
		readErr                       error
	}{
		{name: "details"},
		{name: "controls", controls: true},
		{name: "selection fallback", noRead: true},
		{name: "unavailable", unavailable: true},
		{name: "disappeared", readErr: playercontext.ErrSelectionNotFound},
		{name: "failed", readErr: errors.New("required read failed")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const destination = "custom_facility"
			data := struct{ Fact string }{"custom observation"}
			called := false
			f := facility{title: "default", controlsOnly: tc.controls, placeLog: true,
				read: func(*Handler, context.Context, playercontext.Result) (any, error) { return data, tc.readErr },
				present: func(_ *Handler, _ playercontext.Result, observed any) scenePresentation {
					called = true
					if observed != data {
						t.Fatalf("hook data: %+v", observed)
					}
					title := ""
					return scenePresentation{title: &title, next: &playercontext.PageParams{Destination: destination, Limit: 20},
						hide: []string{"shop_sell"}, members: []PlaceMember{{Kind: "npc", Name: "extra"}},
						templates: map[string]map[string]any{"bank_deposit": {"amount": 1}, "shop_purchase": {"shop_type": "weapon"}}}
				}}
			adapter := sceneAdapter{facility: f, read: func(ctx context.Context, r playercontext.Result) (any, error) { return f.read(nil, ctx, r) }}
			if tc.noRead {
				adapter.facility.read, adapter.present = nil, nil
				adapter.read = func(context.Context, playercontext.Result) (any, error) {
					return SelectionSceneData{Parent: "town"}, nil
				}
			}
			h := &Handler{sceneAdapters: map[string]sceneAdapter{destination: adapter}, actionCommands: map[string]actionCommand{}}
			for _, id := range []string{"scene_page", "shop_sell", "shop_purchase", "bank_deposit"} {
				h.actionCommands[id] = actionCommand{prepare: func(json.RawMessage) (func(context.Context, string, string) (any, error), error) { return nil, nil }}
			}
			r := playercontext.Result{Snapshot: playercontext.Snapshot{SceneActions: []string{"scene_page", "shop_sell", "shop_purchase"}},
				AvailableActions: []string{"scene_page", "shop_sell", "bank_deposit"},
				Navigation:       &playercontext.NavigationObservation{Selection: playercontext.Selection{Destination: destination}, Unavailable: tc.unavailable}}
			var response PlayerContextResponse
			err := h.composeSelectedScene(context.Background(), r, &response)
			if tc.readErr != nil && !errors.Is(tc.readErr, playercontext.ErrSelectionNotFound) {
				if !errors.Is(err, tc.readErr) || called {
					t.Fatalf("read failure: %v, hook called: %v", err, called)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if tc.unavailable || tc.readErr != nil || tc.noRead {
				if called || response.Scene.Support.Observation != "controls" {
					t.Fatalf("fallback: %+v, hook called: %v", response.Scene, called)
				}
				return
			}
			wantObservation := "details"
			if tc.controls {
				wantObservation = "controls"
			}
			if !called || response.Scene.Title != "" || response.Scene.Support.Observation != wantObservation || !slices.Equal(actionIDs(response.AvailableActions), []string{"scene_page"}) || !slices.Equal(response.Scene.Members, []PlaceMember{{Kind: "npc", Name: "extra"}}) {
				t.Fatalf("presentation changed eligibility or lost adjustments: %+v", response)
			}
			if !slices.ContainsFunc(response.Scene.Support.Actions, func(a SceneActionSupport) bool { return a.Action == "shop_purchase" && a.Connected && !a.EntryEligible }) || slices.ContainsFunc(response.Scene.Support.Actions, func(a SceneActionSupport) bool { return a.Action == "bank_deposit" || a.Action == "shop_sell" }) {
				t.Fatalf("support bypassed registered choices: %+v", response.Scene.Support)
			}
		})
	}
}
