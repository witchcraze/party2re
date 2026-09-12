package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/gvg"
)

// GvGService defines Guild Battle operations exposed over HTTP.
type GvGService interface {
	CreateRoom(ctx context.Context, creatorCharID string, req gvg.CreateRoomRequest) (gvg.RoomDetail, error)
	GetRoom(ctx context.Context, roomID string) (gvg.RoomDetail, error)
	ListRooms(ctx context.Context) ([]gvg.RoomSummary, error)
	JoinRoom(ctx context.Context, charID string, roomID string, password string) (gvg.RoomDetail, error)
	LeaveRoom(ctx context.Context, charID string, roomID string) error
	StartMatch(ctx context.Context, leaderID string, roomID string) (gvg.RoomDetail, error)
	AdvanceRound(ctx context.Context, leaderID string, roomID string) (gvg.RoundResolution, error)
	GetStanding(ctx context.Context, guildID string) (gvg.GvGStanding, error)
	GetLeaderboard(ctx context.Context, limit int) ([]gvg.GvGStanding, error)
}

// WithGvG configures the gvg service for the Handler.
func WithGvG(g GvGService) Option {
	return func(h *Handler) {
		h.gvg = g
	}
}

// -------------------------------------------------------------------
// Request & Response Types
// -------------------------------------------------------------------

type gvgCreateRoomRequest struct {
	Name       string `json:"name"`
	Password   string `json:"password,omitempty"`
	Speed      int    `json:"speed,omitempty"`
	Stage      int    `json:"stage,omitempty"`
	MaxMembers int    `json:"max_members,omitempty"`
	TargetWins int    `json:"target_wins,omitempty"`
	NeedJoin   string `json:"need_join,omitempty"`
}

type gvgJoinRoomRequest struct {
	Password string `json:"password,omitempty"`
}

type gvgRoomsResponse struct {
	Rooms []gvg.RoomSummary `json:"rooms"`
}

type gvgMessageResponse struct {
	Message string `json:"message"`
}

type gvgLeaderboardResponse struct {
	Standings []gvg.GvGStanding `json:"standings"`
}

// -------------------------------------------------------------------
// GvG Room HTTP Handlers
// -------------------------------------------------------------------

