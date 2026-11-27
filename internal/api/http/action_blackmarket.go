package http

import (
	"context"
	"errors"
	"fmt"
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

// BlackMarketSacrificeResult and BlackMarketTradeResult add the dealer's reply
// (black_market.cgi:119,138,188-259).
type BlackMarketSacrificeResult struct {
	*blackmarket.SacrificeResult
	Message string `json:"message"`
	commandEcho
}

type BlackMarketTradeResult struct {
	*blackmarket.TradeResult
	Message string `json:"message"`
	commandEcho
}

// Legacy appends one reply per list the item is on.
func blackMarketSacrificeReply(res *blackmarket.SacrificeResult) BlackMarketSacrificeResult {
	msg := ""
	if res.RarePointsGained > 0 {
		msg += "…" + res.ItemName + "…か…。レアだな…。いいだろう…。お前のレアポイントを加算しておこう…"
	}
	if res.URarePointsGained > 0 {
		msg += fmt.Sprintf("これは……! ……いいだろう…。お前の特別なレアポイントを%d加算しておこう…", res.URarePointsGained)
	}
	return BlackMarketSacrificeResult{res, msg, commandEcho{target: res.ItemName, reply: msg}}
}

// WithBlackMarket configures the scene observation, retained NPC reads and two barter commands.
func WithBlackMarket(service BlackMarketService) Option {
	return func(h *Handler) {
		h.blackmarket = service
		if service == nil {
			withEchoCommand[blackMarketSacrificeRequest, BlackMarketSacrificeResult]("blackmarket_sacrifice", nil, nil)(h)
			withEchoCommand[blackMarketTradeRequest, BlackMarketTradeResult]("blackmarket_trade", nil, nil)(h)
			return
		}
		withEchoCommand("blackmarket_sacrifice", func(ctx context.Context, actor string, p blackMarketSacrificeRequest) (BlackMarketSacrificeResult, error) {
			res, err := service.SacrificeItem(ctx, actor, p.ItemInstanceID)
			if err != nil {
				return BlackMarketSacrificeResult{}, err
			}
			return blackMarketSacrificeReply(res), nil
		}, blackMarketActionRejection)(h)
		withEchoCommand("blackmarket_trade", func(ctx context.Context, actor string, p blackMarketTradeRequest) (BlackMarketTradeResult, error) {
			res, err := service.TradePrize(ctx, actor, p.PrizeID)
			if err != nil {
				return BlackMarketTradeResult{}, err
			}
			reply := "取引成立だ…。" + res.ItemName + " はお前の預かり所に送っておいた…"
			return BlackMarketTradeResult{res, reply, commandEcho{target: res.ItemName, reply: reply}}, nil
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
	default:
		return 0, ErrorDetail{}
	}
}
