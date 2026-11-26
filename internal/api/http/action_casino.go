package http

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/witchcraze/party2re/internal/casino"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/placelog"
)

// CasinoService is the Casino surface used by its scenes and commands.
type CasinoService interface {
	GetAccount(ctx context.Context, characterID string) (casino.Account, error)
	ExchangeGoldToCoins(ctx context.Context, characterID string, coins int64) (casino.Account, corecharacter.Character, error)
	ExchangePrize(ctx context.Context, characterID string, costCoins int64, count int) (casino.PrizeExchangeResult, error)
	SpinSlot(ctx context.Context, characterID string, bet int64) (casino.SpinResult, casino.Account, error)
	casinoRoomService
}

type casinoRoomService interface {
	ListRooms(ctx context.Context) ([]casino.RoomSummary, error)
	CreateRoom(ctx context.Context, characterID string, req casino.CreateRoomRequest) (*casino.RoomDetail, error)
	GetRoomView(ctx context.Context, roomID, playerID, characterID string) (*casino.RoomView, error)
	JoinRoom(ctx context.Context, roomID string, characterID string, password string, fatigue int) (*casino.RoomDetail, error)
	SpectateRoom(ctx context.Context, roomID string, characterID string, password string) (*casino.RoomDetail, error)
	LeaveRoom(ctx context.Context, roomID string, characterID string) error
	KickMember(ctx context.Context, roomID string, leaderID string, targetID string) (casino.KickResult, error)
	RecoverCharacterDeparture(ctx context.Context, characterID string) error
	StartGame(ctx context.Context, roomID string, leaderID string) (*casino.RoomDetail, error)
	PlayRoomAction(ctx context.Context, roomID string, characterID string, action string) (*casino.RoomDetail, error)
	RenewRoom(ctx context.Context, roomID, characterID string) error
}

type casinoSlotParams struct {
	Bet int64 `json:"bet"`
}

type casinoExchangeParams struct {
	Coins int64 `json:"coins"`
}

type casinoPrizeExchangeParams struct {
	CostCoins int64 `json:"cost_coins"`
	Count     int   `json:"count"`
}

type casinoRoomCreateParams struct {
	Name            string           `json:"name"`
	GameType        casino.GameType  `json:"game_type"`
	Speed           casino.RoomSpeed `json:"speed"`
	MaxPlayers      int              `json:"max_players"`
	Rate            int64            `json:"rate"`
	Password        string           `json:"password"`
	AllowSpectators bool             `json:"allow_spectators"`
}

type casinoRoomEntryParams struct {
	RoomID   string `json:"room_id"`
	Password string `json:"password"`
}

type casinoRoomKickParams struct {
	RoomID            string `json:"room_id"`
	TargetCharacterID string `json:"target_character_id"`
}

type casinoRoomPlayParams struct {
	RoomID string `json:"room_id"`
	Action string `json:"action"`
}

// Room commands return only the room; members' cards and marks are observed
// through the masked activity scene.
type casinoRoomResult struct {
	RoomID string `json:"room_id"`
}

type casinoSlotResult struct {
	casino.SpinResult
	Coins   int64  `json:"coins"`
	Message string `json:"message"`
}

type casinoExchangeResult struct {
	Coins     int64  `json:"coins"`
	Gold      int    `json:"gold"`
	NPCSpeech string `json:"npc_speech"`
}

type casinoPrizeExchangeResult struct {
	casino.PrizeExchangeResult
	NPCSpeech string `json:"npc_speech"`
}