func (h *Handler) handleCreateGvGRoom(w http.ResponseWriter, r *http.Request) {
	if h.gvg == nil {
		writeError(w, http.StatusNotImplemented, errors.New("gvg service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		var req gvgCreateRoomRequest
		if !decodeJSON(w, r, &req) {
			return
		}

		domainReq := gvg.CreateRoomRequest{
			Name:       req.Name,
			Password:   req.Password,
			Speed:      req.Speed,
			Stage:      req.Stage,
			MaxMembers: req.MaxMembers,
			TargetWins: req.TargetWins,
			NeedJoin:   req.NeedJoin,
		}

		room, err := h.gvg.CreateRoom(r.Context(), char.ID, domainReq)
		if err != nil {
			if errors.Is(err, gvg.ErrInvalidRoomName) || errors.Is(err, gvg.ErrInvalidMaxMembers) ||
				errors.Is(err, gvg.ErrInvalidTargetWins) {
				writeError(w, http.StatusBadRequest, err)
				return
			}
			if errors.Is(err, gvg.ErrActorNotInGuild) || errors.Is(err, gvg.ErrFriendlyGuildCannotBattle) ||
				errors.Is(err, gvg.ErrAlreadyInRoom) || errors.Is(err, gvg.ErrNeedJoinNotMet) ||
				errors.Is(err, gvg.ErrCharacterUnconscious) || errors.Is(err, gvg.ErrCharacterExhausted) {
				writeError(w, http.StatusUnprocessableEntity, err)
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		writeJSON(w, http.StatusCreated, room)
	})
}

func (h *Handler) handleListGvGRooms(w http.ResponseWriter, r *http.Request) {
	if h.gvg == nil {
		writeError(w, http.StatusNotImplemented, errors.New("gvg service not configured"))
		return
	}

	rooms, err := h.gvg.ListRooms(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if rooms == nil {
		rooms = []gvg.RoomSummary{}
	}

	writeJSON(w, http.StatusOK, gvgRoomsResponse{Rooms: rooms})
}

func (h *Handler) handleGetGvGRoom(w http.ResponseWriter, r *http.Request) {
	if h.gvg == nil {
		writeError(w, http.StatusNotImplemented, errors.New("gvg service not configured"))
		return
	}

	roomID := r.PathValue("room_id")
	if roomID == "" {
		writeError(w, http.StatusBadRequest, errors.New("room_id is required"))
		return
	}

	room, err := h.gvg.GetRoom(r.Context(), roomID)
	if err != nil {
		if errors.Is(err, gvg.ErrRoomNotFound) {
			writeError(w, http.StatusNotFound, err)
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusOK, room)
}

func (h *Handler) handleJoinGvGRoom(w http.ResponseWriter, r *http.Request) {
	if h.gvg == nil {
		writeError(w, http.StatusNotImplemented, errors.New("gvg service not configured"))
		return
	}

	charID := r.PathValue("id")
	roomID := r.PathValue("room_id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		var req gvgJoinRoomRequest
		if r.Body != nil && r.ContentLength > 0 {
			_ = json.NewDecoder(r.Body).Decode(&req)
		}

		detail, err := h.gvg.JoinRoom(r.Context(), char.ID, roomID, req.Password)
		if err != nil {
			if errors.Is(err, gvg.ErrRoomNotFound) {
				writeError(w, http.StatusNotFound, err)
				return
			}
			if errors.Is(err, gvg.ErrInvalidPassword) {
				writeError(w, http.StatusForbidden, err)
				return
			}
			if errors.Is(err, gvg.ErrRoomFull) || errors.Is(err, gvg.ErrMatchAlreadyStarted) ||
				errors.Is(err, gvg.ErrAlreadyInRoom) || errors.Is(err, gvg.ErrNeedJoinNotMet) ||
				errors.Is(err, gvg.ErrActorNotInGuild) || errors.Is(err, gvg.ErrFriendlyGuildCannotBattle) ||
				errors.Is(err, gvg.ErrCharacterUnconscious) || errors.Is(err, gvg.ErrCharacterExhausted) {
				writeError(w, http.StatusUnprocessableEntity, err)
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		writeJSON(w, http.StatusOK, detail)
	})
}

func (h *Handler) handleLeaveGvGRoom(w http.ResponseWriter, r *http.Request) {
	if h.gvg == nil {
		writeError(w, http.StatusNotImplemented, errors.New("gvg service not configured"))
		return
	}

	charID := r.PathValue("id")
	roomID := r.PathValue("room_id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		err := h.gvg.LeaveRoom(r.Context(), char.ID, roomID)
		if err != nil {
			if errors.Is(err, gvg.ErrRoomNotFound) {
				writeError(w, http.StatusNotFound, err)
				return
			}
			if errors.Is(err, gvg.ErrCharacterNotInRoom) || errors.Is(err, gvg.ErrMatchNotInProgress) {
				writeError(w, http.StatusUnprocessableEntity, err)
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		writeJSON(w, http.StatusOK, gvgMessageResponse{Message: "left gvg room successfully"})
	})
}

func (h *Handler) handleStartGvGMatch(w http.ResponseWriter, r *http.Request) {
	if h.gvg == nil {
		writeError(w, http.StatusNotImplemented, errors.New("gvg service not configured"))
		return
	}

	charID := r.PathValue("id")
	roomID := r.PathValue("room_id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		detail, err := h.gvg.StartMatch(r.Context(), char.ID, roomID)
		if err != nil {
			if errors.Is(err, gvg.ErrRoomNotFound) {
				writeError(w, http.StatusNotFound, err)
				return
			}
			if errors.Is(err, gvg.ErrNotRoomLeader) {
				writeError(w, http.StatusForbidden, err)
				return
			}
			if errors.Is(err, gvg.ErrMatchAlreadyStarted) || errors.Is(err, gvg.ErrNotEnoughParticipants) ||
				errors.Is(err, gvg.ErrNeedAtLeastTwoGuilds) {
				writeError(w, http.StatusUnprocessableEntity, err)
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		writeJSON(w, http.StatusOK, detail)
	})
}

func (h *Handler) handleAdvanceGvGRound(w http.ResponseWriter, r *http.Request) {
	if h.gvg == nil {
		writeError(w, http.StatusNotImplemented, errors.New("gvg service not configured"))
		return
	}

	charID := r.PathValue("id")
	roomID := r.PathValue("room_id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		res, err := h.gvg.AdvanceRound(r.Context(), char.ID, roomID)
		if err != nil {
			if errors.Is(err, gvg.ErrRoomNotFound) {
				writeError(w, http.StatusNotFound, err)
				return
			}
			if errors.Is(err, gvg.ErrNotRoomLeader) {
				writeError(w, http.StatusForbidden, err)
				return
			}
			if errors.Is(err, gvg.ErrMatchNotInProgress) {
				writeError(w, http.StatusUnprocessableEntity, err)
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		writeJSON(w, http.StatusOK, res)
	})
}

func (h *Handler) handleGetGvGStanding(w http.ResponseWriter, r *http.Request) {
	if h.gvg == nil {
		writeError(w, http.StatusNotImplemented, errors.New("gvg service not configured"))
		return
	}

	guildID := r.PathValue("guild_id")
	if guildID == "" {
		writeError(w, http.StatusBadRequest, errors.New("guild_id is required"))
		return
	}

	standing, err := h.gvg.GetStanding(r.Context(), guildID)
	if err != nil {
		if errors.Is(err, gvg.ErrGuildNotFound) {
			writeError(w, http.StatusNotFound, err)
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusOK, standing)
}

func (h *Handler) handleGetGvGLeaderboard(w http.ResponseWriter, r *http.Request) {
	if h.gvg == nil {
		writeError(w, http.StatusNotImplemented, errors.New("gvg service not configured"))
		return
	}

	limit := 50
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	standings, err := h.gvg.GetLeaderboard(r.Context(), limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if standings == nil {
		standings = []gvg.GvGStanding{}
	}

	writeJSON(w, http.StatusOK, gvgLeaderboardResponse{Standings: standings})
}
