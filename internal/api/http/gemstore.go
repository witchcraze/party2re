package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/gemstore"
)

// GemStoreService defines the gem store operations exposed over HTTP.
type GemStoreService interface {
	BuyGem(ctx context.Context, characterID, gemID string) (gemstore.BuyResult, error)
	SellGem(ctx context.Context, characterID, itemInstanceOrDefID string) (gemstore.SellResult, error)
	SendGem(ctx context.Context, senderID, recipientID, itemInstanceOrDefID string) (gemstore.SendResult, error)
	SynthesizeGem(ctx context.Context, characterID, recipeID string) (gemstore.SynthesizeResult, error)
	AppraiseItem(ctx context.Context, characterID, itemInstanceOrDefID string) (gemstore.AppraiseResult, error)
	GetCatalog(level int) []gemstore.Gem
	GetRecipes() []gemstore.Recipe
	GetDialogue() []string
}

// WithGemStore configures the GemStoreService for the HTTP handler.
func WithGemStore(service GemStoreService) Option {
	return func(h *Handler) {
		h.gemstore = service
	}
}

// Request types
type gemStoreBuyRequest struct {
	GemID string `json:"gem_id"`
}

type gemStoreSellRequest struct {
	ItemID string `json:"item_id"`
}

type gemStoreSendRequest struct {
	RecipientCharacterID string `json:"recipient_character_id"`
	ItemID               string `json:"item_id"`
}

type gemStoreSynthesizeRequest struct {
	RecipeID string `json:"recipe_id"`
}

type gemStoreAppraiseRequest struct {
	ItemID string `json:"item_id"`
}

// Public endpoints
func (h *Handler) handleGetGemStoreCatalog(w http.ResponseWriter, r *http.Request) {
	if h.gemstore == nil {
		writeError(w, http.StatusNotImplemented, errors.New("gemstore service not configured"))
		return
	}

	level := 1
	if levelStr := r.URL.Query().Get("level"); levelStr != "" {
		if l, err := strconv.Atoi(levelStr); err == nil && l > 0 {
			level = l
		}
	}

	gems := h.gemstore.GetCatalog(level)
	writeJSON(w, http.StatusOK, map[string]any{
		"gems":  gems,
		"level": level,
	})
}

func (h *Handler) handleGetGemStoreRecipes(w http.ResponseWriter, r *http.Request) {
	if h.gemstore == nil {
		writeError(w, http.StatusNotImplemented, errors.New("gemstore service not configured"))
		return
	}

	recipes := h.gemstore.GetRecipes()
	writeJSON(w, http.StatusOK, map[string]any{
		"recipes": recipes,
	})
}

func (h *Handler) handleGetGemStoreDialogue(w http.ResponseWriter, r *http.Request) {
	if h.gemstore == nil {
		writeError(w, http.StatusNotImplemented, errors.New("gemstore service not configured"))
		return
	}

	dialogue := h.gemstore.GetDialogue()
	writeJSON(w, http.StatusOK, map[string]any{
		"dialogue": dialogue,
	})
}

// Authenticated Character endpoints
func (h *Handler) handleGemStoreBuy(w http.ResponseWriter, r *http.Request) {
	if h.gemstore == nil {
		writeError(w, http.StatusNotImplemented, errors.New("gemstore service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		var req gemStoreBuyRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, errors.New("invalid request body"))
			return
		}

		if req.GemID == "" {
			writeError(w, http.StatusBadRequest, errors.New("gem_id is required"))
			return
		}

		res, err := h.gemstore.BuyGem(r.Context(), char.ID, req.GemID)
		if err != nil {
			h.writeGemStoreError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, res)
	})
}

func (h *Handler) handleGemStoreSell(w http.ResponseWriter, r *http.Request) {
	if h.gemstore == nil {
		writeError(w, http.StatusNotImplemented, errors.New("gemstore service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		var req gemStoreSellRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, errors.New("invalid request body"))
			return
		}

		if req.ItemID == "" {
			writeError(w, http.StatusBadRequest, errors.New("item_id is required"))
			return
		}

		res, err := h.gemstore.SellGem(r.Context(), char.ID, req.ItemID)
		if err != nil {
			h.writeGemStoreError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, res)
	})
}

func (h *Handler) handleGemStoreSend(w http.ResponseWriter, r *http.Request) {
	if h.gemstore == nil {
		writeError(w, http.StatusNotImplemented, errors.New("gemstore service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		var req gemStoreSendRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, errors.New("invalid request body"))
			return
		}

		if req.RecipientCharacterID == "" || req.ItemID == "" {
			writeError(w, http.StatusBadRequest, errors.New("recipient_character_id and item_id are required"))
			return
		}

		res, err := h.gemstore.SendGem(r.Context(), char.ID, req.RecipientCharacterID, req.ItemID)
		if err != nil {
			h.writeGemStoreError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, res)
	})
}

func (h *Handler) handleGemStoreSynthesize(w http.ResponseWriter, r *http.Request) {
	if h.gemstore == nil {
		writeError(w, http.StatusNotImplemented, errors.New("gemstore service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		var req gemStoreSynthesizeRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, errors.New("invalid request body"))
			return
		}

		if req.RecipeID == "" {
			writeError(w, http.StatusBadRequest, errors.New("recipe_id is required"))
			return
		}

		res, err := h.gemstore.SynthesizeGem(r.Context(), char.ID, req.RecipeID)
		if err != nil {
			h.writeGemStoreError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, res)
	})
}

func (h *Handler) handleGemStoreAppraise(w http.ResponseWriter, r *http.Request) {
	if h.gemstore == nil {
		writeError(w, http.StatusNotImplemented, errors.New("gemstore service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		var req gemStoreAppraiseRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, errors.New("invalid request body"))
			return
		}

		if req.ItemID == "" {
			writeError(w, http.StatusBadRequest, errors.New("item_id is required"))
			return
		}

		res, err := h.gemstore.AppraiseItem(r.Context(), char.ID, req.ItemID)
		if err != nil {
			h.writeGemStoreError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, res)
	})
}

func (h *Handler) writeGemStoreError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, gemstore.ErrGemNotFound), errors.Is(err, gemstore.ErrRecipeNotFound):
		writeError(w, http.StatusNotFound, err)
	case errors.Is(err, gemstore.ErrLevelTooLow),
		errors.Is(err, gemstore.ErrInsufficientFunds),
		errors.Is(err, gemstore.ErrItemNotOwned),
		errors.Is(err, gemstore.ErrCannotSendToSelf),
		errors.Is(err, gemstore.ErrInsufficientMaterials),
		errors.Is(err, gemstore.ErrInvalidCharacterID),
		errors.Is(err, gemstore.ErrInvalidGemID),
		errors.Is(err, gemstore.ErrInvalidRecipeID):
		writeError(w, http.StatusBadRequest, err)
	case errors.Is(err, corecharacter.ErrNotFound):
		writeError(w, http.StatusNotFound, err)
	default:
		writeError(w, http.StatusInternalServerError, err)
	}
}
