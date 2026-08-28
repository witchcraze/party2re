package http

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/park"
)

// ParkService defines park / public board operations exposed over HTTP.
type ParkService interface {
	PostMessage(ctx context.Context, characterID, content, color, recipient string) (park.Post, error)
	GetRecentPosts(ctx context.Context, limit, offset int) ([]park.Post, int, error)
	TalkToNPC(ctx context.Context, characterID string) (string, error)
	Divinate(ctx context.Context, characterID string) (park.DivinationResult, error)
	InspectNPC() string
}

// WithPark configures the ParkService for the handler.
func WithPark(p ParkService) Option {
	return func(h *Handler) {
		h.park = p
	}
}

type listParkPostsResponse struct {
	Posts []park.Post `json:"posts"`
	Total int         `json:"total"`
}

type postParkMessageRequest struct {
	CharacterID   string `json:"character_id"`
	Content       string `json:"content"`
	Color         string `json:"color"`
	RecipientName string `json:"recipient_name"`
}

type parkCharacterActionRequest struct {
	CharacterID string `json:"character_id"`
}

type parkDialogueResponse struct {
	Dialogue string `json:"dialogue"`
}

func (h *Handler) handleGetParkPosts(w http.ResponseWriter, r *http.Request) {
	if h.park == nil {
		writeError(w, http.StatusNotImplemented, errors.New("park service not configured"))
		return
	}

	limit := 20
	offset := 0
	if l := r.URL.Query().Get("limit"); l != "" {
		if val, err := strconv.Atoi(l); err == nil && val > 0 {
			limit = val
		}
	}
	if o := r.URL.Query().Get("offset"); o != "" {
		if val, err := strconv.Atoi(o); err == nil && val >= 0 {
			offset = val
		}
	}

	posts, total, err := h.park.GetRecentPosts(r.Context(), limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusOK, listParkPostsResponse{
		Posts: posts,
		Total: total,
	})
}

func (h *Handler) handlePostParkMessage(w http.ResponseWriter, r *http.Request) {
	if h.park == nil {
		writeError(w, http.StatusNotImplemented, errors.New("park service not configured"))
		return
	}

	withAuthenticatedCharacterAndJSON(h, w, r, func(req *postParkMessageRequest) string {
		return req.CharacterID
	}, func(_ coreplayer.Player, char corecharacter.Character, req postParkMessageRequest) {
		post, err := h.park.PostMessage(r.Context(), char.ID, req.Content, req.Color, req.RecipientName)
		if err != nil {
			if errors.Is(err, park.ErrRateLimited) {
				writeError(w, http.StatusTooManyRequests, err)
				return
			}
			if errors.Is(err, park.ErrEmptyContent) || errors.Is(err, park.ErrContentTooLong) || errors.Is(err, park.ErrInvalidColor) {
				writeError(w, http.StatusUnprocessableEntity, err)
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		writeJSON(w, http.StatusCreated, post)
	})
}

func (h *Handler) handleParkNPCTalk(w http.ResponseWriter, r *http.Request) {
	if h.park == nil {
		writeError(w, http.StatusNotImplemented, errors.New("park service not configured"))
		return
	}

	withAuthenticatedCharacterAndJSON(h, w, r, func(req *parkCharacterActionRequest) string {
		return req.CharacterID
	}, func(_ coreplayer.Player, char corecharacter.Character, req parkCharacterActionRequest) {
		dialogue, err := h.park.TalkToNPC(r.Context(), char.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		writeJSON(w, http.StatusOK, parkDialogueResponse{Dialogue: dialogue})
	})
}

func (h *Handler) handleParkNPCDivinate(w http.ResponseWriter, r *http.Request) {
	if h.park == nil {
		writeError(w, http.StatusNotImplemented, errors.New("park service not configured"))
		return
	}

	withAuthenticatedCharacterAndJSON(h, w, r, func(req *parkCharacterActionRequest) string {
		return req.CharacterID
	}, func(_ coreplayer.Player, char corecharacter.Character, req parkCharacterActionRequest) {
		result, err := h.park.Divinate(r.Context(), char.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		writeJSON(w, http.StatusOK, result)
	})
}

func (h *Handler) handleParkNPCInspect(w http.ResponseWriter, r *http.Request) {
	if h.park == nil {
		writeError(w, http.StatusNotImplemented, errors.New("park service not configured"))
		return
	}

	dialogue := h.park.InspectNPC()
	writeJSON(w, http.StatusOK, parkDialogueResponse{Dialogue: dialogue})
}
