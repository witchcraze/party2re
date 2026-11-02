package http

import (
	"context"
	"slices"

	"github.com/witchcraze/party2re/internal/dungeon"
	"github.com/witchcraze/party2re/internal/playercontext"
)

func (h *Handler) dungeonActivityData(ctx context.Context, result playercontext.Result, fact playercontext.Activity) (*dungeon.ExpeditionObservation, error) {
	if h.dungeons == nil {
		return nil, errSceneNotConfigured
	}
	view, err := h.dungeons.ObserveExpedition(ctx, result.Snapshot.Character.ID)
	if err != nil {
		return nil, err
	}
	role := "member"
	if view != nil && view.LeaderID == result.Snapshot.Character.ID {
		role = "actor"
	}
	// Never combine an old activity's eligibility with another run or revision.
	if view == nil || view.ID != fact.ID || view.Revision != fact.Revision || view.PartyID != fact.PartyID || string(view.Status) != fact.Phase || view.CurrentFloor != fact.Round || role != fact.Role ||
		view.Controls.CanMove != slices.Contains(fact.Actions, "dungeon_move") || view.Controls.CanFight != slices.Contains(fact.Actions, "dungeon_fight") || view.Controls.CanEscape != slices.Contains(fact.Actions, "dungeon_escape") {
		return nil, dungeon.ErrExpeditionStateChanged
	}
	view.Controls.CanMove = view.Controls.CanMove && slices.Contains(result.AvailableActions, "dungeon_move")
	view.Controls.CanFight = view.Controls.CanFight && slices.Contains(result.AvailableActions, "dungeon_fight")
	view.Controls.CanEscape = view.Controls.CanEscape && slices.Contains(result.AvailableActions, "dungeon_escape")
	if !view.Controls.CanMove {
		view.Directions = []dungeon.Direction{}
	}
	return view, nil
}
