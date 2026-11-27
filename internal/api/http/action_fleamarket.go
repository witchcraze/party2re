package http

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/fleamarket"
)

// FleaMarketService is the Flea Market surface used by the scene and commands.
type FleaMarketService interface {
	CreateListing(ctx context.Context, sellerCharacterID, itemInstanceOrDefID string, price int, now time.Time) (fleamarket.Listing, error)
	PurchaseListing(ctx context.Context, buyerCharacterID, listingID string, now time.Time) (fleamarket.PurchaseResult, error)
	CancelListing(ctx context.Context, sellerCharacterID, listingID string) (fleamarket.Listing, error)
	ListActiveListings(ctx context.Context, limit, offset int) ([]fleamarket.Listing, int, error)
	GetActiveCharacterListings(ctx context.Context, characterID string) ([]fleamarket.Listing, error)
}

type fleaMarketListRequest struct {
	ItemID string `json:"item_id"`
	Price  int    `json:"price"`
}

type fleaMarketListingRequest struct {
	ListingID string `json:"listing_id"`
}

// WithFleaMarket configures the scene observation and the three listing commands.
func WithFleaMarket(service FleaMarketService) Option {
	return func(h *Handler) {
		h.fleamarket = service
		if service == nil {
			withEchoCommand[fleaMarketListRequest, fleaMarketListingResult]("fleamarket_list", nil, nil)(h)
			withEchoCommand[fleaMarketListingRequest, fleaMarketPurchaseResult]("fleamarket_purchase", nil, nil)(h)
			withEchoCommand[fleaMarketListingRequest, fleaMarketListingResult]("fleamarket_cancel", nil, nil)(h)
			return
		}
		withEchoCommand("fleamarket_list", func(ctx context.Context, actor string, p fleaMarketListRequest) (fleaMarketListingResult, error) {
			listing, err := service.CreateListing(ctx, actor, p.ItemID, p.Price, time.Now().UTC())
			if err != nil {
				return fleaMarketListingResult{}, err
			}
			reply := fmt.Sprintf("%sを%d Gで出品しとくぞ", listing.ItemName, listing.Price)
			return fleaMarketListingResult{listing, commandEcho{target: fmt.Sprintf("%s＠ねだん>%d", listing.ItemName, listing.Price), reply: reply}}, nil
		}, fleaMarketActionRejection)(h)
		// Legacy names a listing by its display number; Go listings have none, so
		// the actor rows of purchase and cancel name the item instead.
		withEchoCommand("fleamarket_purchase", func(ctx context.Context, actor string, p fleaMarketListingRequest) (fleaMarketPurchaseResult, error) {
			res, err := service.PurchaseListing(ctx, actor, p.ListingID, time.Now().UTC())
			if err != nil {
				return fleaMarketPurchaseResult{}, err
			}
			return fleaMarketPurchaseResult{res, commandEcho{target: res.Listing.ItemName, reply: "まいどあり"}}, nil
		}, fleaMarketActionRejection)(h)
		withEchoCommand("fleamarket_cancel", func(ctx context.Context, actor string, p fleaMarketListingRequest) (fleaMarketListingResult, error) {
			listing, err := service.CancelListing(ctx, actor, p.ListingID)
			if err != nil {
				return fleaMarketListingResult{}, err
			}
			return fleaMarketListingResult{listing, commandEcho{target: listing.ItemName, reply: "そら、" + listing.ItemName + "だ"}}, nil
		}, fleaMarketActionRejection)(h)
	}
}

// Flea Market results keep their service shape and add the place-log echo
// (free.cgi:90,163,229).
type fleaMarketListingResult struct {
	fleamarket.Listing
	commandEcho
}

type fleaMarketPurchaseResult struct {
	fleamarket.PurchaseResult
	commandEcho
}

func fleaMarketActionRejection(err error) (int, ErrorDetail) {
	switch {
	case errors.Is(err, fleamarket.ErrCharacterNotFound), errors.Is(err, corecharacter.ErrNotFound):
		return http.StatusNotFound, ErrorDetail{Code: "CHARACTER_NOT_FOUND", Message: "Character not found."}
	case errors.Is(err, fleamarket.ErrListingNotFound):
		return http.StatusNotFound, ErrorDetail{Code: "FLEAMARKET_LISTING_NOT_FOUND", Message: "Listing not found."}
	case errors.Is(err, fleamarket.ErrUnauthorizedSeller):
		return http.StatusForbidden, ErrorDetail{Code: "FLEAMARKET_NOT_SELLER", Message: "Only the seller can cancel this listing."}
	case errors.Is(err, fleamarket.ErrInvalidInput):
		return http.StatusBadRequest, ErrorDetail{Code: "FLEAMARKET_INVALID_INPUT", Message: "Item or listing ID is required."}
	case errors.Is(err, fleamarket.ErrInvalidPrice):
		return http.StatusBadRequest, ErrorDetail{Code: "FLEAMARKET_INVALID_PRICE", Message: "Price must be between 1 and 999999 gold."}
	case errors.Is(err, fleamarket.ErrListingNotActive):
		return http.StatusBadRequest, ErrorDetail{Code: "FLEAMARKET_LISTING_NOT_ACTIVE", Message: "Listing is no longer active."}
	case errors.Is(err, fleamarket.ErrCannotBuyOwnListing):
		return http.StatusBadRequest, ErrorDetail{Code: "FLEAMARKET_OWN_LISTING", Message: "You cannot buy your own listing."}
	case errors.Is(err, fleamarket.ErrInsufficientGold):
		return http.StatusBadRequest, ErrorDetail{Code: "FLEAMARKET_INSUFFICIENT_GOLD", Message: "Insufficient gold."}
	case errors.Is(err, fleamarket.ErrMaxListingsReached):
		return http.StatusBadRequest, ErrorDetail{Code: "FLEAMARKET_MAX_LISTINGS", Message: "Your listing limit has been reached."}
	case errors.Is(err, fleamarket.ErrServerMaxListingsReached):
		return http.StatusBadRequest, ErrorDetail{Code: "FLEAMARKET_SERVER_MAX_LISTINGS", Message: "The market listing limit has been reached."}
	case errors.Is(err, fleamarket.ErrItemNotInDepot):
		return http.StatusBadRequest, ErrorDetail{Code: "FLEAMARKET_ITEM_NOT_IN_DEPOT", Message: "Item is not in your depot."}
	default:
		return 0, ErrorDetail{}
	}
}
