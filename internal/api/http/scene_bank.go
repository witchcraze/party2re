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

var bankFacility = facility{
	title: "銀行", commands: []string{"bank_deposit", "bank_withdraw"}, read: (*Handler).bankSceneData, placeLog: true,
	npc: &npcProfile{name: bank.NPCName, words: []string{
		"ゴールドをお預かりいたします",
		"24時間いつでも、手数料もございません",
		"99999999999999 Gまでお預かりいたします",
	}},
}
