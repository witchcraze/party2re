package http

import (
	"context"
	"errors"
	"net/http"

	"github.com/witchcraze/party2re/internal/collection"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
)

// CollectionService defines the monster encyclopedia and item collection operations exposed over HTTP.
type CollectionService interface {
	GetMonsterBook(ctx context.Context, characterID string) ([]collection.MonsterBookEntry, collection.CompletionProgress, error)
	GetItemCollection(ctx context.Context, characterID, category string) ([]collection.ItemCollectionEntry, collection.CompletionProgress, error)
}

// WithCollection configures the collection service for the Handler.
func WithCollection(c CollectionService) Option {
	return func(h *Handler) {
		h.collections = c
	}
}

type monsterBookResponse struct {
	Entries  []collection.MonsterBookEntry `json:"entries"`
	Progress collection.CompletionProgress `json:"progress"`
}

type itemCollectionResponse struct {
	Entries  []collection.ItemCollectionEntry `json:"entries"`
	Progress collection.CompletionProgress    `json:"progress"`
}

func (h *Handler) handleGetMonsterBook(w http.ResponseWriter, r *http.Request) {
	if h.collections == nil {
		writeError(w, http.StatusNotImplemented, errors.New("collection service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		entries, progress, err := h.collections.GetMonsterBook(r.Context(), char.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		writeJSON(w, http.StatusOK, monsterBookResponse{
			Entries:  entries,
			Progress: progress,
		})
	})
}

func (h *Handler) handleGetItemCollection(w http.ResponseWriter, r *http.Request) {
	if h.collections == nil {
		writeError(w, http.StatusNotImplemented, errors.New("collection service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		category := r.URL.Query().Get("category")
		entries, progress, err := h.collections.GetItemCollection(r.Context(), char.ID, category)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		writeJSON(w, http.StatusOK, itemCollectionResponse{
			Entries:  entries,
			Progress: progress,
		})
	})
}
