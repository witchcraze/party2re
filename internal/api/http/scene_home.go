package http

import (
	"context"

	"github.com/witchcraze/party2re/internal/home"
	"github.com/witchcraze/party2re/internal/playercontext"
)

type HomeSceneData struct {
	Parent       string             `json:"parent"`
	View         home.HomeView      `json:"view"`
	Destinations []SceneDestination `json:"destinations"`
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
	target := actor.ID
	if r.Navigation != nil && r.Navigation.Selection.Subject != (playercontext.Subject{}) {
		target = r.Navigation.Selection.Subject.ID
	}
	view, err := h.homes.GetHomeView(ctx, target, actor.ID, actor.PlayerID)
	if err != nil {
		return nil, err
	}
	data := HomeSceneData{Parent: "town", View: view, Destinations: []SceneDestination{}}
	if view.IsOwner {
		for _, id := range []string{"home_inbox", "home_outbox"} {
			if adapter, ok := h.sceneAdapters[id]; ok {
				data.Destinations = append(data.Destinations, SceneDestination{ID: id, Title: adapter.title, Supported: adapter.read != nil, EnterParams: sceneEnterParams{Destination: id}})
			}
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
