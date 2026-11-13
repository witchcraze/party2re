package http

import (
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/witchcraze/party2re/internal/altar"
	"github.com/witchcraze/party2re/internal/playercontext"
	"github.com/witchcraze/party2re/internal/town"
)

var errSceneNotConfigured = errors.New("selected scene adapter not configured")

// sceneAdapter is HTTP-owned. Facility migrations replace the controls-only
// read with their typed public-service projection; it must never perform a
// gameplay mutation or return a partial observation after a required read fails.
type sceneAdapter struct {
	definition playercontext.SceneDefinition
	title      string
	commands   []string
	read       func(context.Context, playercontext.Result) (any, error)
	// placeLog marks facilities with a shared legacy log (docs/architecture/place-log.md).
	placeLog bool
}

type SceneSupport struct {
	Observation string               `json:"observation"`
	Actions     []SceneActionSupport `json:"actions"`
}

type SceneActionSupport struct {
	Action        string `json:"action"`
	Connected     bool   `json:"connected"`
	EntryEligible bool   `json:"entry_eligible"`
	Mode          string `json:"mode,omitempty"`
}

type SceneDestination struct {
	ID          string           `json:"id"`
	Title       string           `json:"title"`
	Supported   bool             `json:"supported"`
	EnterParams sceneEnterParams `json:"enter_params"`
}

type ScenePage struct {
	Mode   string                    `json:"mode"`
	Offset int                       `json:"offset"`
	Limit  int                       `json:"limit"`
	Next   *playercontext.PageParams `json:"next"`
}

type TownSceneData struct {
	Destinations []SceneDestination `json:"destinations"`
	Page         ScenePage          `json:"page"`
}

// SelectionSceneData contains navigation facts only, never a raw facility or
// another character's private state. Feature detail remains adapter-owned.
type SelectionSceneData struct {
	Parent string `json:"parent"`
}

