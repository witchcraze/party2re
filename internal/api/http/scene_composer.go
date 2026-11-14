package http

import (
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/witchcraze/party2re/internal/playercontext"
	"github.com/witchcraze/party2re/internal/shop"
	"github.com/witchcraze/party2re/internal/town"
)

var errSceneNotConfigured = errors.New("selected scene adapter not configured")

// facility is one Gateway facility's HTTP registration, keyed by scene
// destination. read observes a typed public-service projection; it must never
// perform a gameplay mutation or return a partial observation after a required
// read fails. A nil read offers the controls-only selection view.
type facility struct {
	title    string
	commands []string
	// subjectCommands replace commands while a subject is selected, and
	// ownedSubjectCommands replace those for a subject the actor owns.
	subjectCommands, ownedSubjectCommands []string
	// bind fixes command inputs to the observed selection. Discovery offers
	// them as schema constants and execution rejects any other submitted value.
	bind func(n playercontext.Selection, actorID string) map[string]map[string]any
	read func(*Handler, context.Context, playercontext.Result) (any, error)
	// present only adjusts display facts; it cannot admit commands.
	present      func(*Handler, playercontext.Result, any) scenePresentation
	controlsOnly bool
	// placeLog marks facilities with a shared legacy log (docs/architecture/place-log.md).
	placeLog bool
	npc      *npcProfile
}

type scenePresentation struct {
	title     *string
	next      *playercontext.PageParams
	hide      []string
	templates map[string]map[string]any
	members   []PlaceMember
}

type sceneAdapter struct {
	definition playercontext.SceneDefinition
	facility
	read func(context.Context, playercontext.Result) (any, error)
}

// facilities holds every HTTP facility registration.
var facilities = func() map[string]facility {
	m := map[string]facility{
		"town": townFacility, "alchemy": alchemyFacility, "altar": altarFacility, "wishingwell": wishingWellFacility,
		"god": godFacility, "u_god": underworldGodFacility, "auction": auctionFacility, "bank": bankFacility,
		"blackmarket": blackMarketFacility, "blacksmith": blacksmithFacility, "chapel": chapelFacility, "exile": exileFacility,
		"depot": depotFacility, "fleamarket": fleaMarketFacility, "gemstore": gemStoreFacility, "secretshop": secretShopFacility,
		"home": homeFacility, "home_inbox": homeInboxFacility, "home_outbox": homeOutboxFacility, "casino": casinoFacility, "tavern": tavernFacility, "lottery": lotteryFacility, "takarakuji": takarakujiFacility, "plantation": plantationFacility, "oracle": oracleFacility, "contest": contestFacility,
	}
	for _, t := range town.AllTowns() {
		m[t.ID] = townDistrictFacility(t.Name)
	}
	for _, kind := range []shop.ShopType{shop.ShopTypeWeapon, shop.ShopTypeArmor, shop.ShopTypeItem, shop.ShopTypeAccessory} {
		m["shop_"+string(kind)] = shopFacility(kind)
	}
	return m
}()

// FacilityIDs lists the registered facility destinations; the composition
// root checks them against its scene definitions.
func FacilityIDs() []string {
	ids := make([]string, 0, len(facilities))
	for id := range facilities {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return ids
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
		f, ok := facilities[definition.ID]
		if !ok {
			// Navigation registration does not manufacture observation support.
			h.sceneAdapters[definition.ID] = sceneAdapter{definition: definition}
			continue
		}
		adapter := sceneAdapter{definition: definition, facility: f}
		if service != nil && service.NavigationConfigured() {
			var shared []string
			if f.placeLog {
				shared = append(shared, "place_say", "place_whisper")
			}
			if f.npc != nil {
				shared = append(shared, "npc_talk", "npc_inspect")
			}
			var subject, owned []string
			if f.subjectCommands != nil {
				subject = slices.Concat(f.subjectCommands, shared)
			}
			if f.ownedSubjectCommands != nil {
				owned = slices.Concat(f.ownedSubjectCommands, shared)
			}
			service.AdmitSceneActions(definition.ID, slices.Concat(f.commands, shared), subject, owned)
		}
		adapter.read = func(_ context.Context, _ playercontext.Result) (any, error) {
			return SelectionSceneData{Parent: definition.Parent}, nil
		}
		if f.read != nil {
			adapter.read = func(ctx context.Context, r playercontext.Result) (any, error) { return f.read(h, ctx, r) }
		}
		h.sceneAdapters[definition.ID] = adapter
	}
}