// WithCasino configures the casino scenes and commands.
func WithCasino(service CasinoService) Option {
	return func(h *Handler) {
		h.casino = service
		if service == nil {
			withActionCommand[casinoSlotParams]("casino_slot", nil, nil)(h)
			withActionCommand[casinoExchangeParams]("casino_exchange", nil, nil)(h)
			withActionCommand[casinoPrizeExchangeParams]("casino_prize_exchange", nil, nil)(h)
			withActionCommand[casinoRoomCreateParams]("casino_room_create", nil, nil)(h)
			withActionCommand[casinoRoomEntryParams]("casino_room_join", nil, nil)(h)
			withActionCommand[casinoRoomEntryParams]("casino_room_spectate", nil, nil)(h)
			withActionCommand[casinoRoomPlayParams]("casino_room_action", nil, nil)(h)
			withActionCommand[casinoRoomKickParams]("casino_room_kick", nil, nil)(h)
			withActionCommand[CasinoRoomParams]("casino_room_leave", nil, nil)(h)
			withActionCommand[CasinoRoomParams]("casino_room_start", nil, nil)(h)
			return
		}
		withActionCommand("casino_slot", func(ctx context.Context, actor string, p casinoSlotParams) (any, error) {
			spin, account, err := service.SpinSlot(ctx, actor, p.Bet)
			if err != nil {
				return nil, err
			}
			return casinoSlotResult{SpinResult: spin, Coins: account.Coins, Message: casinoSlotMessage(spin)}, nil
		}, casinoActionRejection)(h)
		withActionCommand("casino_exchange", func(ctx context.Context, actor string, p casinoExchangeParams) (any, error) {
			account, char, err := service.ExchangeGoldToCoins(ctx, actor, p.Coins)
			if err != nil {
				return nil, err
			}
			return casinoExchangeResult{Coins: account.Coins, Gold: char.Money, NPCSpeech: fmt.Sprintf("%d枚のコインと両替しました", p.Coins)}, nil
		}, casinoActionRejection)(h)
		withActionCommand("casino_prize_exchange", func(ctx context.Context, actor string, p casinoPrizeExchangeParams) (any, error) {
			// Read the name first: a failed read after the exchange would hide a committed outcome.
			char, err := h.characters.Get(ctx, actor)
			if err != nil {
				return nil, err
			}
			res, err := service.ExchangePrize(ctx, actor, p.CostCoins, p.Count)
			if err != nil {
				return nil, err
			}
			speech := fmt.Sprintf("コイン%d枚の賞品%d個と交換ですね！%sは%sの預かり所に送っておきました", res.Prize.CostCoins, res.Count, res.Prize.ItemName, char.Name)
			return casinoPrizeExchangeResult{PrizeExchangeResult: res, NPCSpeech: speech}, nil
		}, casinoActionRejection)(h)
		// Entering a room leaves the lobby's member list (casino.cgi:376,469,513).
		enter := func(ctx context.Context, actor string, room *casino.RoomDetail, err error) (any, error) {
			if err == nil {
				h.leavePlace(ctx, actor)
			}
			return casinoRoom(room, err)
		}
		withActionCommand("casino_room_create", func(ctx context.Context, actor string, p casinoRoomCreateParams) (any, error) {
			room, err := service.CreateRoom(ctx, actor, casino.CreateRoomRequest{
				Name: p.Name, GameType: p.GameType, Speed: p.Speed, MaxPlayers: p.MaxPlayers, Rate: p.Rate, Password: p.Password, AllowSpectators: p.AllowSpectators,
			})
			return enter(ctx, actor, room, err)
		}, casinoActionRejection)(h)
		withActionCommand("casino_room_join", func(ctx context.Context, actor string, p casinoRoomEntryParams) (any, error) {
			char, err := h.characters.Get(ctx, actor)
			if err != nil {
				return nil, err
			}
			room, err := service.JoinRoom(ctx, p.RoomID, actor, p.Password, char.Tired)
			return enter(ctx, actor, room, err)
		}, casinoActionRejection)(h)
		withActionCommand("casino_room_spectate", func(ctx context.Context, actor string, p casinoRoomEntryParams) (any, error) {
			room, err := service.SpectateRoom(ctx, p.RoomID, actor, p.Password)
			return enter(ctx, actor, room, err)
		}, casinoActionRejection)(h)
		withActionCommand("casino_room_start", func(ctx context.Context, actor string, p CasinoRoomParams) (any, error) {
			return casinoRoom(service.StartGame(ctx, p.RoomID, actor))
		}, casinoActionRejection)(h)
		withActionCommand("casino_room_action", func(ctx context.Context, actor string, p casinoRoomPlayParams) (any, error) {
			return casinoRoom(service.PlayRoomAction(ctx, p.RoomID, actor, p.Action))
		}, casinoActionRejection)(h)
		withActionCommand("casino_room_kick", func(ctx context.Context, actor string, p casinoRoomKickParams) (any, error) {
			res, err := service.KickMember(ctx, p.RoomID, actor, p.TargetCharacterID)
			if err != nil {
				return nil, err
			}
			h.appendKickPlaceLog(ctx, placelog.Place{Kind: placelog.KindFacility, ID: "casino"}, res.RoomName, res.LeaderName, p.TargetCharacterID)
			return casinoRoomResult{RoomID: p.RoomID}, nil
		}, casinoActionRejection)(h)
		withActionCommand("casino_room_leave", func(ctx context.Context, actor string, p CasinoRoomParams) (any, error) {
			return casinoRoomResult{RoomID: p.RoomID}, service.LeaveRoom(ctx, p.RoomID, actor)
		}, casinoActionRejection)(h)
	}
}

