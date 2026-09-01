package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/god"
)

// GodService defines the god wishes and limit break operations exposed over HTTP.
type GodService interface {
	GetWishes(ctx context.Context, characterID string, realm god.Realm) ([]god.Wish, error)
	GrantWish(ctx context.Context, characterID, wishID string, realm god.Realm) (god.WishResult, error)
	GetDialogue(realm god.Realm) []string
}

// WithGod configures the GodService for the Handler.
func WithGod(g GodService) Option {
	return func(h *Handler) {
		h.god = g
	}
}

type grantWishRequest struct {
	Realm  string `json:"realm"`
	WishID string `json:"wish_id"`
}

type listWishesResponse struct {
	Realm  string     `json:"realm"`
	Wishes []god.Wish `json:"wishes"`
}

type dialogueResponse struct {
	Realm    string   `json:"realm"`
	Dialogue []string `json:"dialogue"`
}

type grantWishResponse struct {
	Character corecharacter.Character `json:"character"`
	Wish      god.Wish                `json:"wish"`
	Message   string                  `json:"message"`
	NPCSpeech string                  `json:"npc_speech"`
}

func (h *Handler) handleGetGodDialogue(w http.ResponseWriter, r *http.Request) {
	if h.god == nil {
		writeError(w, http.StatusNotImplemented, errors.New("god service not configured"))
		return
	}

	realmParam := strings.TrimSpace(r.URL.Query().Get("realm"))
	realm := god.RealmHeaven
	if realmParam == string(god.RealmUnderworld) {
		realm = god.RealmUnderworld
	}

	dialogue := h.god.GetDialogue(realm)
	writeJSON(w, http.StatusOK, dialogueResponse{
		Realm:    string(realm),
		Dialogue: dialogue,
	})
}

func (h *Handler) handleGetGodWishes(w http.ResponseWriter, r *http.Request) {
	if h.god == nil {
		writeError(w, http.StatusNotImplemented, errors.New("god service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		realmParam := strings.TrimSpace(r.URL.Query().Get("realm"))
		realm := god.RealmHeaven
		if realmParam == string(god.RealmUnderworld) {
			realm = god.RealmUnderworld
		}

		wishes, err := h.god.GetWishes(r.Context(), char.ID, realm)
		if err != nil {
			if errors.Is(err, god.ErrInvalidRealm) {
				writeError(w, http.StatusBadRequest, err)
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		writeJSON(w, http.StatusOK, listWishesResponse{
			Realm:  string(realm),
			Wishes: wishes,
		})
	})
}

func (h *Handler) handleGrantGodWish(w http.ResponseWriter, r *http.Request) {
	if h.god == nil {
		writeError(w, http.StatusNotImplemented, errors.New("god service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		var req grantWishRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, errors.New("invalid request body"))
			return
		}

		req.Realm = strings.TrimSpace(req.Realm)
		req.WishID = strings.TrimSpace(req.WishID)
		realm := god.RealmHeaven
		if req.Realm == string(god.RealmUnderworld) {
			realm = god.RealmUnderworld
		} else if req.Realm != "" && req.Realm != string(god.RealmHeaven) {
			writeError(w, http.StatusBadRequest, god.ErrInvalidRealm)
			return
		}

		res, err := h.god.GrantWish(r.Context(), char.ID, req.WishID, realm)
		if err != nil {
			if errors.Is(err, god.ErrWishNotFound) ||
				errors.Is(err, god.ErrWishRequirement) ||
				errors.Is(err, god.ErrLimitBreakMaxed) ||
				errors.Is(err, god.ErrInvalidWishID) ||
				errors.Is(err, god.ErrInvalidRealm) {
				writeError(w, http.StatusBadRequest, err)
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		writeJSON(w, http.StatusOK, grantWishResponse{
			Character: res.Character,
			Wish:      res.Wish,
			Message:   res.Message,
			NPCSpeech: res.NPCSpeech,
		})
	})
}
