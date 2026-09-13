package http

import (
	"context"
	"errors"
	"net/http"

	"github.com/witchcraze/party2re/internal/casino"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/depot"
)

type CasinoAccountService interface {
	GetAccount(ctx context.Context, characterID string) (casino.Account, error)
	ExchangeGoldToCoins(ctx context.Context, characterID string, coins int64) (casino.Account, corecharacter.Character, error)
	ExchangeCoinsToGold(ctx context.Context, characterID string, coins int64) (casino.Account, corecharacter.Character, error)
	ExchangePrize(ctx context.Context, characterID string, costCoins int64, count int) (casino.PrizeExchangeResult, error)
}

type CasinoSoloGameService interface {
	SpinSlot(ctx context.Context, characterID string, bet int64) (casino.SpinResult, casino.Account, error)
	PlayHighLow(ctx context.Context, characterID string, betCoins int64, guess casino.GuessType) (casino.HighLowResult, casino.Account, error)
	PlayDoppel(ctx context.Context, characterID string, bet int64, poolSize int, playerMark casino.DoppelMark) (casino.DoppelResult, casino.Account, error)
}

type CasinoRoomService interface {
	ListRooms(ctx context.Context) ([]casino.RoomDetail, error)
	CreateRoom(ctx context.Context, characterID string, req casino.CreateRoomRequest) (*casino.RoomDetail, error)
	GetRoomDetail(ctx context.Context, roomID string, viewingCharID string) (*casino.RoomDetail, error)
	JoinRoom(ctx context.Context, roomID string, characterID string, password string, fatigue int) (*casino.RoomDetail, error)
	SpectateRoom(ctx context.Context, roomID string, characterID string, password string) (*casino.RoomDetail, error)
	LeaveRoom(ctx context.Context, roomID string, characterID string) error
	KickMember(ctx context.Context, roomID string, leaderID string, targetID string) error
	StartIndianPoker(ctx context.Context, roomID string, leaderID string) (*casino.RoomDetail, error)
	PlayIndianPokerAction(ctx context.Context, roomID string, characterID string, action casino.Action) (*casino.RoomDetail, error)
}

// CasinoService defines the casino operations exposed over HTTP.
type CasinoService interface {
	CasinoAccountService
	CasinoSoloGameService
	CasinoRoomService
}

// WithCasino configures the casino service for the Handler.
func WithCasino(c CasinoService) Option {
	return func(h *Handler) {
		h.casino = c
	}
}

type getCasinoAccountResponse struct {
	Account casino.Account `json:"account"`
}

type casinoExchangeRequest struct {
	Direction string `json:"direction"` // "gold_to_coins" or "coins_to_gold"
	Coins     int64  `json:"coins"`
}

type casinoExchangeResponse struct {
	Account   casino.Account    `json:"account"`
	Character characterResponse `json:"character"`
}

type casinoSlotRequest struct {
	Bet int64 `json:"bet"`
}

type casinoSlotResponse struct {
	Result  casino.SpinResult `json:"result"`
	Account casino.Account    `json:"account"`
}

type casinoHighLowRequest struct {
	Bet   int64  `json:"bet"`
	Guess string `json:"guess"` // "HIGH" or "LOW"
}

type casinoHighLowResponse struct {
	Result  casino.HighLowResult `json:"result"`
	Account casino.Account       `json:"account"`
}

type casinoDoppelRequest struct {
	Bet        int64  `json:"bet"`
	PoolSize   int    `json:"pool_size"`
	PlayerMark string `json:"player_mark"`
}

type casinoDoppelResponse struct {
	Result  casino.DoppelResult `json:"result"`
	Account casino.Account      `json:"account"`
}

type casinoPrizeExchangeRequest struct {
	CostCoins int64 `json:"cost_coins"`
	Count     int   `json:"count"`
}

type casinoPrizeExchangeResponse struct {
	Result casino.PrizeExchangeResult `json:"result"`
}

func (h *Handler) handleGetCasinoAccount(w http.ResponseWriter, r *http.Request) {
	if h.casino == nil {
		writeError(w, http.StatusNotImplemented, errors.New("casino service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		account, err := h.casino.GetAccount(r.Context(), char.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		writeJSON(w, http.StatusOK, getCasinoAccountResponse{
			Account: account,
		})
	})
}

