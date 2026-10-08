package http

import (
	"context"
	"errors"
	"slices"

	"github.com/witchcraze/party2re/internal/casino"
	"github.com/witchcraze/party2re/internal/playercontext"
)

type CasinoMenu struct {
	Coins        int64          `json:"coins"`
	GoldPerCoin  int            `json:"gold_per_coin"`
	SlotBetRates []int64        `json:"slot_bet_rates"`
	Prizes       []casino.Prize `json:"prizes"`
}

type CasinoLobbyRoom struct {
	casino.RoomSummary
	SelectParams playercontext.Subject `json:"select_params"`
}

type CasinoLobbySceneData struct {
	Parent      string            `json:"parent"`
	Menu        CasinoMenu        `json:"menu"`
	Rooms       []CasinoLobbyRoom `json:"rooms"`
	Page        ScenePage         `json:"page"`
	WindowLimit int               `json:"window_limit"`
}

type CasinoRoomParams struct {
	RoomID string `json:"room_id"`
}

// Selected rooms expose public lobby facts only. Admission is an explicit command.
type CasinoSelectedSceneData struct {
	Parent         string             `json:"parent"`
	Menu           CasinoMenu         `json:"menu"`
	Room           casino.RoomSummary `json:"room"`
	JoinParams     CasinoRoomParams   `json:"join_params"`
	SpectateParams *CasinoRoomParams  `json:"spectate_params"`
}

type CasinoRoomSceneData struct {
	Coins    int64               `json:"coins"`
	View     casino.RoomView     `json:"view"`
	Controls casino.RoomControls `json:"controls"`
}

func (h *Handler) casinoSceneData(ctx context.Context, r playercontext.Result) (any, error) {
	if h.casino == nil {
		return nil, errSceneNotConfigured
	}
	account, err := h.casino.GetAccount(ctx, r.Snapshot.Character.ID)
	if err != nil {
		return nil, err
	}
	menu := CasinoMenu{Coins: account.Coins, GoldPerCoin: casino.GoldPerCoin, SlotBetRates: casino.SlotBetRates(r.Snapshot.Character.JobID), Prizes: casino.GetPrizes()}
	rooms, err := h.casino.ListRooms(ctx)
	if err != nil {
		return nil, err
	}
	n := r.Navigation.Selection
	if n.Subject != (playercontext.Subject{}) {
		for _, room := range rooms {
			if room.ID != n.Subject.ID {
				continue
			}
			data := CasinoSelectedSceneData{Parent: "town", Menu: menu, Room: room, JoinParams: CasinoRoomParams{RoomID: room.ID}}
			if room.AllowSpectators {
				data.SpectateParams = &CasinoRoomParams{RoomID: room.ID}
			}
			return data, nil
		}
		return nil, playercontext.ErrSelectionNotFound
	}
	limit := n.Limit
	if limit == 0 {
		limit = 20
	}
	start, end := min(n.Offset, len(rooms)), min(n.Offset+limit, len(rooms))
	page := ScenePage{Mode: "offset", Offset: n.Offset, Limit: limit}
	if end < len(rooms) {
		page.Next = &playercontext.PageParams{Destination: "casino", Offset: end, Limit: limit}
	}
	rows := make([]CasinoLobbyRoom, 0, end-start)
	for _, room := range rooms[start:end] {
		rows = append(rows, CasinoLobbyRoom{RoomSummary: room, SelectParams: playercontext.Subject{Kind: "room", ID: room.ID}})
	}
	return CasinoLobbySceneData{Parent: "town", Menu: menu, Rooms: rows, Page: page, WindowLimit: 100}, nil
}

func (h *Handler) casinoActivityData(ctx context.Context, result playercontext.Result, fact playercontext.Activity) (*CasinoRoomSceneData, error) {
	if h.casino == nil {
		return nil, errSceneNotConfigured
	}
	actor := result.Snapshot.Character
	account, err := h.casino.GetAccount(ctx, actor.ID)
	if err != nil {
		return nil, err
	}
	view, err := h.casino.GetRoomView(ctx, fact.ID, actor.PlayerID, actor.ID)
	if err != nil {
		return nil, err
	}
	controls, err := view.Controls(actor.ID)
	if err != nil {
		return nil, err
	}
	// Membership/phase can change between the activity query and detail read.
	// Fail the observation instead of combining stale eligibility with new facts.
	if view.Room.ID != fact.ID || string(view.Room.Status) != fact.Phase || view.Room.Round != fact.Round || controls.Role != fact.Role || controls.CanStart != slices.Contains(fact.Actions, "casino_room_start") || (len(controls.Actions) > 0) != slices.Contains(fact.Actions, "casino_room_action") {
		return nil, errors.New("casino activity changed during observation")
	}
	if !slices.Contains(result.AvailableActions, "casino_room_start") {
		controls.CanStart = false
		controls.KickTargets = []string{}
	}
	if !slices.Contains(result.AvailableActions, "casino_room_action") {
		controls.Actions = []string{}
	}
	return &CasinoRoomSceneData{Coins: account.Coins, View: *view, Controls: controls}, nil
}
