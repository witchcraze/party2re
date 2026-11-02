package http

import (
	"context"

	"github.com/witchcraze/party2re/internal/chapel"
	"github.com/witchcraze/party2re/internal/playercontext"
)

type ChapelSceneData struct {
	Parent   string                   `json:"parent"`
	Blessing chapel.CharacterBlessing `json:"blessing"`
	Status   chapel.ChapelStatus      `json:"status"`
}

func (h *Handler) chapelSceneData(ctx context.Context, r playercontext.Result) (any, error) {
	if h.chapel == nil {
		return nil, errSceneNotConfigured
	}
	actor := r.Snapshot.Character
	blessing, err := h.chapel.GetBlessing(ctx, actor.ID)
	if err != nil {
		return nil, err
	}
	status, err := h.chapel.GetStatus(ctx, actor.ID, actor.Name)
	if err != nil {
		return nil, err
	}
	return ChapelSceneData{Parent: "town", Blessing: blessing, Status: status}, nil
}
