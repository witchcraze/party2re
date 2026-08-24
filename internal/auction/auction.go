package auction

import (
	"context"
	"errors"
	"time"
)

type AuctionStatus string

const (
	StatusActive    AuctionStatus = "ACTIVE"
	StatusSold      AuctionStatus = "SOLD"
	StatusExpired   AuctionStatus = "EXPIRED"
	StatusCancelled AuctionStatus = "CANCELLED"
)

var (
	ErrInvalidBidAmount     = errors.New("bid amount must be higher than current bid and start bid")
	ErrListingNotFound      = errors.New("auction listing not found")
	ErrListingNotActive     = errors.New("auction listing is not active")
	ErrListingExpired       = errors.New("auction listing has expired")
	ErrSellerCannotBid      = errors.New("seller cannot bid on own listing")
	ErrNoBuyoutPrice        = errors.New("listing does not have a buyout price")
	ErrCannotCancelWithBids = errors.New("cannot cancel auction with active bids")
	ErrUnauthorizedSeller   = errors.New("only the seller can cancel this auction")
	ErrInsufficientGold     = errors.New("insufficient gold for bid or buyout")
	ErrInvalidPricing       = errors.New("invalid starting bid or buyout price")
)

type AuctionListing struct {
	ID                string        `json:"id"`
	SellerCharacterID string        `json:"seller_character_id"`
	ItemID            string        `json:"item_id"`
	ItemName          string        `json:"item_name"`
	ItemCategory      string        `json:"item_category"`
	EnhancementLevel  int           `json:"enhancement_level"`
	StartBid          int           `json:"start_bid"`
	CurrentBid        int           `json:"current_bid"`
	BuyoutPrice       int           `json:"buyout_price"`
	HighestBidderID   *string       `json:"highest_bidder_id,omitempty"`
	Status            AuctionStatus `json:"status"`
	CreatedAt         time.Time     `json:"created_at"`
	ExpiresAt         time.Time     `json:"expires_at"`
	SettledAt         *time.Time    `json:"settled_at,omitempty"`
}

type Repository interface {
	CreateListing(ctx context.Context, listing AuctionListing) (AuctionListing, error)
	GetListing(ctx context.Context, listingID string) (AuctionListing, error)
	ListActive(ctx context.Context, limit, offset int) ([]AuctionListing, error)
	PlaceBid(ctx context.Context, listingID, bidderID string, bidAmount int) (AuctionListing, error)
	Buyout(ctx context.Context, listingID, buyerID string) (AuctionListing, error)
	SettleListing(ctx context.Context, listingID string) (AuctionListing, error)
	CancelListing(ctx context.Context, listingID, sellerID string) (AuctionListing, error)
}

type Service struct {
	repo Repository
}

func NewService(repo Repository) (*Service, error) {
	if repo == nil {
		return nil, errors.New("repository is required")
	}
	return &Service{repo: repo}, nil
}

func (s *Service) CreateListing(ctx context.Context, sellerID, itemID, itemName, itemCategory string, enhancement int, startBid, buyoutPrice int, duration time.Duration) (AuctionListing, error) {
	if startBid <= 0 {
		return AuctionListing{}, ErrInvalidPricing
	}
	if buyoutPrice > 0 && buyoutPrice < startBid {
		return AuctionListing{}, ErrInvalidPricing
	}
	if duration <= 0 {
		duration = 24 * time.Hour
	}

	now := time.Now().UTC()
	listing := AuctionListing{
		SellerCharacterID: sellerID,
		ItemID:            itemID,
		ItemName:          itemName,
		ItemCategory:      itemCategory,
		EnhancementLevel:  enhancement,
		StartBid:          startBid,
		CurrentBid:        0,
		BuyoutPrice:       buyoutPrice,
		Status:            StatusActive,
		CreatedAt:         now,
		ExpiresAt:         now.Add(duration),
	}

	return s.repo.CreateListing(ctx, listing)
}

func (s *Service) GetListing(ctx context.Context, listingID string) (AuctionListing, error) {
	return s.repo.GetListing(ctx, listingID)
}

func (s *Service) ListActive(ctx context.Context, limit, offset int) ([]AuctionListing, error) {
	if limit <= 0 {
		limit = 50
	}
	return s.repo.ListActive(ctx, limit, offset)
}

func (s *Service) PlaceBid(ctx context.Context, bidderID, listingID string, bidAmount int) (AuctionListing, error) {
	listing, err := s.repo.GetListing(ctx, listingID)
	if err != nil {
		return AuctionListing{}, err
	}
	if listing.Status != StatusActive {
		return AuctionListing{}, ErrListingNotActive
	}
	if time.Now().UTC().After(listing.ExpiresAt) {
		return AuctionListing{}, ErrListingExpired
	}
	if listing.SellerCharacterID == bidderID {
		return AuctionListing{}, ErrSellerCannotBid
	}
	if bidAmount < listing.StartBid || (listing.CurrentBid > 0 && bidAmount <= listing.CurrentBid) {
		return AuctionListing{}, ErrInvalidBidAmount
	}

	// If buyout price is set and bid meets or exceeds buyout, execute buyout
	if listing.BuyoutPrice > 0 && bidAmount >= listing.BuyoutPrice {
		return s.repo.Buyout(ctx, listingID, bidderID)
	}

	return s.repo.PlaceBid(ctx, listingID, bidderID, bidAmount)
}

func (s *Service) Buyout(ctx context.Context, buyerID, listingID string) (AuctionListing, error) {
	listing, err := s.repo.GetListing(ctx, listingID)
	if err != nil {
		return AuctionListing{}, err
	}
	if listing.Status != StatusActive {
		return AuctionListing{}, ErrListingNotActive
	}
	if time.Now().UTC().After(listing.ExpiresAt) {
		return AuctionListing{}, ErrListingExpired
	}
	if listing.SellerCharacterID == buyerID {
		return AuctionListing{}, ErrSellerCannotBid
	}
	if listing.BuyoutPrice <= 0 {
		return AuctionListing{}, ErrNoBuyoutPrice
	}

	return s.repo.Buyout(ctx, listingID, buyerID)
}

func (s *Service) SettleListing(ctx context.Context, listingID string) (AuctionListing, error) {
	return s.repo.SettleListing(ctx, listingID)
}

func (s *Service) CancelListing(ctx context.Context, sellerID, listingID string) (AuctionListing, error) {
	listing, err := s.repo.GetListing(ctx, listingID)
	if err != nil {
		return AuctionListing{}, err
	}
	if listing.SellerCharacterID != sellerID {
		return AuctionListing{}, ErrUnauthorizedSeller
	}
	if listing.HighestBidderID != nil {
		return AuctionListing{}, ErrCannotCancelWithBids
	}
	return s.repo.CancelListing(ctx, listingID, sellerID)
}
