package http

import (
	"context"
	"errors"
	"net/http"
	"strings"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/pagination"
	"github.com/witchcraze/party2re/internal/replay"
)

// ReplayService defines battle replay operations exposed over HTTP.
type ReplayService interface {
	GetReplay(ctx context.Context, sessionID, id string) (*replay.BattleReplay, error)
	GetCharacterHistory(ctx context.Context, sessionID, characterID string, combatType string, limit int) ([]replay.ReplayHeader, error)
	GetCharacterHistoryByCursor(ctx context.Context, sessionID, characterID string, combatType string, limit int, cursor string) (pagination.CursorPage[replay.ReplayHeader], error)
	GetRecentReplays(ctx context.Context, combatType string, limit int) ([]replay.ReplayHeader, error)
	GetRecentReplaysByCursor(ctx context.Context, combatType string, limit int, cursor string) (pagination.CursorPage[replay.ReplayHeader], error)
}

// WithReplay configures the replay service for the Handler.
func WithReplay(r ReplayService) Option {
	return func(h *Handler) {
		h.replay = r
	}
}

func (h *Handler) handleGetReplay(w http.ResponseWriter, r *http.Request) {
	if h.replay == nil {
		writeError(w, http.StatusNotImplemented, errors.New("replay service not configured"))
		return
	}

	replayID := strings.TrimSpace(r.PathValue("id"))
	if replayID == "" {
		writeError(w, http.StatusBadRequest, errors.New("replay id is required"))
		return
	}

	if _, supplied := r.Header["Authorization"]; supplied {
		if _, ok := h.authenticatePlayer(w, r); !ok {
			return
		}
	}
	rep, err := h.replay.GetReplay(r.Context(), sessionIDFromRequest(r), replayID)
	if err != nil {
		writeReplayReadError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, rep)
}

func (h *Handler) handleGetCharacterReplays(w http.ResponseWriter, r *http.Request) {
	if h.replay == nil {
		writeError(w, http.StatusNotImplemented, errors.New("replay service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		combatType := r.URL.Query().Get("combat_type")

		if r.URL != nil && r.URL.Query().Has("cursor") {
			cursorParams := pagination.ParseCursorRequest(r)
			page, err := h.replay.GetCharacterHistoryByCursor(r.Context(), sessionIDFromRequest(r), char.ID, combatType, cursorParams.Limit, cursorParams.Cursor)
			if err != nil {
				writeReplayReadError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, page)
			return
		}

		params := pagination.ParseRequest(r)
		items, err := h.replay.GetCharacterHistory(r.Context(), sessionIDFromRequest(r), char.ID, combatType, params.Limit)
		if err != nil {
			writeReplayReadError(w, err)
			return
		}
		if items == nil {
			items = []replay.ReplayHeader{}
		}
		writeJSON(w, http.StatusOK, items)
	})
}

func writeReplayReadError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, coreplayer.ErrPlayerBanned):
		writeError(w, http.StatusForbidden, err)
	case errors.Is(err, coreplayer.ErrAuthentication), errors.Is(err, coreplayer.ErrInvalidSession):
		writeError(w, http.StatusUnauthorized, errors.New("invalid session"))
	case errors.Is(err, replay.ErrReplayNotFound):
		writeError(w, http.StatusNotFound, err)
	default:
		writeError(w, http.StatusInternalServerError, err)
	}
}

func (h *Handler) handleGetRecentReplays(w http.ResponseWriter, r *http.Request) {
	if h.replay == nil {
		writeError(w, http.StatusNotImplemented, errors.New("replay service not configured"))
		return
	}

	combatType := r.URL.Query().Get("combat_type")

	if r.URL != nil && r.URL.Query().Has("cursor") {
		cursorParams := pagination.ParseCursorRequest(r)
		page, err := h.replay.GetRecentReplaysByCursor(r.Context(), combatType, cursorParams.Limit, cursorParams.Cursor)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, page)
		return
	}

	params := pagination.ParseRequest(r)
	items, err := h.replay.GetRecentReplays(r.Context(), combatType, params.Limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if items == nil {
		items = []replay.ReplayHeader{}
	}
	writeJSON(w, http.StatusOK, items)
}
