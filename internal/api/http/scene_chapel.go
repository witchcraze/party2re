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

var chapelFacility = facility{
	title: "礼拝堂", commands: []string{"chapel_pray"}, read: (*Handler).chapelSceneData, placeLog: true,
	npc: &npcProfile{name: "@シスター", words: []string{
		"{name}に神のご加護があらんことを", "待つだけでは祈りは叶いません…", "{name}が力を尽くすとき、祈りは叶うことでしょう",
	}},
}