func (h *Handler) registerScenes(service *playercontext.Service) {
	h.sceneAdapters = make(map[string]sceneAdapter)
	definitions := []playercontext.SceneDefinition{{ID: "town"}}
	if service != nil && service.NavigationConfigured() {
		definitions = service.SceneDefinitions()
	}
	for _, definition := range definitions {
		adapter := sceneAdapter{definition: definition}
		switch definition.ID {
		case "town":
			adapter.title, adapter.commands = "交流広場", []string{"adventure_start", "park_divinate"}
			adapter.read = func(_ context.Context, r playercontext.Result) (any, error) { return h.townSceneData(r), nil }
		case "town1", "town2", "town3", "town4":
			t, _ := town.GetTown(definition.ID)
			adapter.title = t.Name
			adapter.read = h.townDistrictSceneData
		case "alchemy":
			adapter.title, adapter.commands = "錬金場", []string{"alchemy_synthesize", "alchemy_recipes"}
			adapter.read = h.alchemySceneData
		case "altar":
			adapter.title, adapter.commands = "復活の祭壇", []string{"altar_pray", "altar_wish"}
			adapter.read = h.altarSceneData
		case "wishingwell":
			adapter.title, adapter.commands = "願いの泉", []string{"wishingwell_exchange"}
			adapter.read = h.wishingWellSceneData
		case "god", "u_god":
			adapter.title = map[string]string{"god": "天界", "u_god": "天界?"}[definition.ID]
			adapter.commands, adapter.read = []string{"god_wish"}, h.godSceneData
		case "bank":
			adapter.title, adapter.commands = "銀行", []string{"bank_deposit", "bank_withdraw"}
			adapter.read = h.bankSceneData
		case "blackmarket":
			adapter.title, adapter.commands = "闇市場", []string{"blackmarket_sacrifice", "blackmarket_trade"}
			adapter.read = h.blackMarketSceneData
		case "blacksmith":
			adapter.title, adapter.commands = "鍛冶屋", []string{"blacksmith_seal", "blacksmith_name", "blacksmith_deposit", "blacksmith_withdraw"}
			adapter.read = h.blacksmithSceneData
		case "chapel":
			adapter.title, adapter.commands = "礼拝堂", []string{"chapel_pray"}
			adapter.read = h.chapelSceneData
		case "exile":
			adapter.title, adapter.commands = "荒らし追放騎士団", []string{"exile_petition", "exile_approve", "exile_oppose"}
			adapter.read = h.exileSceneData
		case "depot":
			adapter.title, adapter.commands = "預かり所", []string{"depot_deposit", "depot_send_item", "depot_send_money", "depot_withdraw", "depot_expand", "depot_sell", "depot_sell_batch", "depot_sort"}
			adapter.read = h.depotSceneData
		case "auction":
			adapter.title, adapter.commands = "オークション会場", []string{"auction_send"}
		case "fleamarket":
			adapter.title, adapter.commands = "フリーマーケット", []string{"fleamarket_list", "fleamarket_purchase", "fleamarket_cancel"}
			adapter.read = h.fleaMarketSceneData
		case "gemstore":
			adapter.title, adapter.commands = "宝石店", []string{"gemstore_buy", "gemstore_sell", "gemstore_send", "gemstore_synthesize", "gemstore_appraise", "gemstore_sort"}
			adapter.read = h.gemStoreSceneData
		case "secretshop":
			adapter.title, adapter.commands = "秘密の店", []string{"secretshop_purchase", "secretshop_puffpuff"}
			adapter.read = h.secretShopSceneData
		case "home":
			adapter.title, adapter.commands = "自宅", []string{"home_sleep"}
			adapter.read = h.homeSceneData
		case "home_inbox", "home_outbox":
			adapter.title = map[string]string{"home_inbox": "受信箱", "home_outbox": "送信箱"}[definition.ID]
			adapter.read = h.homeMailboxSceneData
		case "casino":
			adapter.title, adapter.commands = "カジノ", []string{"casino_slot", "casino_exchange", "casino_prize_exchange", "casino_room_create"}
			adapter.read = h.casinoSceneData
		case "shop_weapon", "shop_armor", "shop_item", "shop_accessory":
			adapter.title = map[string]string{"shop_weapon": "武器店", "shop_armor": "防具店", "shop_item": "道具店", "shop_accessory": "装飾品店"}[definition.ID]
			adapter.commands = []string{"shop_purchase", "shop_batch_purchase", "shop_sell"}
			if definition.ID == "shop_accessory" {
				adapter.commands = []string{"shop_purchase", "shop_sell", "shop_synthesize"}
			}
			adapter.read = h.shopSceneData
		default:
			// Navigation registration does not manufacture observation support.
			h.sceneAdapters[definition.ID] = adapter
			continue
		}
		if adapter.placeLog = placeLogFacilities[definition.ID]; adapter.placeLog {
			adapter.commands = append(adapter.commands, "place_say", "place_whisper")
		}
		if _, ok := npcProfiles[definition.ID]; ok {
			adapter.commands = append(adapter.commands, "npc_talk", "npc_inspect")
		}
		if adapter.read == nil {
			adapter.read = func(_ context.Context, _ playercontext.Result) (any, error) {
				return SelectionSceneData{Parent: definition.Parent}, nil
			}
		}
		h.sceneAdapters[definition.ID] = adapter
	}
}

func (h *Handler) townSceneData(result playercontext.Result) TownSceneData {
	rows := make([]SceneDestination, 0, len(h.sceneAdapters))
	for id, adapter := range h.sceneAdapters {
		if adapter.definition.Parent != "town" {
			continue
		}
		if canEnter := adapter.definition.CanEnter; canEnter != nil && !canEnter(result.Snapshot.Character) {
			continue
		}
		rows = append(rows, SceneDestination{ID: id, Title: adapter.title, Supported: adapter.read != nil, EnterParams: sceneEnterParams{Destination: id}})
	}
	// IDs are the stable tie-breaker; no cross-page snapshot is promised.
	slices.SortFunc(rows, func(a, b SceneDestination) int { return strings.Compare(a.ID, b.ID) })
	n := playercontext.Selection{Destination: "town"}
	if result.Navigation != nil {
		n = result.Navigation.Selection
	}
	return pageDestinations(rows, n)
}

func pageDestinations(rows []SceneDestination, n playercontext.Selection) TownSceneData {
	limit := n.Limit
	if limit == 0 {
		limit = 20
	}
	start := min(n.Offset, len(rows))
	end := min(start+limit, len(rows))
	page := ScenePage{Mode: "offset", Offset: n.Offset, Limit: limit}
	if end < len(rows) {
		page.Next = &playercontext.PageParams{Destination: n.Destination, Offset: end, Limit: limit}
	}
	return TownSceneData{Destinations: append([]SceneDestination{}, rows[start:end]...), Page: page}
}

