package http

import (
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/witchcraze/party2re/internal/playercontext"
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
}

type SceneSupport struct {
	Observation string               `json:"observation"`
	Actions     []SceneActionSupport `json:"actions"`
}

type SceneActionSupport struct {
	Action        string `json:"action"`
	Connected     bool   `json:"connected"`
	EntryEligible bool   `json:"entry_eligible"`
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
			adapter.title, adapter.commands = "始まりの街", []string{"adventure_start"}
			adapter.read = func(_ context.Context, r playercontext.Result) (any, error) { return h.townSceneData(r), nil }
		case "bank":
			adapter.title, adapter.commands = "銀行", []string{"bank_deposit", "bank_withdraw"}
		case "home":
			adapter.title, adapter.commands = "自宅", []string{"home_sleep"}
			adapter.read = h.homeSceneData
		case "home_inbox", "home_outbox":
			adapter.title = map[string]string{"home_inbox": "受信箱", "home_outbox": "送信箱"}[definition.ID]
			adapter.read = h.homeMailboxSceneData
		case "shop_weapon", "shop_armor", "shop_item", "shop_accessory":
			adapter.title = map[string]string{"shop_weapon": "武器店", "shop_armor": "防具店", "shop_item": "道具店", "shop_accessory": "装飾品店"}[definition.ID]
			adapter.commands = []string{"shop_purchase"}
			if definition.ID == "shop_accessory" {
				adapter.commands = []string{"shop_accessory_buy"}
			}
			adapter.read = h.shopSceneData
		default:
			// Navigation registration does not manufacture observation support.
			h.sceneAdapters[definition.ID] = adapter
			continue
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
		switch data := data.(type) {
		case ShopCatalogSceneData:
			scene.Support.Observation, scene.Title = "details", data.Title
			if data.Page.Next != nil {
				choices = append(choices, "scene_page")
				p := data.Page.Next
				templates["scene_page"] = map[string]any{"destination": p.Destination, "offset": p.Offset, "limit": p.Limit}
			}
		case ShopProductSceneData:
			scene.Support.Observation, scene.Title = "details", data.Title
			templates[adapter.commands[0]] = map[string]any{"item_definition_id": data.Product.PurchaseParams.ItemDefinitionID}
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
	choices = append(choices, "home_wake", "rescue_request")
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
		action, err := h.contextAction(def.ID, templates[def.ID])
		if err != nil {
			return err
		}
		response.AvailableActions = append(response.AvailableActions, action)
	}
	return nil
}
