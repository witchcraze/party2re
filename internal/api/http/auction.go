package http

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/witchcraze/party2re/internal/auction"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/pagination"
)

// AuctionService defines the auction house operations exposed over HTTP.
type AuctionService interface {
	CreateListing(ctx context.Context, sellerID, itemID, itemName, itemCategory string, enhancement int, startBid, buyoutPrice int, duration time.Duration) (auction.AuctionListing, error)
	GetListing(ctx context.Context, listingID string) (auction.AuctionListing, error)
	ListActive(ctx context.Context, limit, offset int) (pagination.Page[auction.AuctionListing], error)
	PlaceBid(ctx context.Context, listingID, bidderID string, bidAmount int) (auction.AuctionListing, error)
	Buyout(ctx context.Context, listingID, buyerID string) (auction.AuctionListing, error)
	CancelListing(ctx context.Context, listingID, sellerID string) (auction.AuctionListing, error)
}

// WithAuction configures the auction house service for the Handler.
func WithAuction(a AuctionService) Option {
	return func(h *Handler) {
		h.auctions = a
	}
}

type auctionListingResponse struct {
	Listing auction.AuctionListing `json:"listing"`
}

type createAuctionRequest struct {
	SellerCharacterID string `json:"seller_character_id"`
	ItemID            string `json:"item_id"`
	ItemName          string `json:"item_name"`
	ItemCategory      string `json:"item_category"`
	EnhancementLevel  int    `json:"enhancement_level"`
	StartBid          int    `json:"start_bid"`
	BuyoutPrice       int    `json:"buyout_price"`
	DurationHours     int    `json:"duration_hours,omitempty"`
}

type placeBidRequest struct {
	BidderCharacterID string `json:"bidder_character_id"`
	BidAmount         int    `json:"bid_amount"`
}

type buyoutAuctionRequest struct {
	BuyerCharacterID string `json:"buyer_character_id"`
}

type cancelAuctionRequest struct {
	SellerCharacterID string `json:"seller_character_id"`
}

func (h *Handler) handleListAuctions(w http.ResponseWriter, r *http.Request) {
	if h.auctions == nil {
		writeError(w, http.StatusNotImplemented, errors.New("auction service not configured"))
		return
	}

	params := pagination.ParseRequest(r)

	page, err := h.auctions.ListActive(r.Context(), params.Limit, params.Offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusOK, page)
}