func (h *Handler) composeSelectedScene(ctx context.Context, result playercontext.Result, response *PlayerContextResponse) error {
	if active, err := h.composeActivityScene(ctx, result, response); active || err != nil {
		return err
	}
	n := playercontext.Selection{Destination: "town"}
	unavailable := false
	if result.Navigation != nil {
		n, unavailable = result.Navigation.Selection, result.Navigation.Unavailable
	}
	adapter, registered := h.sceneAdapters[n.Destination]
	if !unavailable && (!registered || adapter.read == nil) {
		return errSceneNotConfigured
	}
	scene := &response.Scene
	scene.LocationID, scene.Kind = n.Destination, "facility"
	if n.Destination != "town" || unavailable {
		scene.Bgimg, scene.BgimgURL, scene.Speaker = "", "", nil
	}
	scene.Support = SceneSupport{Observation: "controls", Actions: []SceneActionSupport{}}
	if n.Subject != (playercontext.Subject{}) {
		scene.Subject, scene.Kind = &n.Subject, "subject"
	}
	choices := append([]string{}, adapter.commands...)
	templates := make(map[string]map[string]any)
	var data any
	if !unavailable {
		var err error
		data, err = adapter.read(ctx, result)
		if errors.Is(err, playercontext.ErrSelectionNotFound) {
			unavailable = true
		} else if err != nil {
			return err
		}
	}
	if unavailable {
		scene.Kind, scene.Title, scene.Dialogue, scene.Speaker = "selection_unavailable", "選択した対象は利用できません", "戻って対象を選び直してください。", nil
		scene.Data = SelectionSceneData{Parent: adapter.definition.Parent}
		choices = []string{"scene_enter"}
		if registered {
			choices = append(choices, "scene_back")
		}
	} else {
		scene.Title = adapter.title
		if n.Destination == "town" {
			scene.Kind = "town"
		} else {
			scene.Dialogue = "操作を選んでください。"
		}
		scene.Data = data
		if adapter.placeLog {
			var err error
			if scene.PlaceLog, err = h.readPlaceLog(ctx, n.Destination, result.Snapshot.Character.ID); err != nil {
				return err
			}
			if scene.Members, err = h.readPlaceMembers(ctx, n.Destination); err != nil {
				return err
			}
		}
		switch data := data.(type) {
		case BankSceneData, BlacksmithSceneData, ChapelSceneData, ExileSceneData, GemStoreSceneData, GodSceneData, WishingWellSceneData:
			scene.Support.Observation = "details"
		case AltarSceneData:
			scene.Support.Observation = "details"
			// Legacy ＠いのる lists Ramia as a member until she leaves (reborn.cgi:81-84).
			if data.RamiaPresent && adapter.placeLog {
				scene.Members = append(scene.Members, PlaceMember{Kind: "npc", Name: altar.RamiaMemberName, Color: npcColor})
			}
		case AlchemySceneData:
			scene.Support.Observation = "details"
			if p := data.Page.Next; p != nil {
				choices = append(choices, "scene_page")
				templates["scene_page"] = map[string]any{"destination": p.Destination, "offset": p.Offset, "limit": p.Limit}
			}
		case BlackMarketSceneData:
			scene.Support.Observation = "details"
			if p := data.Page.Next; p != nil {
				choices = append(choices, "scene_page")
				templates["scene_page"] = map[string]any{"destination": p.Destination, "offset": p.Offset, "limit": p.Limit}
			}
		case FleaMarketSceneData:
			scene.Support.Observation = "details"
			if p := data.Page.Next; p != nil {
				choices = append(choices, "scene_page")
				templates["scene_page"] = map[string]any{"destination": p.Destination, "offset": p.Offset, "limit": p.Limit}
			}
		case DepotSceneData:
			scene.Support.Observation = "details"
			if p := data.Page.Next; p != nil {
				choices = append(choices, "scene_page")
				templates["scene_page"] = map[string]any{"destination": p.Destination, "offset": p.Offset, "limit": p.Limit}
			}
		case CasinoLobbySceneData:
			scene.Support.Observation = "details"
			if data.Page.Next != nil {
				choices = append(choices, "scene_page")
				p := data.Page.Next
				templates["scene_page"] = map[string]any{"destination": p.Destination, "offset": p.Offset, "limit": p.Limit}
			}
		case CasinoSelectedSceneData:
			scene.Support.Observation, scene.Title = "details", data.Room.Name
			choices = []string{"casino_room_join"}
			templates["casino_room_join"] = map[string]any{"room_id": data.Room.ID}
			if data.SpectateParams != nil {
				choices = append(choices, "casino_room_spectate")
				templates["casino_room_spectate"] = map[string]any{"room_id": data.Room.ID}
			}
		case ShopCatalogSceneData:
			scene.Support.Observation, scene.Title = "details", data.Title
			choices = shopTemplates(data.ShopSceneInfo, choices, templates)
			if data.Page.Next != nil {
				choices = append(choices, "scene_page")
				p := data.Page.Next
				templates["scene_page"] = map[string]any{"destination": p.Destination, "offset": p.Offset, "limit": p.Limit}
			}
		case ShopProductSceneData:
			scene.Support.Observation, scene.Title = "details", data.Title
			choices = shopTemplates(data.ShopSceneInfo, choices, templates)
			templates["shop_purchase"]["item_definition_id"] = data.Product.PurchaseParams.ItemDefinitionID
		case SecretShopCatalogSceneData:
			scene.Support.Observation, scene.Title = "details", data.Title
			if p := data.Page.Next; p != nil {
				choices = append(choices, "scene_page")
				templates["scene_page"] = map[string]any{"destination": p.Destination, "offset": p.Offset, "limit": p.Limit}
			}
		case SecretShopProductSceneData:
			scene.Support.Observation, scene.Title = "details", data.Title
			templates["secretshop_purchase"] = map[string]any{"item_id": data.Product.PurchaseParams.ItemID}
		case TownDistrictSceneData:
			scene.Support.Observation = "details"
			choices = append(choices, "store_build")
			templates["store_build"] = map[string]any{"town_id": data.Town.ID}
		case StoreSceneData:
			scene.Support.Observation, scene.Title = "details", data.Details.Store.StoreName
			commands := storeVisitorCommands
			if data.IsOwner {
				commands = storeOwnerCommands
			}
			choices = append(choices, commands...)
		case HomeSceneData:
			scene.Support.Observation = "details"
			scene.Title = data.View.Owner.Name + "の家"
			if data.View.IsOwner {
				choices = append(choices, "scene_enter")
			}
			templates["home_sleep"] = map[string]any{"target_home_id": data.View.Owner.ID}
		case HomeMailboxSceneData:
			scene.Support.Observation = "details"
			if data.Page.Next != nil {
				choices = append(choices, "scene_page")
				p := data.Page.Next
				templates["scene_page"] = map[string]any{"destination": p.Destination, "limit": p.Limit}
				if p.Cursor != nil {
					templates["scene_page"]["cursor"] = *p.Cursor
				} else {
					templates["scene_page"]["offset"] = p.Offset
				}
			}
		}
		if result.Navigation == nil {
			// Without a selector, preserve entry discovery for configured commands.
			choices = append([]string{}, result.AvailableActions...)
		} else if n.Destination == "town" {
			choices = append(choices, "scene_enter")
			if data, ok := data.(TownSceneData); ok && adapter.definition.Pageable && data.Page.Next != nil {
				choices = append(choices, "scene_page")
				templates["scene_page"] = map[string]any{"destination": n.Destination, "offset": data.Page.Next.Offset, "limit": data.Page.Next.Limit}
			}
		} else {
			choices = append(choices, "scene_back")
			if n.Subject == (playercontext.Subject{}) && adapter.definition.SubjectKind != "" {
				choices = append(choices, "scene_select")
				templates["scene_select"] = map[string]any{"target_kind": adapter.definition.SubjectKind}
			}
		}
	}
	choices = append(choices, "player_inspect", "home_wake", "rescue_request", "presence_refresh")
	for _, def := range playercontext.DefaultCatalog {
		if !slices.Contains(choices, def.ID) {
			continue
		}
		connected := h.actionCommands[def.ID].prepare != nil
		eligible := slices.Contains(result.AvailableActions, def.ID)
		scene.Support.Actions = append(scene.Support.Actions, SceneActionSupport{Action: def.ID, Connected: connected, EntryEligible: eligible})
		if !connected || !eligible {
			continue
		}
		action, err := h.contextAction(def.ID, templates[def.ID], result.Snapshot.Character)
		if err != nil {
			return err
		}
		response.AvailableActions = append(response.AvailableActions, action)
	}
	return nil
}