func casinoRoom(room *casino.RoomDetail, err error) (any, error) {
	if err != nil {
		return nil, err
	}
	return casinoRoomResult{RoomID: room.Room.ID}, nil
}

var casinoSlotMarks = map[casino.SlotSymbol]string{
	casino.SymbolCherry: "∞", casino.SymbolNote: "♪", casino.SymbolDagger: "†", casino.SymbolStar: "★", casino.SymbolSeven: "７",
}

// casinoSlotMessage is the ＠＄Nすろっと reply (casino.cgi slot).
func casinoSlotMessage(spin casino.SpinResult) string {
	lines := []string{fmt.Sprintf("$%dスロット", spin.BetCoins), "【" + casinoSlotMarks[spin.Reels[0]] + "】【" + casinoSlotMarks[spin.Reels[1]] + "】【" + casinoSlotMarks[spin.Reels[2]] + "】"}
	switch {
	case !spin.IsWin:
		lines = append(lines, "ハズレ")
	case spin.Reels[0] == spin.Reels[2] && spin.Reels[1] == spin.Reels[2]:
		lines = append(lines, "なんと!! "+casinoSlotMarks[spin.Reels[0]]+" が3つそろいました!!", "おめでとうございます!!", fmt.Sprintf("***** コイン %d 枚 GET !! *****", spin.PayoutCoins-spin.BonusCoins))
	default:
		lines = append(lines, "チェリーが2つそろいました♪", fmt.Sprintf("コイン %d 枚Up♪", spin.PayoutCoins-spin.BonusCoins))
	}
	if spin.BonusCoins > 0 {
		lines = append(lines, "コインがちょっとおまけされた！")
	}
	return strings.Join(lines, "\n")
}

