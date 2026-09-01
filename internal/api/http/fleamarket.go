package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/fleamarket"
	"github.com/witchcraze/party2re/internal/pagination"
)

// FleaMarketService defines the flea market operations exposed over HTTP.
type FleaMarketService interface {
	CreateListing(ctx context.Context, sellerCharacterID, itemInstanceOrDefID string, price int, now time.Time) (fleamarket.Listing, error)
	PurchaseListing(ctx context.Context, buyerCharacterID, listingID string, now time.Time) (fleamarket.PurchaseResult, error)
	CancelListing(ctx context.Context, sellerCharacterID, listingID string) (fleamarket.Listing, error)
	ListActiveListings(ctx context.Context, limit, offset int) ([]fleamarket.Listing, int, error)
	GetListing(ctx context.Context, listingID string) (fleamarket.Listing, error)
	GetCharacterListings(ctx context.Context, characterID string) ([]fleamarket.Listing, error)
}

// WithFleaMarket configures the FleaMarketService for the HTTP handler.
func WithFleaMarket(service FleaMarketService) Option {
	return func(h *Handler) {
		h.fleamarket = service
	}
}

type createFleaMarketListingRequest struct {
	ItemID string `json:"item_id"`
	Price  int    `json:"price"`
}

func (h *Handler) handleListFleaMarketListings(w http.ResponseWriter, r *http.Request) {
	if h.fleamarket == nil {
		writeError(w, http.StatusNotImplemented, errors.New("fleamarket service not configured"))
		return
	}

	params := pagination.ParseRequest(r)

	listings, total, err := h.fleamarket.ListActiveListings(r.Context(), params.Limit, params.Offset)
	if err != nil {
		h.writeFleaMarketError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, pagination.NewPage(listings, total, params.Limit, params.Offset))
}

func (h *Handler) handleGetFleaMarketListing(w http.ResponseWriter, r *http.Request) {
	if h.fleamarket == nil {
		writeError(w, http.StatusNotImplemented, errors.New("fleamarket service not configured"))
		return
	}

	listingID := r.PathValue("listing_id")
	listing, err := h.fleamarket.GetListing(r.Context(), listingID)
	if err != nil {
		h.writeFleaMarketError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, listing)
}

func (h *Handler) handleGetCharacterFleaMarketListings(w http.ResponseWriter, r *http.Request) {
	if h.fleamarket == nil {
		writeError(w, http.StatusNotImplemented, errors.New("fleamarket service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		listings, err := h.fleamarket.GetCharacterListings(r.Context(), char.ID)
		if err != nil {
			h.writeFleaMarketError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, listings)
	})
}

func (h *Handler) handleCreateFleaMarketListing(w http.ResponseWriter, r *http.Request) {
	if h.fleamarket == nil {
		writeError(w, http.StatusNotImplemented, errors.New("fleamarket service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		var req createFleaMarketListingRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, errors.New("invalid request body"))
			return
		}

		listing, err := h.fleamarket.CreateListing(r.Context(), char.ID, req.ItemID, req.Price, time.Now().UTC())
		if err != nil {
			h.writeFleaMarketError(w, err)
			return
		}

		writeJSON(w, http.StatusCreated, listing)
	})
}

func (h *Handler) handlePurchaseFleaMarketListing(w http.ResponseWriter, r *http.Request) {
	if h.fleamarket == nil {
		writeError(w, http.StatusNotImplemented, errors.New("fleamarket service not configured"))
		return
	}

	charID := r.PathValue("id")
	listingID := r.PathValue("listing_id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		res, err := h.fleamarket.PurchaseListing(r.Context(), char.ID, listingID, time.Now().UTC())
		if err != nil {
			h.writeFleaMarketError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, res)
	})
}

func (h *Handler) handleCancelFleaMarketListing(w http.ResponseWriter, r *http.Request) {
	if h.fleamarket == nil {
		writeError(w, http.StatusNotImplemented, errors.New("fleamarket service not configured"))
		return
	}

	charID := r.PathValue("id")
	listingID := r.PathValue("listing_id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		cancelled, err := h.fleamarket.CancelListing(r.Context(), char.ID, listingID)
		if err != nil {
			h.writeFleaMarketError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, cancelled)
	})
}

func (h *Handler) writeFleaMarketError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, fleamarket.ErrListingNotFound),
		errors.Is(err, fleamarket.ErrCharacterNotFound):
		writeError(w, http.StatusNotFound, err)
	case errors.Is(err, fleamarket.ErrForbidden),
		errors.Is(err, fleamarket.ErrUnauthorizedSeller):
		writeError(w, http.StatusForbidden, err)
	case errors.Is(err, fleamarket.ErrListingNotActive),
		errors.Is(err, fleamarket.ErrCannotBuyOwnListing),
		errors.Is(err, fleamarket.ErrInsufficientGold),
		errors.Is(err, fleamarket.ErrMaxListingsReached),
		errors.Is(err, fleamarket.ErrInvalidPrice),
		errors.Is(err, fleamarket.ErrItemNotInInventory),
		errors.Is(err, fleamarket.ErrInvalidInput):
		writeError(w, http.StatusBadRequest, err)
	default:
		writeError(w, http.StatusInternalServerError, err)
	}
}
