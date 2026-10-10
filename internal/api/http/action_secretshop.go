package http

import (
	"context"
	"errors"
	"net/http"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/secretshop"
)

// WithSecretShop configures the REST service and its Gateway purchase command.
func WithSecretShop(service SecretShopService) Option {
	return func(h *Handler) {
		h.secretshop = service
		if service == nil {
			withActionCommand[secretShopPurchaseRequest]("secretshop_purchase", nil, nil)(h)
			return
		}
		withActionCommand("secretshop_purchase", func(ctx context.Context, actorID string, params secretShopPurchaseRequest) (any, error) {
			// Read presentation inputs before mutation to preserve a known outcome.
			char, err := h.characters.Get(ctx, actorID)
			if err != nil {
				return nil, err
			}
			result, err := service.PurchaseItem(ctx, actorID, params.ItemID, params.Quantity)
			if err != nil {
				return nil, err
			}
			return toSecretShopPurchaseResponse(result, char.Name), nil
		}, secretShopActionRejection)(h)
	}
}

func secretShopActionRejection(err error) (int, ErrorDetail) {
	switch {
	case errors.Is(err, secretshop.ErrAccessDenied):
		return http.StatusForbidden, ErrorDetail{Code: "SECRETSHOP_ACCESS_DENIED", Message: "Secret shop access denied."}
	case errors.Is(err, secretshop.ErrCharacterNotFound), errors.Is(err, corecharacter.ErrNotFound):
		return http.StatusNotFound, ErrorDetail{Code: "SECRETSHOP_CHARACTER_NOT_FOUND", Message: "Character not found."}
	case errors.Is(err, secretshop.ErrItemNotFound):
		return http.StatusNotFound, ErrorDetail{Code: "SECRETSHOP_ITEM_NOT_FOUND", Message: "Secret shop item not found."}
	case errors.Is(err, secretshop.ErrItemUnavailableInHelperQuest):
		return http.StatusConflict, ErrorDetail{Code: "SECRETSHOP_ITEM_UNAVAILABLE", Message: "Item unavailable during an active helper request."}
	case errors.Is(err, secretshop.ErrInvalidQuantity):
		return http.StatusBadRequest, ErrorDetail{Code: "SECRETSHOP_INVALID_QUANTITY", Message: "Secret shop quantity must be between 1 and 99."}
	case errors.Is(err, secretshop.ErrInsufficientFunds):
		return http.StatusBadRequest, ErrorDetail{Code: "SECRETSHOP_INSUFFICIENT_FUNDS", Message: "Insufficient wallet funds."}
	case errors.Is(err, secretshop.ErrPriceOverflow):
		return http.StatusBadRequest, ErrorDetail{Code: "SECRETSHOP_PRICE_OVERFLOW", Message: "Secret shop total price exceeds the supported range."}
	case errors.Is(err, secretshop.ErrDepotFull):
		return http.StatusBadRequest, ErrorDetail{Code: "SECRETSHOP_DEPOT_FULL", Message: "Depot is full."}
	default:
		return 0, ErrorDetail{}
	}
}
