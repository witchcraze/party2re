package http

import (
	"context"

	"github.com/witchcraze/party2re/internal/home"
	"github.com/witchcraze/party2re/internal/playercontext"
)

type HomeNoticesSceneData struct {
	Parent  string                `json:"parent"`
	Notices []home.DeliveryNotice `json:"notices"`
	Page    ScenePage             `json:"page"`
}

func (h *Handler) homeNoticesSceneData(ctx context.Context, r playercontext.Result) (any, error) {
	if h.homes == nil {
		return nil, errSceneNotConfigured
	}
	n := r.Navigation.Selection
	limit := n.Limit
	if limit == 0 {
		limit = 50
	}
	cursor := ""
	if n.Cursor != nil {
		cursor = *n.Cursor
	}
	actor := r.Snapshot.Character
	page, err := h.homes.ListDeliveryNoticesPage(ctx, actor.ID, actor.PlayerID, limit, n.Offset, cursor)
	if err != nil {
		return nil, err
	}
	data := HomeNoticesSceneData{Parent: "home", Notices: append([]home.DeliveryNotice{}, page.Items...), Page: ScenePage{Mode: "cursor", Limit: page.Limit}}
	if page.HasMore {
		data.Page.Next = &playercontext.PageParams{Destination: n.Destination, Limit: page.Limit, Cursor: &page.NextCursor}
	}
	return data, nil
}

var homeNoticesFacility = facility{title: "お知らせ", commands: []string{"home_notice_ack"}, read: (*Handler).homeNoticesSceneData,
	present: func(_ *Handler, _ context.Context, _ playercontext.Result, data any) scenePresentation {
		return scenePresentation{next: data.(HomeNoticesSceneData).Page.Next}
	}}
