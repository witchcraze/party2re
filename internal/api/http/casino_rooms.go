package http

import (
	"errors"
	"net/http"

	"github.com/witchcraze/party2re/internal/casino"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
)

type listCasinoRoomsResponse struct {
	Rooms []casino.RoomDetail `json:"rooms"`
}

type createCasinoRoomRequest struct {
	Name            string           `json:"name"`
	GameType        casino.GameType  `json:"game_type"`
	Speed           casino.RoomSpeed `json:"speed"`
	MaxPlayers      int              `json:"max_players"`
	Rate            int64            `json:"rate"`
	Password        string           `json:"password,omitempty"`
	AllowSpectators bool             `json:"allow_spectators"`
}

type casinoRoomResponse struct {
	Room casino.RoomDetail `json:"room"`
}

type joinCasinoRoomRequest struct {
	Password string `json:"password,omitempty"`
}

type kickCasinoRoomMemberRequest struct {
	TargetCharacterID string `json:"target_character_id"`
}

type casinoRoomActionRequest struct {
	Action casino.Action `json:"action"` // "call", "showdown", "fold"
}

func (h *Handler) handleListCasinoRooms(w http.ResponseWriter, r *http.Request) {
	if h.casino == nil {
		writeError(w, http.StatusNotImplemented, errors.New("casino service not configured"))
		return
	}

	rooms, err := h.casino.ListRooms(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if rooms == nil {
		rooms = []casino.RoomDetail{}
	}

	writeJSON(w, http.StatusOK, listCasinoRoomsResponse{
		Rooms: rooms,
	})
}

func (h *Handler) handleCreateCasinoRoom(w http.ResponseWriter, r *http.Request) {
	if h.casino == nil {
		writeError(w, http.StatusNotImplemented, errors.New("casino service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		var req createCasinoRoomRequest
		if !decodeJSON(w, r, &req) {
			return
		}

		roomReq := casino.CreateRoomRequest{
			Name:            req.Name,
			GameType:        req.GameType,
			Speed:           req.Speed,
			MaxPlayers:      req.MaxPlayers,
			Rate:            req.Rate,
			Password:        req.Password,
			AllowSpectators: req.AllowSpectators,
		}

		room, err := h.casino.CreateRoom(r.Context(), char.ID, roomReq)
		if err != nil {
			if errors.Is(err, casino.ErrInsufficientCoinsForRate) {
				writeError(w, http.StatusUnprocessableEntity, err)
				return
			}
			if errors.Is(err, casino.ErrRoomNameTaken) {
				writeError(w, http.StatusConflict, err)
				return
			}
			if errors.Is(err, casino.ErrInvalidRoomName) || errors.Is(err, casino.ErrInvalidGameType) ||
				errors.Is(err, casino.ErrInvalidSpeed) || errors.Is(err, casino.ErrInvalidMaxPlayers) ||
				errors.Is(err, casino.ErrInvalidRate) {
				writeError(w, http.StatusBadRequest, err)
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		writeJSON(w, http.StatusCreated, casinoRoomResponse{
			Room: *room,
		})
	})
}

func (h *Handler) handleGetCasinoRoom(w http.ResponseWriter, r *http.Request) {
	if h.casino == nil {
		writeError(w, http.StatusNotImplemented, errors.New("casino service not configured"))
		return
	}

	roomID := r.PathValue("roomId")
	viewingCharID := r.URL.Query().Get("character_id")

	room, err := h.casino.GetRoomDetail(r.Context(), roomID, viewingCharID)
	if err != nil {
		if errors.Is(err, casino.ErrRoomNotFound) {
			writeError(w, http.StatusNotFound, err)
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusOK, casinoRoomResponse{
		Room: *room,
	})
}

func (h *Handler) handleJoinCasinoRoom(w http.ResponseWriter, r *http.Request) {
	if h.casino == nil {
		writeError(w, http.StatusNotImplemented, errors.New("casino service not configured"))
		return
	}

	charID := r.PathValue("id")
	roomID := r.PathValue("roomId")

	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		var req joinCasinoRoomRequest
		_ = decodeJSON(w, r, &req)

		room, err := h.casino.JoinRoom(r.Context(), roomID, char.ID, req.Password, char.Tired)
		if err != nil {
			if errors.Is(err, casino.ErrRoomNotFound) {
				writeError(w, http.StatusNotFound, err)
				return
			}
			if errors.Is(err, casino.ErrInvalidPassword) {
				writeError(w, http.StatusForbidden, err)
				return
			}
			if errors.Is(err, casino.ErrCharacterExhausted) || errors.Is(err, casino.ErrInsufficientCoins) ||
				errors.Is(err, casino.ErrRoomFull) || errors.Is(err, casino.ErrAlreadyInRoom) {
				writeError(w, http.StatusUnprocessableEntity, err)
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		writeJSON(w, http.StatusOK, casinoRoomResponse{
			Room: *room,
		})
	})
}

func (h *Handler) handleSpectateCasinoRoom(w http.ResponseWriter, r *http.Request) {
	if h.casino == nil {
		writeError(w, http.StatusNotImplemented, errors.New("casino service not configured"))
		return
	}

	charID := r.PathValue("id")
	roomID := r.PathValue("roomId")

	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		var req joinCasinoRoomRequest
		_ = decodeJSON(w, r, &req)

		room, err := h.casino.SpectateRoom(r.Context(), roomID, char.ID, req.Password)
		if err != nil {
			if errors.Is(err, casino.ErrRoomNotFound) {
				writeError(w, http.StatusNotFound, err)
				return
			}
			if errors.Is(err, casino.ErrInvalidPassword) || errors.Is(err, casino.ErrSpectatorsNotAllowed) {
				writeError(w, http.StatusForbidden, err)
				return
			}
			if errors.Is(err, casino.ErrAlreadyInRoom) {
				writeError(w, http.StatusUnprocessableEntity, err)
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		writeJSON(w, http.StatusOK, casinoRoomResponse{
			Room: *room,
		})
	})
}

func (h *Handler) handleLeaveCasinoRoom(w http.ResponseWriter, r *http.Request) {
	if h.casino == nil {
		writeError(w, http.StatusNotImplemented, errors.New("casino service not configured"))
		return
	}

	charID := r.PathValue("id")
	roomID := r.PathValue("roomId")

	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		err := h.casino.LeaveRoom(r.Context(), roomID, char.ID)
		if err != nil {
			if errors.Is(err, casino.ErrRoomNotFound) || errors.Is(err, casino.ErrMemberNotFound) {
				writeError(w, http.StatusNotFound, err)
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{"success": true})
	})
}

func (h *Handler) handleKickCasinoRoomMember(w http.ResponseWriter, r *http.Request) {
	if h.casino == nil {
		writeError(w, http.StatusNotImplemented, errors.New("casino service not configured"))
		return
	}

	charID := r.PathValue("id")
	roomID := r.PathValue("roomId")

	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		var req kickCasinoRoomMemberRequest
		if !decodeJSON(w, r, &req) {
			return
		}

		err := h.casino.KickMember(r.Context(), roomID, char.ID, req.TargetCharacterID)
		if err != nil {
			if errors.Is(err, casino.ErrRoomNotFound) || errors.Is(err, casino.ErrMemberNotFound) {
				writeError(w, http.StatusNotFound, err)
				return
			}
			if errors.Is(err, casino.ErrNotLeader) || errors.Is(err, casino.ErrCannotKickSelf) {
				writeError(w, http.StatusForbidden, err)
				return
			}
			if errors.Is(err, casino.ErrGameInProgress) {
				writeError(w, http.StatusConflict, err)
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{"success": true})
	})
}

func (h *Handler) handleStartCasinoIndianPoker(w http.ResponseWriter, r *http.Request) {
	if h.casino == nil {
		writeError(w, http.StatusNotImplemented, errors.New("casino service not configured"))
		return
	}

	charID := r.PathValue("id")
	roomID := r.PathValue("roomId")

	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		room, err := h.casino.StartIndianPoker(r.Context(), roomID, char.ID)
		if err != nil {
			if errors.Is(err, casino.ErrRoomNotFound) {
				writeError(w, http.StatusNotFound, err)
				return
			}
			if errors.Is(err, casino.ErrNotLeader) {
				writeError(w, http.StatusForbidden, err)
				return
			}
			if errors.Is(err, casino.ErrNotEnoughPlayers) {
				writeError(w, http.StatusUnprocessableEntity, err)
				return
			}
			if errors.Is(err, casino.ErrGameInProgress) {
				writeError(w, http.StatusConflict, err)
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		writeJSON(w, http.StatusOK, casinoRoomResponse{
			Room: *room,
		})
	})
}

func (h *Handler) handlePlayCasinoIndianPokerAction(w http.ResponseWriter, r *http.Request) {
	if h.casino == nil {
		writeError(w, http.StatusNotImplemented, errors.New("casino service not configured"))
		return
	}

	charID := r.PathValue("id")
	roomID := r.PathValue("roomId")

	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		var req casinoRoomActionRequest
		if !decodeJSON(w, r, &req) {
			return
		}

		if !req.Action.Valid() {
			writeError(w, http.StatusBadRequest, casino.ErrInvalidAction)
			return
		}

		room, err := h.casino.PlayIndianPokerAction(r.Context(), roomID, char.ID, req.Action)
		if err != nil {
			if errors.Is(err, casino.ErrRoomNotFound) || errors.Is(err, casino.ErrMemberNotFound) {
				writeError(w, http.StatusNotFound, err)
				return
			}
			if errors.Is(err, casino.ErrAlreadyActed) || errors.Is(err, casino.ErrSpectatorCannot) {
				writeError(w, http.StatusForbidden, err)
				return
			}
			if errors.Is(err, casino.ErrGameNotInRound) {
				writeError(w, http.StatusConflict, err)
				return
			}
			if errors.Is(err, casino.ErrInsufficientCoins) {
				writeError(w, http.StatusUnprocessableEntity, err)
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		writeJSON(w, http.StatusOK, casinoRoomResponse{
			Room: *room,
		})
	})
}
