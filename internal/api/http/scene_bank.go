package http

import (
	"context"

	"github.com/witchcraze/party2re/internal/bank"
	"github.com/witchcraze/party2re/internal/playercontext"
)

type BankSceneData struct {
	Parent string `json:"parent"`
	bank.State
}

func (h *Handler) bankSceneData(ctx context.Context, r playercontext.Result) (any, error) {
	if h.bank == nil {
		return nil, errSceneNotConfigured
	}
	state, err := h.bank.GetState(ctx, r.Snapshot.Character.ID)
	if err != nil {
		return nil, err
	}
	return BankSceneData{Parent: "town", State: state}, nil
}
