package http

import (
	"context"

	"github.com/witchcraze/party2re/internal/exile"
	"github.com/witchcraze/party2re/internal/playercontext"
)

type ExileSceneData struct {
	Parent string      `json:"parent"`
	Board  exile.Board `json:"board"`
}

func (h *Handler) exileSceneData(ctx context.Context, r playercontext.Result) (any, error) {
	if h.exile == nil {
		return nil, errSceneNotConfigured
	}
	board, err := h.exile.GetBoard(ctx, r.Snapshot.Character.ID)
	if err != nil {
		return nil, err
	}
	return ExileSceneData{Parent: "town", Board: board}, nil
}

var exileFacility = facility{
	title: "荒らし追放騎士団", commands: []string{"exile_petition", "exile_approve", "exile_oppose"}, read: (*Handler).exileSceneData,
}