func (h *Handler) handleGetAuction(w http.ResponseWriter, r *http.Request) {
	if h.auctions == nil {
		writeError(w, http.StatusNotImplemented, errors.New("auction service not configured"))
		return
	}

	auctionID := r.PathValue("id")
	listing, err := h.auctions.GetListing(r.Context(), auctionID)
	if err != nil {
		if errors.Is(err, auction.ErrListingNotFound) {
			writeError(w, http.StatusNotFound, err)
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusOK, auctionListingResponse{
		Listing: listing,
	})
}

func (h *Handler) handleCreateAuction(w http.ResponseWriter, r *http.Request) {
	if h.auctions == nil {
		writeError(w, http.StatusNotImplemented, errors.New("auction service not configured"))
		return
	}

	withAuthenticatedCharacterAndJSON(h, w, r, func(req *createAuctionRequest) string {
		return req.SellerCharacterID
	}, func(_ coreplayer.Player, char corecharacter.Character, req createAuctionRequest) {
		duration := 24 * time.Hour
		if req.DurationHours > 0 {
			duration = time.Duration(req.DurationHours) * time.Hour
		}

		listing, err := h.auctions.CreateListing(
			r.Context(),
			char.ID,
			req.ItemID,
			req.ItemName,
			req.ItemCategory,
			req.EnhancementLevel,
			req.StartBid,
			req.BuyoutPrice,
			duration,
		)
		if err != nil {
			if errors.Is(err, auction.ErrInvalidPricing) {
				writeError(w, http.StatusBadRequest, err)
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		writeJSON(w, http.StatusCreated, auctionListingResponse{
			Listing: listing,
		})
	})
}

func (h *Handler) handleAuctionBid(w http.ResponseWriter, r *http.Request) {
	if h.auctions == nil {
		writeError(w, http.StatusNotImplemented, errors.New("auction service not configured"))
		return
	}

	auctionID := r.PathValue("id")
	withAuthenticatedCharacterAndJSON(h, w, r, func(req *placeBidRequest) string {
		return req.BidderCharacterID
	}, func(_ coreplayer.Player, char corecharacter.Character, req placeBidRequest) {
		listing, err := h.auctions.PlaceBid(r.Context(), auctionID, char.ID, req.BidAmount)
		if err != nil {
			if errors.Is(err, auction.ErrListingNotFound) {
				writeError(w, http.StatusNotFound, err)
				return
			}
			if errors.Is(err, auction.ErrInvalidBidAmount) || errors.Is(err, auction.ErrSellerCannotBid) {
				writeError(w, http.StatusBadRequest, err)
				return
			}
			if errors.Is(err, auction.ErrListingNotActive) || errors.Is(err, auction.ErrListingExpired) || errors.Is(err, auction.ErrInsufficientGold) {
				writeError(w, http.StatusUnprocessableEntity, err)
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		writeJSON(w, http.StatusOK, auctionListingResponse{
			Listing: listing,
		})
	})
}

func (h *Handler) handleAuctionBuyout(w http.ResponseWriter, r *http.Request) {
	if h.auctions == nil {
		writeError(w, http.StatusNotImplemented, errors.New("auction service not configured"))
		return
	}

	auctionID := r.PathValue("id")
	withAuthenticatedCharacterAndJSON(h, w, r, func(req *buyoutAuctionRequest) string {
		return req.BuyerCharacterID
	}, func(_ coreplayer.Player, char corecharacter.Character, req buyoutAuctionRequest) {
		listing, err := h.auctions.Buyout(r.Context(), auctionID, char.ID)
		if err != nil {
			if errors.Is(err, auction.ErrListingNotFound) {
				writeError(w, http.StatusNotFound, err)
				return
			}
			if errors.Is(err, auction.ErrNoBuyoutPrice) || errors.Is(err, auction.ErrSellerCannotBid) {
				writeError(w, http.StatusBadRequest, err)
				return
			}
			if errors.Is(err, auction.ErrListingNotActive) || errors.Is(err, auction.ErrListingExpired) || errors.Is(err, auction.ErrInsufficientGold) {
				writeError(w, http.StatusUnprocessableEntity, err)
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		writeJSON(w, http.StatusOK, auctionListingResponse{
			Listing: listing,
		})
	})
}

func (h *Handler) handleAuctionCancel(w http.ResponseWriter, r *http.Request) {
	if h.auctions == nil {
		writeError(w, http.StatusNotImplemented, errors.New("auction service not configured"))
		return
	}

	auctionID := r.PathValue("id")
	withAuthenticatedCharacterAndJSON(h, w, r, func(req *cancelAuctionRequest) string {
		return req.SellerCharacterID
	}, func(_ coreplayer.Player, char corecharacter.Character, req cancelAuctionRequest) {
		listing, err := h.auctions.CancelListing(r.Context(), auctionID, char.ID)
		if err != nil {
			if errors.Is(err, auction.ErrListingNotFound) {
				writeError(w, http.StatusNotFound, err)
				return
			}
			if errors.Is(err, auction.ErrUnauthorizedSeller) {
				writeError(w, http.StatusForbidden, err)
				return
			}
			if errors.Is(err, auction.ErrCannotCancelWithBids) || errors.Is(err, auction.ErrListingNotActive) {
				writeError(w, http.StatusUnprocessableEntity, err)
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		writeJSON(w, http.StatusOK, auctionListingResponse{
			Listing: listing,
		})
	})
}