// boundParams returns the command inputs the observed selection fixes.
func (h *Handler) boundParams(result playercontext.Result) map[string]map[string]any {
	if n := result.Navigation; n != nil && !n.Unavailable {
		if bind := h.sceneAdapters[n.Selection.Destination].bind; bind != nil {
			if bound := bind(n.Selection, result.Snapshot.Character.ID); bound != nil {
				return bound
			}
		}
	}
	return map[string]map[string]any{}
}

func (h *Handler) townSceneData(_ context.Context, result playercontext.Result) (any, error) {
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
	return pageDestinations(rows, n), nil
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
	// The scene's admitted commands come from the shared availability policy;
	// observed facts below only hide offers the owning service would reject.
	choices := slices.Clone(result.Snapshot.SceneActions)
	if result.Navigation == nil {
		// Without a selector, preserve entry discovery for configured commands.
		choices = slices.Clone(result.AvailableActions)
	}
	hide := func(ids ...string) {
		choices = slices.DeleteFunc(choices, func(id string) bool { return slices.Contains(ids, id) })
	}
	templates := h.boundParams(result)
	if slices.Contains(choices, "scene_select") {
		templates["scene_select"] = map[string]any{"target_kind": adapter.definition.SubjectKind}
	}
	var data any
	if !unavailable {
		var err error
		data, err = adapter.read(ctx, result)
		if errors.Is(err, playercontext.ErrSelectionNotFound) {
			unavailable = true
			choices = []string{"scene_enter", "scene_back"}
			delete(templates, "scene_select")
		} else if err != nil {
			return err
		}
	}
	if unavailable {
		scene.Kind, scene.Title, scene.Dialogue, scene.Speaker = "selection_unavailable", "選択した対象は利用できません", "戻って対象を選び直してください。", nil
		scene.Data = SelectionSceneData{Parent: adapter.definition.Parent}
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
		if adapter.facility.read != nil && !adapter.controlsOnly {
			scene.Support.Observation = "details"
		}
		if adapter.present != nil {
			presentation := adapter.present(h, result, data)
			if presentation.title != nil {
				scene.Title = *presentation.title
			}
			hide(presentation.hide...)
			for id, params := range presentation.templates {
				if templates[id] == nil {
					templates[id] = make(map[string]any)
				}
				for key, value := range params {
					templates[id][key] = value
				}
			}
			if p := presentation.next; p != nil {
				templates["scene_page"] = map[string]any{"destination": p.Destination, "limit": p.Limit}
				if p.Cursor != nil {
					templates["scene_page"]["cursor"] = *p.Cursor
				} else {
					templates["scene_page"]["offset"] = p.Offset
				}
			}
			if adapter.placeLog {
				scene.Members = append(scene.Members, presentation.members...)
			}
		}
		if templates["scene_page"] == nil {
			hide("scene_page")
		}
	}
	for _, def := range playercontext.DefaultCatalog {
		if !def.GlobalSupport && !slices.Contains(choices, def.ID) {
			continue
		}
		connected := h.actionCommands[def.ID].prepare != nil
		eligible := slices.Contains(result.AvailableActions, def.ID)
		scene.Support.Actions = append(scene.Support.Actions, SceneActionSupport{Action: def.ID, Connected: connected, EntryEligible: eligible})
		if !connected || !eligible {
			continue
		}
		action, err := h.contextAction(def.ID, templates[def.ID], result)
		if err != nil {
			return err
		}
		response.AvailableActions = append(response.AvailableActions, action)
	}
	return nil
}
