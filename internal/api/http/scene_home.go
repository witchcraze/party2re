package http

import (
	"context"
	"slices"

	"github.com/witchcraze/party2re/internal/home"
	"github.com/witchcraze/party2re/internal/placelog"
	"github.com/witchcraze/party2re/internal/playercontext"
)

type HomeSceneData struct {
	Parent       string             `json:"parent"`
	View         home.HomeView      `json:"view"`
	Destinations []SceneDestination `json:"destinations"`
	// ColorSamples are the ＠からー sample palette, offered in the owner's home.
	ColorSamples []HomeColorSample `json:"color_samples"`
}

type HomeColorSample struct {
	Name  string `json:"name"`
	Color string `json:"color"`
}

// homeColorSamples is the ＠からー sample palette (home.cgi color).
var homeColorSamples = []HomeColorSample{
	{"レッド", "#FF3333"}, {"ピンク", "#FF33CC"}, {"オレンジ", "#FF9933"}, {"イエロー", "#FFFF33"},
	{"グリーン", "#33FF33"}, {"アクア", "#33CCFF"}, {"ブルー", "#6666FF"}, {"パープル", "#CC66FF"},
	{"グレイ", "#CCCCFF"}, {"ホワイト", "#FFFFFF"}, {"エメラルド", "#33FF99"},
}

// homeOwner is the home being observed: the selected subject, otherwise the actor's own.
func homeOwner(n *playercontext.NavigationObservation, actorID string) string {
	if n != nil && n.Selection.Subject != (playercontext.Subject{}) {
		return n.Selection.Subject.ID
	}
	return actorID
}

type HomeMailboxSceneData struct {
	Parent  string        `json:"parent"`
	Letters []home.Letter `json:"letters"`
	Page    ScenePage     `json:"page"`
	Total   *int          `json:"total,omitempty"`
}

func (h *Handler) homeSceneData(ctx context.Context, r playercontext.Result) (any, error) {
	if h.homes == nil {
		return nil, errSceneNotConfigured
	}
	actor := r.Snapshot.Character
	view, err := h.homes.GetHomeView(ctx, homeOwner(r.Navigation, actor.ID), actor.ID, actor.PlayerID)
	if err != nil {
		return nil, err
	}
	data := HomeSceneData{Parent: "town", View: view, Destinations: []SceneDestination{}, ColorSamples: []HomeColorSample{}}
	destinations := homeRecordScenes
	if view.IsOwner {
		data.ColorSamples = homeColorSamples
		destinations = append(slices.Clone(homeRecordScenes), "home_inbox", "home_outbox", "home_notices", "home_custom_skill")
		if h.playerContext.Enterable(ctx, r, "home_image") == nil {
			destinations = append(destinations, "home_image")
		}
	}
	for _, id := range destinations {
		if adapter, ok := h.sceneAdapters[id]; ok {
			data.Destinations = append(data.Destinations, SceneDestination{ID: id, Title: adapter.title, Supported: adapter.read != nil, EnterParams: sceneEnterParams{Destination: id}})
		}
	}
	return data, nil
}

func (h *Handler) homeMailboxSceneData(ctx context.Context, r playercontext.Result) (any, error) {
	if h.homes == nil {
		return nil, errSceneNotConfigured
	}
	n := r.Navigation.Selection
	limit := n.Limit
	if limit == 0 {
		limit = 20
	}
	actor := r.Snapshot.Character.ID
	data := HomeMailboxSceneData{Parent: "home", Letters: []home.Letter{}, Page: ScenePage{Mode: "offset", Offset: n.Offset, Limit: limit}}
	if n.Cursor != nil {
		read := h.homes.ListInboxByCursor
		if n.Destination == "home_outbox" {
			read = h.homes.ListOutboxByCursor
		}
		page, err := read(ctx, actor, limit, *n.Cursor)
		if err != nil {
			return nil, err
		}
		data.Letters = append(data.Letters, page.Items...)
		data.Page.Mode = "cursor"
		if page.HasMore {
			data.Page.Next = &playercontext.PageParams{Destination: n.Destination, Limit: page.Limit, Cursor: &page.NextCursor}
		}
	} else {
		read := h.homes.ListInbox
		if n.Destination == "home_outbox" {
			read = h.homes.ListOutbox
		}
		page, err := read(ctx, actor, limit, n.Offset)
		if err != nil {
			return nil, err
		}
		data.Letters = append(data.Letters, page.Items...)
		data.Total = &page.Total
		if page.Offset+len(page.Items) < page.Total {
			data.Page.Next = &playercontext.PageParams{Destination: n.Destination, Offset: page.Offset + page.Limit, Limit: page.Limit}
		}
	}
	return data, nil
}

var (
	// Legacy ＠ねる rests in the home being visited ($m{home}); only one's own
	// home registers the living actions (home.cgi:26-42,73-105).
	homeFacility = facility{title: "自宅", commands: homeOwnerCommands, subjectCommands: []string{"home_sleep"}, ownedSubjectCommands: homeOwnerCommands,
		read: (*Handler).homeSceneData, npc: &npcProfile{talk: homeTalk}, placeLog: true,
		place: func(_ *Handler, _ context.Context, r playercontext.Result) (placelog.Place, error) {
			return placelog.Place{Kind: "home", ID: homeOwner(r.Navigation, r.Snapshot.Character.ID)}, nil
		},
		present: func(_ *Handler, _ context.Context, _ playercontext.Result, data any) scenePresentation {
			view := data.(HomeSceneData).View
			title := view.Owner.Name + "の家"
			p := scenePresentation{title: &title}
			for _, pet := range view.ResidentPets {
				p.members = append(p.members, PlaceMember{Kind: "npc", Name: pet.DisplayName(), Color: npcColor})
			}
			return p
		},
		bind: func(n playercontext.Selection, actorID string) map[string]map[string]any {
			return map[string]map[string]any{"home_sleep": {"target_home_id": homeOwner(&playercontext.NavigationObservation{Selection: n}, actorID)}}
		}}
	homeOwnerCommands  = []string{"home_sleep", "home_color", "home_item_use", "home_phrase_teach", "home_phrase_forget", "home_notice_ack", "home_letter_send"}
	homeInboxFacility  = facility{title: "受信箱", commands: []string{"home_letter_read", "home_letter_delete"}, read: (*Handler).homeMailboxSceneData, present: homeMailboxPresentation}
	homeOutboxFacility = facility{title: "送信箱", commands: []string{"home_letter_delete"}, read: (*Handler).homeMailboxSceneData, present: homeMailboxPresentation}
)

func homeMailboxPresentation(_ *Handler, _ context.Context, _ playercontext.Result, data any) scenePresentation {
	return scenePresentation{next: data.(HomeMailboxSceneData).Page.Next}
}
