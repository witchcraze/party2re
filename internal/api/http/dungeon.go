package http

import (
	"context"
	"errors"
	"net/http"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/dungeon"
)

// DungeonService defines dungeon explorations operations exposed over HTTP.
type DungeonService interface {
	ListDungeons(ctx context.Context, characterID string) ([]dungeon.DungeonOverview, error)
	StartExpedition(ctx context.Context, characterID string, dungeonID string) (*dungeon.ActiveExpedition, error)
	StartPartyExpedition(ctx context.Context, leaderID string, memberIDs []string, dungeonID string, partyID string) (*dungeon.ActiveExpedition, error)
	Move(ctx context.Context, characterID string, dir dungeon.Direction) (dungeon.ExpeditionStepResult, error)
	Escape(ctx context.Context, characterID string) (dungeon.ExpeditionStepResult, error)
	GetActiveExpedition(ctx context.Context, characterID string) (*dungeon.ActiveExpedition, error)
	ViewMap(ctx context.Context, characterID string) (dungeon.MapView, error)
}

// WithDungeon configures the dungeon service for the Handler.
func WithDungeon(d DungeonService) Option {
	return func(h *Handler) {
		h.dungeons = d
	}
}

// -------------------------------------------------------------------
// Dungeon Handlers & DTOs
// -------------------------------------------------------------------

type startDungeonRequest struct {
	DungeonID string `json:"dungeon_id"`
}

type startPartyDungeonRequest struct {
	DungeonID string   `json:"dungeon_id"`
	MemberIDs []string `json:"member_ids"`
	PartyID   string   `json:"party_id"`
}

type moveDungeonRequest struct {
	Direction string `json:"direction"` // "north", "south", "east", "west"
}

type dungeonListResponse struct {
	Dungeons []dungeon.DungeonOverview `json:"dungeons"`
}

type startDungeonResponse struct {
	Expedition *dungeon.ActiveExpedition `json:"expedition"`
}

type dungeonStepResponse struct {
	Result dungeon.ExpeditionStepResult `json:"result"`
}

type dungeonMapResponse struct {
	MapView dungeon.MapView `json:"map_view"`
}

func (h *Handler) handleListDungeons(w http.ResponseWriter, r *http.Request) {
	if h.dungeons == nil {
		writeError(w, http.StatusNotImplemented, errors.New("dungeon service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		dungeonsList, err := h.dungeons.ListDungeons(r.Context(), char.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		writeJSON(w, http.StatusOK, dungeonListResponse{
			Dungeons: dungeonsList,
		})
	})
}

func (h *Handler) handleStartDungeon(w http.ResponseWriter, r *http.Request) {
	if h.dungeons == nil {
		writeError(w, http.StatusNotImplemented, errors.New("dungeon service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		var req startDungeonRequest
		if !decodeJSON(w, r, &req) {
			return
		}

		if req.DungeonID == "" {
			writeError(w, http.StatusBadRequest, errors.New("dungeon_id is required"))
			return
		}

		exp, err := h.dungeons.StartExpedition(r.Context(), char.ID, req.DungeonID)
		if err != nil {
			if errors.Is(err, dungeon.ErrDungeonNotFound) || errors.Is(err, dungeon.ErrCharacterNotFound) {
				writeError(w, http.StatusBadRequest, err)
				return
			}
			if errors.Is(err, dungeon.ErrLevelRequirementNotMet) || errors.Is(err, dungeon.ErrActiveExpeditionExists) {
				writeError(w, http.StatusUnprocessableEntity, err)
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		writeJSON(w, http.StatusOK, startDungeonResponse{
			Expedition: exp,
		})
	})
}

func (h *Handler) handleStartPartyDungeon(w http.ResponseWriter, r *http.Request) {
	if h.dungeons == nil {
		writeError(w, http.StatusNotImplemented, errors.New("dungeon service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		var req startPartyDungeonRequest
		if !decodeJSON(w, r, &req) {
			return
		}

		if req.DungeonID == "" {
			writeError(w, http.StatusBadRequest, errors.New("dungeon_id is required"))
			return
		}

		memberIDs := req.MemberIDs
		if len(memberIDs) == 0 {
			memberIDs = []string{char.ID}
		}

		exp, err := h.dungeons.StartPartyExpedition(r.Context(), char.ID, memberIDs, req.DungeonID, req.PartyID)
		if err != nil {
			if errors.Is(err, dungeon.ErrDungeonNotFound) || errors.Is(err, dungeon.ErrCharacterNotFound) {
				writeError(w, http.StatusBadRequest, err)
				return
			}
			if errors.Is(err, dungeon.ErrLevelRequirementNotMet) || errors.Is(err, dungeon.ErrActiveExpeditionExists) || errors.Is(err, dungeon.ErrTooManyPartyMembers) {
				writeError(w, http.StatusUnprocessableEntity, err)
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		writeJSON(w, http.StatusOK, startDungeonResponse{
			Expedition: exp,
		})
	})
}

func (h *Handler) handleMoveDungeon(w http.ResponseWriter, r *http.Request) {
	if h.dungeons == nil {
		writeError(w, http.StatusNotImplemented, errors.New("dungeon service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		var req moveDungeonRequest
		if !decodeJSON(w, r, &req) {
			return
		}

		dir := dungeon.Direction(req.Direction)
		res, err := h.dungeons.Move(r.Context(), char.ID, dir)
		if err != nil {
			if errors.Is(err, dungeon.ErrNoActiveExpedition) {
				writeError(w, http.StatusNotFound, err)
				return
			}
			if errors.Is(err, dungeon.ErrInvalidDirection) || errors.Is(err, dungeon.ErrImpassableWall) {
				writeError(w, http.StatusBadRequest, err)
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		writeJSON(w, http.StatusOK, dungeonStepResponse{
			Result: res,
		})
	})
}

func (h *Handler) handleEscapeDungeon(w http.ResponseWriter, r *http.Request) {
	if h.dungeons == nil {
		writeError(w, http.StatusNotImplemented, errors.New("dungeon service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		res, err := h.dungeons.Escape(r.Context(), char.ID)
		if err != nil {
			if errors.Is(err, dungeon.ErrNoActiveExpedition) {
				writeError(w, http.StatusNotFound, err)
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		writeJSON(w, http.StatusOK, dungeonStepResponse{
			Result: res,
		})
	})
}

func (h *Handler) handleGetDungeonMap(w http.ResponseWriter, r *http.Request) {
	if h.dungeons == nil {
		writeError(w, http.StatusNotImplemented, errors.New("dungeon service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		mv, err := h.dungeons.ViewMap(r.Context(), char.ID)
		if err != nil {
			if errors.Is(err, dungeon.ErrNoActiveExpedition) {
				writeError(w, http.StatusNotFound, err)
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		writeJSON(w, http.StatusOK, dungeonMapResponse{
			MapView: mv,
		})
	})
}
