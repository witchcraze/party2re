package http

import (
	"context"
	"errors"
	"net/http"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/eventplaza"
)

// EventPlazaService defines town event plaza and traveling merchant operations over HTTP.
type EventPlazaService interface {
	GetPlazaStatus(ctx context.Context) (eventplaza.PlazaStatus, error)
	ListAvailableBazaarItems(ctx context.Context) ([]eventplaza.BazaarItem, int, error)
	PurchaseBazaarItem(ctx context.Context, characterID string, itemID string, quantity int) (eventplaza.BazaarPurchaseResult, error)
	ListActiveBanquets(ctx context.Context) ([]eventplaza.CelebrationBanquet, error)
	ToastBanquet(ctx context.Context, banquetID string, characterID string) (eventplaza.BanquetToastResult, error)
}

// WithEventPlaza configures the EventPlazaService for the HTTP handler.
func WithEventPlaza(service EventPlazaService) Option {
	return func(h *Handler) {
		h.eventplaza = service
	}
}

type getBazaarItemsResponse struct {
	MerchantTier     int                     `json:"merchant_tier"`
	MerchantTierName string                  `json:"merchant_tier_name"`
	Items            []eventplaza.BazaarItem `json:"items"`
}

type purchaseBazaarItemRequest struct {
	CharacterID string `json:"character_id"`
	ItemID      string `json:"item_id"`
	Quantity    int    `json:"quantity"`
}

type listBanquetsResponse struct {
	Banquets []eventplaza.CelebrationBanquet `json:"banquets"`
	Total    int                             `json:"total"`
}

type toastBanquetRequest struct {
	CharacterID string `json:"character_id"`
}

func (h *Handler) handleGetEventPlaza(w http.ResponseWriter, r *http.Request) {
	if h.eventplaza == nil {
		writeError(w, http.StatusNotImplemented, errors.New("event plaza service not configured"))
		return
	}

	status, err := h.eventplaza.GetPlazaStatus(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusOK, status)
}

func (h *Handler) handleGetEventPlazaMerchantItems(w http.ResponseWriter, r *http.Request) {
	if h.eventplaza == nil {
		writeError(w, http.StatusNotImplemented, errors.New("event plaza service not configured"))
		return
	}

	items, tier, err := h.eventplaza.ListAvailableBazaarItems(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	_, tierName, _ := eventplaza.CalculateMerchantTier(0)
	if tier > 0 {
		_, tierName, _ = eventplaza.CalculateMerchantTier(tier * 10)
	}

	if items == nil {
		items = []eventplaza.BazaarItem{}
	}

	writeJSON(w, http.StatusOK, getBazaarItemsResponse{
		MerchantTier:     tier,
		MerchantTierName: tierName,
		Items:            items,
	})
}

func (h *Handler) handlePostEventPlazaMerchantPurchase(w http.ResponseWriter, r *http.Request) {
	if h.eventplaza == nil {
		writeError(w, http.StatusNotImplemented, errors.New("event plaza service not configured"))
		return
	}

	withAuthenticatedCharacterAndJSON(h, w, r, func(req *purchaseBazaarItemRequest) string {
		return req.CharacterID
	}, func(_ coreplayer.Player, char corecharacter.Character, req purchaseBazaarItemRequest) {
		if req.Quantity <= 0 {
			req.Quantity = 1
		}

		result, err := h.eventplaza.PurchaseBazaarItem(r.Context(), char.ID, req.ItemID, req.Quantity)
		if err != nil {
			switch {
			case errors.Is(err, eventplaza.ErrCharacterNotFound):
				writeError(w, http.StatusNotFound, err)
			case errors.Is(err, eventplaza.ErrItemNotFound):
				writeError(w, http.StatusNotFound, err)
			case errors.Is(err, eventplaza.ErrInsufficientGold):
				writeError(w, http.StatusBadRequest, err)
			case errors.Is(err, eventplaza.ErrItemTierLocked):
				writeError(w, http.StatusBadRequest, err)
			case errors.Is(err, eventplaza.ErrInvalidQuantity):
				writeError(w, http.StatusBadRequest, err)
			case errors.Is(err, eventplaza.ErrPriceOverflow):
				writeError(w, http.StatusBadRequest, err)
			default:
				writeError(w, http.StatusInternalServerError, err)
			}
			return
		}

		writeJSON(w, http.StatusOK, result)
	})
}

func (h *Handler) handleGetEventPlazaBanquets(w http.ResponseWriter, r *http.Request) {
	if h.eventplaza == nil {
		writeError(w, http.StatusNotImplemented, errors.New("event plaza service not configured"))
		return
	}

	banquets, err := h.eventplaza.ListActiveBanquets(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	if banquets == nil {
		banquets = []eventplaza.CelebrationBanquet{}
	}

	writeJSON(w, http.StatusOK, listBanquetsResponse{
		Banquets: banquets,
		Total:    len(banquets),
	})
}

func (h *Handler) handlePostEventPlazaBanquetToast(w http.ResponseWriter, r *http.Request) {
	if h.eventplaza == nil {
		writeError(w, http.StatusNotImplemented, errors.New("event plaza service not configured"))
		return
	}

	banquetID := r.PathValue("id")
	if banquetID == "" {
		writeError(w, http.StatusBadRequest, errors.New("banquet id is required"))
		return
	}

	withAuthenticatedCharacterAndJSON(h, w, r, func(req *toastBanquetRequest) string {
		return req.CharacterID
	}, func(_ coreplayer.Player, char corecharacter.Character, req toastBanquetRequest) {
		result, err := h.eventplaza.ToastBanquet(r.Context(), banquetID, char.ID)
		if err != nil {
			switch {
			case errors.Is(err, eventplaza.ErrBanquetNotFound):
				writeError(w, http.StatusNotFound, err)
			case errors.Is(err, eventplaza.ErrCharacterNotFound):
				writeError(w, http.StatusNotFound, err)
			case errors.Is(err, eventplaza.ErrAlreadyToasted):
				writeError(w, http.StatusConflict, err)
			case errors.Is(err, eventplaza.ErrBanquetExpired):
				writeError(w, http.StatusGone, err)
			default:
				writeError(w, http.StatusInternalServerError, err)
			}
			return
		}

		writeJSON(w, http.StatusOK, result)
	})
}