func (h *Handler) handleCasinoExchange(w http.ResponseWriter, r *http.Request) {
	if h.casino == nil {
		writeError(w, http.StatusNotImplemented, errors.New("casino service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		var req casinoExchangeRequest
		if !decodeJSON(w, r, &req) {
			return
		}

		if req.Coins <= 0 {
			writeError(w, http.StatusBadRequest, casino.ErrInvalidAmount)
			return
		}

		var account casino.Account
		var updatedChar corecharacter.Character
		var err error

		if req.Direction == "coins_to_gold" {
			account, updatedChar, err = h.casino.ExchangeCoinsToGold(r.Context(), char.ID, req.Coins)
		} else {
			account, updatedChar, err = h.casino.ExchangeGoldToCoins(r.Context(), char.ID, req.Coins)
		}

		if err != nil {
			if errors.Is(err, casino.ErrInsufficientCoins) || errors.Is(err, casino.ErrInsufficientGold) {
				writeError(w, http.StatusUnprocessableEntity, err)
				return
			}
			if errors.Is(err, casino.ErrInvalidAmount) {
				writeError(w, http.StatusBadRequest, err)
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		writeJSON(w, http.StatusOK, casinoExchangeResponse{
			Account:   account,
			Character: toCharacterResponse(updatedChar),
		})
	})
}

func (h *Handler) handleCasinoSlot(w http.ResponseWriter, r *http.Request) {
	if h.casino == nil {
		writeError(w, http.StatusNotImplemented, errors.New("casino service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		var req casinoSlotRequest
		if !decodeJSON(w, r, &req) {
			return
		}

		if req.Bet <= 0 {
			writeError(w, http.StatusBadRequest, casino.ErrInvalidBetRate)
			return
		}

		res, account, err := h.casino.SpinSlot(r.Context(), char.ID, req.Bet)
		if err != nil {
			if errors.Is(err, casino.ErrInsufficientCoins) {
				writeError(w, http.StatusUnprocessableEntity, err)
				return
			}
			if errors.Is(err, casino.ErrInvalidBetRate) {
				writeError(w, http.StatusBadRequest, err)
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		writeJSON(w, http.StatusOK, casinoSlotResponse{
			Result:  res,
			Account: account,
		})
	})
}

func (h *Handler) handleCasinoHighLow(w http.ResponseWriter, r *http.Request) {
	if h.casino == nil {
		writeError(w, http.StatusNotImplemented, errors.New("casino service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		var req casinoHighLowRequest
		if !decodeJSON(w, r, &req) {
			return
		}

		if req.Bet <= 0 {
			writeError(w, http.StatusBadRequest, casino.ErrInvalidHighLowBet)
			return
		}

		guess := casino.GuessType(req.Guess)
		if guess != casino.GuessHigh && guess != casino.GuessLow {
			guess = casino.GuessHigh
		}

		res, account, err := h.casino.PlayHighLow(r.Context(), char.ID, req.Bet, guess)
		if err != nil {
			if errors.Is(err, casino.ErrInsufficientCoins) {
				writeError(w, http.StatusUnprocessableEntity, err)
				return
			}
			if errors.Is(err, casino.ErrInvalidHighLowBet) || errors.Is(err, casino.ErrInvalidGuess) {
				writeError(w, http.StatusBadRequest, err)
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		writeJSON(w, http.StatusOK, casinoHighLowResponse{
			Result:  res,
			Account: account,
		})
	})
}

func (h *Handler) handleCasinoDoppel(w http.ResponseWriter, r *http.Request) {
	if h.casino == nil {
		writeError(w, http.StatusNotImplemented, errors.New("casino service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		var req casinoDoppelRequest
		if !decodeJSON(w, r, &req) {
			return
		}

		if req.Bet <= 0 || req.PoolSize < 2 {
			writeError(w, http.StatusBadRequest, casino.ErrInvalidDoppelBet)
			return
		}

		mark := casino.DoppelMark(req.PlayerMark)
		res, account, err := h.casino.PlayDoppel(r.Context(), char.ID, req.Bet, req.PoolSize, mark)
		if err != nil {
			if errors.Is(err, casino.ErrInsufficientCoins) {
				writeError(w, http.StatusUnprocessableEntity, err)
				return
			}
			if errors.Is(err, casino.ErrInvalidDoppelBet) || errors.Is(err, casino.ErrInvalidPoolSize) || errors.Is(err, casino.ErrInvalidDoppelMark) {
				writeError(w, http.StatusBadRequest, err)
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		writeJSON(w, http.StatusOK, casinoDoppelResponse{
			Result:  res,
			Account: account,
		})
	})
}

func (h *Handler) handleGetCasinoPrizes(w http.ResponseWriter, r *http.Request) {
	prizes := casino.GetPrizes()
	writeJSON(w, http.StatusOK, map[string]any{"prizes": prizes})
}

func (h *Handler) handleCasinoExchangePrize(w http.ResponseWriter, r *http.Request) {
	if h.casino == nil {
		writeError(w, http.StatusNotImplemented, errors.New("casino service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		var req casinoPrizeExchangeRequest
		if !decodeJSON(w, r, &req) {
			return
		}
		if req.Count <= 0 {
			req.Count = 1
		}

		res, err := h.casino.ExchangePrize(r.Context(), char.ID, req.CostCoins, req.Count)
		if err != nil {
			if errors.Is(err, casino.ErrInsufficientCoins) || errors.Is(err, depot.ErrDepotFull) {
				writeError(w, http.StatusUnprocessableEntity, err)
				return
			}
			if errors.Is(err, casino.ErrPrizeNotFound) || errors.Is(err, casino.ErrInvalidCount) {
				writeError(w, http.StatusBadRequest, err)
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		writeJSON(w, http.StatusOK, casinoPrizeExchangeResponse{
			Result: res,
		})
	})
}
