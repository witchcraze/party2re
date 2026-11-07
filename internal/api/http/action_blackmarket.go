package http

import (
	"context"
	"errors"
	"net/http"

	"github.com/witchcraze/party2re/internal/blackmarket"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

type blackMarketSacrificeRequest struct {
	ItemInstanceID string `json:"item_instance_id"`
}

type blackMarketTradeRequest struct {
	PrizeID string `json:"prize_id"`
}

// WithBlackMarket configures the scene observation, retained NPC reads and two barter commands.
func WithBlackMarket(service BlackMarketService) Option {
	return func(h *Handler) {
		h.blackmarket = service
		if service == nil {
			withActionCommand[blackMarketSacrificeRequest]("blackmarket_sacrifice", nil, nil)(h)
			withActionCommand[blackMarketTradeRequest]("blackmarket_trade", nil, nil)(h)
			return
		}
		withActionCommand("blackmarket_sacrifice", func(ctx context.Context, actor string, p blackMarketSacrificeRequest) (any, error) {
			return service.SacrificeItem(ctx, actor, p.ItemInstanceID)
		}, blackMarketActionRejection)(h)
		withActionCommand("blackmarket_trade", func(ctx context.Context, actor string, p blackMarketTradeRequest) (any, error) {
			return service.TradePrize(ctx, actor, p.PrizeID)
		}, blackMarketActionRejection)(h)
	}
}

func blackMarketActionRejection(err error) (int, ErrorDetail) {
	switch {
	case errors.Is(err, blackmarket.ErrAccessDenied):
		return http.StatusForbidden, ErrorDetail{Code: "BLACKMARKET_ACCESS_DENIED", Message: "Black market access denied."}
	case errors.Is(err, blackmarket.ErrCharacterNotFound), errors.Is(err, corecharacter.ErrNotFound):
		return http.StatusNotFound, ErrorDetail{Code: "CHARACTER_NOT_FOUND", Message: "Character not found."}
	case errors.Is(err, blackmarket.ErrPrizeNotFound):
		return http.StatusNotFound, ErrorDetail{Code: "BLACKMARKET_PRIZE_NOT_FOUND", Message: "Prize not found."}
	case errors.Is(err, blackmarket.ErrUnownedItem):
		return http.StatusBadRequest, ErrorDetail{Code: "BLACKMARKET_ITEM_NOT_OWNED", Message: "Item is not in your inventory or depot."}
	case errors.Is(err, blackmarket.ErrNotSacrificeEligible):
		return http.StatusBadRequest, ErrorDetail{Code: "BLACKMARKET_ITEM_NOT_ELIGIBLE", Message: "Item is not eligible for sacrifice."}
	case errors.Is(err, blackmarket.ErrInsufficientRarePoints):
		return http.StatusBadRequest, ErrorDetail{Code: "BLACKMARKET_INSUFFICIENT_RARE_POINTS", Message: "Insufficient rare points."}
	case errors.Is(err, blackmarket.ErrInsufficientURarePoints):
		return http.StatusBadRequest, ErrorDetail{Code: "BLACKMARKET_INSUFFICIENT_U_RARE_POINTS", Message: "Insufficient U-rare points."}
	case errors.Is(err, blackmarket.ErrDepotFull):
		return http.StatusBadRequest, ErrorDetail{Code: "BLACKMARKET_DEPOT_FULL", Message: "Depot is full."}
	default:
		return 0, ErrorDetail{}
	}
}
