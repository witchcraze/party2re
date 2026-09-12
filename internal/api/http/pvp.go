package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/pvp"
)

// PvPService defines Colosseum operations exposed over HTTP.
type PvPService interface {
	CreateRoom(ctx context.Context, leaderID string, req pvp.CreateRoomRequest) (pvp.RoomDetail, error)
	GetRoom(ctx context.Context, roomID string) (pvp.RoomDetail, error)
	ListRooms(ctx context.Context) ([]pvp.RoomSummary, error)
	JoinRoom(ctx context.Context, characterID string, roomID string, password string) (pvp.RoomDetail, error)
	LeaveRoom(ctx context.Context, characterID string, roomID string) error
	SelectTeam(ctx context.Context, characterID string, roomID string, teamColor string) (pvp.RoomDetail, error)
	StartMatch(ctx context.Context, leaderID string, roomID string) (pvp.RoomDetail, error)
	AdvanceRound(ctx context.Context, leaderID string, roomID string) (pvp.RoundResolution, error)
}

// WithPvP configures the pvp service for the Handler.
func WithPvP(p PvPService) Option {
	return func(h *Handler) {
		h.pvp = p
	}
}

// -------------------------------------------------------------------
// Request & Response Types
// -------------------------------------------------------------------

type pvpCreateRoomRequest struct {
	Name       string `json:"name"`
	Password   string `json:"password,omitempty"`
	Speed      int    `json:"speed,omitempty"`
	Stage      int    `json:"stage,omitempty"`
	MaxMembers int    `json:"max_members,omitempty"`
	TargetWins int    `json:"target_wins,omitempty"`
	Bet        int    `json:"bet"`
	NeedJoin   string `json:"need_join,omitempty"`
}

type pvpJoinRoomRequest struct {
	Password string `json:"password,omitempty"`
}

type pvpSelectTeamRequest struct {
	TeamColor string `json:"team_color"`
}

type pvpRoomsResponse struct {
	Rooms []pvp.RoomSummary `json:"rooms"`
}

type pvpMessageResponse struct {
	Message string `json:"message"`
}

// -------------------------------------------------------------------
// Colosseum Room HTTP Handlers
// -------------------------------------------------------------------