func casinoActionRejection(err error) (int, ErrorDetail) {
	type rejection struct {
		status int
		code   string
	}
	for target, r := range map[error]rejection{
		corecharacter.ErrNotFound:          {http.StatusNotFound, "CHARACTER_NOT_FOUND"},
		casino.ErrRoomNotFound:             {http.StatusNotFound, "CASINO_ROOM_NOT_FOUND"},
		casino.ErrMemberNotFound:           {http.StatusNotFound, "CASINO_MEMBER_NOT_FOUND"},
		casino.ErrInvalidPassword:          {http.StatusForbidden, "CASINO_INVALID_PASSWORD"},
		casino.ErrSpectatorsNotAllowed:     {http.StatusForbidden, "CASINO_SPECTATORS_NOT_ALLOWED"},
		casino.ErrNotLeader:                {http.StatusForbidden, "CASINO_NOT_LEADER"},
		casino.ErrCannotKickSelf:           {http.StatusForbidden, "CASINO_CANNOT_KICK_SELF"},
		casino.ErrSpectatorCannot:          {http.StatusForbidden, "CASINO_SPECTATOR"},
		casino.ErrAlreadyActed:             {http.StatusConflict, "CASINO_ALREADY_ACTED"},
		casino.ErrGameInProgress:           {http.StatusConflict, "CASINO_GAME_IN_PROGRESS"},
		casino.ErrGameNotInRound:           {http.StatusConflict, "CASINO_GAME_NOT_IN_ROUND"},
		casino.ErrRoomNameTaken:            {http.StatusConflict, "CASINO_ROOM_NAME_TAKEN"},
		casino.ErrRoomFull:                 {http.StatusConflict, "CASINO_ROOM_FULL"},
		casino.ErrAlreadyInRoom:            {http.StatusConflict, "CASINO_ALREADY_IN_ROOM"},
		casino.ErrNotEnoughPlayers:         {http.StatusConflict, "CASINO_NOT_ENOUGH_PLAYERS"},
		casino.ErrCharacterExhausted:       {http.StatusConflict, "CASINO_EXHAUSTED"},
		casino.ErrInsufficientCoins:        {http.StatusUnprocessableEntity, "CASINO_INSUFFICIENT_COINS"},
		casino.ErrInsufficientCoinsForRate: {http.StatusUnprocessableEntity, "CASINO_INSUFFICIENT_COINS"},
		casino.ErrInsufficientGold:         {http.StatusUnprocessableEntity, "CASINO_INSUFFICIENT_GOLD"},
		casino.ErrJobNotEligibleForSlot200: {http.StatusUnprocessableEntity, "CASINO_SLOT_JOB_REQUIRED"},
		casino.ErrInvalidAmount:            {http.StatusBadRequest, "CASINO_INVALID_INPUT"},
		casino.ErrCoinOverflow:             {http.StatusBadRequest, "CASINO_INVALID_INPUT"},
		casino.ErrInvalidBetRate:           {http.StatusBadRequest, "CASINO_INVALID_INPUT"},
		casino.ErrPrizeNotFound:            {http.StatusBadRequest, "CASINO_INVALID_INPUT"},
		casino.ErrInvalidCount:             {http.StatusBadRequest, "CASINO_INVALID_INPUT"},
		casino.ErrInvalidRoomName:          {http.StatusBadRequest, "CASINO_INVALID_INPUT"},
		casino.ErrInvalidGameType:          {http.StatusBadRequest, "CASINO_INVALID_INPUT"},
		casino.ErrInvalidSpeed:             {http.StatusBadRequest, "CASINO_INVALID_INPUT"},
		casino.ErrInvalidMaxPlayers:        {http.StatusBadRequest, "CASINO_INVALID_INPUT"},
		casino.ErrInvalidRate:              {http.StatusBadRequest, "CASINO_INVALID_INPUT"},
		casino.ErrInvalidAction:            {http.StatusBadRequest, "CASINO_INVALID_INPUT"},
		casino.ErrInvalidHighLowAction:     {http.StatusBadRequest, "CASINO_INVALID_INPUT"},
		casino.ErrLowNotAllowedTwo:         {http.StatusBadRequest, "CASINO_INVALID_INPUT"},
		casino.ErrHighLowCallUnavailable:   {http.StatusBadRequest, "CASINO_INVALID_INPUT"},
		casino.ErrInvalidDoppelMark:        {http.StatusBadRequest, "CASINO_INVALID_INPUT"},
		casino.ErrInvalidDoppelMarkIndex:   {http.StatusBadRequest, "CASINO_INVALID_INPUT"},
	} {
		if errors.Is(err, target) {
			return r.status, ErrorDetail{Code: r.code, Message: err.Error()}
		}
	}
	return 0, ErrorDetail{}
}