func (h *Handler) handleCreatePvPRoom(w http.ResponseWriter, r *http.Request) {
	if h.pvp == nil {
		writeError(w, http.StatusNotImplemented, errors.New("pvp service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		var req pvpCreateRoomRequest
		if !decodeJSON(w, r, &req) {
			return
		}

		domainReq := pvp.CreateRoomRequest{
			Name:       req.Name,
			Password:   req.Password,
			Speed:      req.Speed,
			Stage:      req.Stage,
			MaxMembers: req.MaxMembers,
			TargetWins: req.TargetWins,
			Bet:        req.Bet,
			NeedJoin:   req.NeedJoin,
		}

		room, err := h.pvp.CreateRoom(r.Context(), char.ID, domainReq)
		if err != nil {
			if errors.Is(err, pvp.ErrInvalidRoomName) || errors.Is(err, pvp.ErrInvalidMaxMembers) ||
				errors.Is(err, pvp.ErrInvalidTargetWins) || errors.Is(err, pvp.ErrInvalidBet) {
				writeError(w, http.StatusBadRequest, err)
				return
			}
			if errors.Is(err, pvp.ErrInsufficientBetFunds) || errors.Is(err, pvp.ErrAlreadyInRoom) ||
				errors.Is(err, pvp.ErrNeedJoinNotMet) {
				writeError(w, http.StatusUnprocessableEntity, err)
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		writeJSON(w, http.StatusCreated, room)
	})
}

func (h *Handler) handleListPvPRooms(w http.ResponseWriter, r *http.Request) {
	if h.pvp == nil {
		writeError(w, http.StatusNotImplemented, errors.New("pvp service not configured"))
		return
	}

	rooms, err := h.pvp.ListRooms(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if rooms == nil {
		rooms = []pvp.RoomSummary{}
	}

	writeJSON(w, http.StatusOK, pvpRoomsResponse{
		Rooms: rooms,
	})
}

func (h *Handler) handleGetPvPRoom(w http.ResponseWriter, r *http.Request) {
	if h.pvp == nil {
		writeError(w, http.StatusNotImplemented, errors.New("pvp service not configured"))
		return
	}

	roomID := r.PathValue("room_id")
	if roomID == "" {
		writeError(w, http.StatusBadRequest, errors.New("room_id is required"))
		return
	}

	room, err := h.pvp.GetRoom(r.Context(), roomID)
	if err != nil {
		if errors.Is(err, pvp.ErrRoomNotFound) {
			writeError(w, http.StatusNotFound, err)
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusOK, room)
}

func (h *Handler) handleJoinPvPRoom(w http.ResponseWriter, r *http.Request) {
	if h.pvp == nil {
		writeError(w, http.StatusNotImplemented, errors.New("pvp service not configured"))
		return
	}

	charID := r.PathValue("id")
	roomID := r.PathValue("room_id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		var req pvpJoinRoomRequest
		if r.Body != nil && r.ContentLength > 0 {
			_ = json.NewDecoder(r.Body).Decode(&req)
		}

		detail, err := h.pvp.JoinRoom(r.Context(), char.ID, roomID, req.Password)
		if err != nil {
			if errors.Is(err, pvp.ErrRoomNotFound) {
				writeError(w, http.StatusNotFound, err)
				return
			}
			if errors.Is(err, pvp.ErrInvalidPassword) {
				writeError(w, http.StatusForbidden, err)
				return
			}
			if errors.Is(err, pvp.ErrRoomFull) || errors.Is(err, pvp.ErrMatchAlreadyStarted) ||
				errors.Is(err, pvp.ErrAlreadyInRoom) || errors.Is(err, pvp.ErrInsufficientBetFunds) ||
				errors.Is(err, pvp.ErrNeedJoinNotMet) {
				writeError(w, http.StatusUnprocessableEntity, err)
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		writeJSON(w, http.StatusOK, detail)
	})
}

func (h *Handler) handleLeavePvPRoom(w http.ResponseWriter, r *http.Request) {
	if h.pvp == nil {
		writeError(w, http.StatusNotImplemented, errors.New("pvp service not configured"))
		return
	}

	charID := r.PathValue("id")
	roomID := r.PathValue("room_id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		err := h.pvp.LeaveRoom(r.Context(), char.ID, roomID)
		if err != nil {
			if errors.Is(err, pvp.ErrRoomNotFound) {
				writeError(w, http.StatusNotFound, err)
				return
			}
			if errors.Is(err, pvp.ErrCharacterNotInRoom) || errors.Is(err, pvp.ErrMatchAlreadyStarted) {
				writeError(w, http.StatusUnprocessableEntity, err)
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		writeJSON(w, http.StatusOK, pvpMessageResponse{Message: "left room successfully"})
	})
}

func (h *Handler) handleSelectPvPTeam(w http.ResponseWriter, r *http.Request) {
	if h.pvp == nil {
		writeError(w, http.StatusNotImplemented, errors.New("pvp service not configured"))
		return
	}

	charID := r.PathValue("id")
	roomID := r.PathValue("room_id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		var req pvpSelectTeamRequest
		if !decodeJSON(w, r, &req) {
			return
		}

		detail, err := h.pvp.SelectTeam(r.Context(), char.ID, roomID, req.TeamColor)
		if err != nil {
			if errors.Is(err, pvp.ErrInvalidTeamColor) {
				writeError(w, http.StatusBadRequest, err)
				return
			}
			if errors.Is(err, pvp.ErrRoomNotFound) {
				writeError(w, http.StatusNotFound, err)
				return
			}
			if errors.Is(err, pvp.ErrCharacterNotInRoom) || errors.Is(err, pvp.ErrMatchAlreadyStarted) {
				writeError(w, http.StatusUnprocessableEntity, err)
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		writeJSON(w, http.StatusOK, detail)
	})
}

func (h *Handler) handleStartPvPMatch(w http.ResponseWriter, r *http.Request) {
	if h.pvp == nil {
		writeError(w, http.StatusNotImplemented, errors.New("pvp service not configured"))
		return
	}

	charID := r.PathValue("id")
	roomID := r.PathValue("room_id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		detail, err := h.pvp.StartMatch(r.Context(), char.ID, roomID)
		if err != nil {
			if errors.Is(err, pvp.ErrRoomNotFound) {
				writeError(w, http.StatusNotFound, err)
				return
			}
			if errors.Is(err, pvp.ErrNotRoomLeader) {
				writeError(w, http.StatusForbidden, err)
				return
			}
			if errors.Is(err, pvp.ErrMatchAlreadyStarted) || errors.Is(err, pvp.ErrNotEnoughParticipants) ||
				errors.Is(err, pvp.ErrTeamsNotConfigured) || errors.Is(err, pvp.ErrNeedAtLeastTwoTeams) {
				writeError(w, http.StatusUnprocessableEntity, err)
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		writeJSON(w, http.StatusOK, detail)
	})
}

func (h *Handler) handleAdvancePvPRound(w http.ResponseWriter, r *http.Request) {
	if h.pvp == nil {
		writeError(w, http.StatusNotImplemented, errors.New("pvp service not configured"))
		return
	}

	charID := r.PathValue("id")
	roomID := r.PathValue("room_id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		res, err := h.pvp.AdvanceRound(r.Context(), char.ID, roomID)
		if err != nil {
			if errors.Is(err, pvp.ErrRoomNotFound) {
				writeError(w, http.StatusNotFound, err)
				return
			}
			if errors.Is(err, pvp.ErrNotRoomLeader) {
				writeError(w, http.StatusForbidden, err)
				return
			}
			if errors.Is(err, pvp.ErrMatchNotInProgress) {
				writeError(w, http.StatusUnprocessableEntity, err)
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		writeJSON(w, http.StatusOK, res)
	})
}
