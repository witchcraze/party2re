package auction_test

import (
	"context"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/auction"
)

type mockAuctionRepo struct {
	createFn func(ctx context.Context, listing auction.AuctionListing) (auction.AuctionListing, error)
	getFn    func(ctx context.Context, id string) (auction.AuctionListing, error)
	listFn   func(ctx context.Context, limit, offset int) ([]auction.AuctionListing, error)
	bidFn    func(ctx context.Context, id, bidder string, amount int) (auction.AuctionListing, error)
	buyoutFn func(ctx context.Context, id, buyer string) (auction.AuctionListing, error)
	settleFn func(ctx context.Context, id string) (auction.AuctionListing, error)
	cancelFn func(ctx context.Context, id, seller string) (auction.AuctionListing, error)
}

func (m *mockAuctionRepo) CreateListing(ctx context.Context, listing auction.AuctionListing) (auction.AuctionListing, error) {
	if m.createFn != nil {
		return m.createFn(ctx, listing)
	}
	return listing, nil
}
func (m *mockAuctionRepo) GetListing(ctx context.Context, id string) (auction.AuctionListing, error) {
	if m.getFn != nil {
		return m.getFn(ctx, id)
	}
	return auction.AuctionListing{}, auction.ErrListingNotFound
}
func (m *mockAuctionRepo) ListActive(ctx context.Context, limit, offset int) ([]auction.AuctionListing, error) {
	if m.listFn != nil {
		return m.listFn(ctx, limit, offset)
	}
	return nil, nil
}
func (m *mockAuctionRepo) PlaceBid(ctx context.Context, id, bidder string, amount int) (auction.AuctionListing, error) {
	if m.bidFn != nil {
		return m.bidFn(ctx, id, bidder, amount)
	}
	return auction.AuctionListing{}, nil
}
func (m *mockAuctionRepo) Buyout(ctx context.Context, id, buyer string) (auction.AuctionListing, error) {
	if m.buyoutFn != nil {
		return m.buyoutFn(ctx, id, buyer)
	}
	return auction.AuctionListing{}, nil
}
func (m *mockAuctionRepo) SettleListing(ctx context.Context, id string) (auction.AuctionListing, error) {
	if m.settleFn != nil {
		return m.settleFn(ctx, id)
	}
	return auction.AuctionListing{}, nil
}
func (m *mockAuctionRepo) CancelListing(ctx context.Context, id, seller string) (auction.AuctionListing, error) {
	if m.cancelFn != nil {
		return m.cancelFn(ctx, id, seller)
	}
	return auction.AuctionListing{}, nil
}

func TestAuctionService_Validation(t *testing.T) {
	ctx := context.Background()
	svc, _ := auction.NewService(&mockAuctionRepo{})

	// 1. Invalid Pricing
	if _, err := svc.CreateListing(ctx, "seller1", "item1", "Sword", "WEAPON", 0, 0, 100, 24*time.Hour); err != auction.ErrInvalidPricing {
		t.Errorf("expected ErrInvalidPricing for 0 start bid, got %v", err)
	}
	if _, err := svc.CreateListing(ctx, "seller1", "item1", "Sword", "WEAPON", 0, 500, 200, 24*time.Hour); err != auction.ErrInvalidPricing {
		t.Errorf("expected ErrInvalidPricing for buyout < startBid, got %v", err)
	}
}

func TestAuctionService_BiddingFlow(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	listing := auction.AuctionListing{
		ID:                "auc1",
		SellerCharacterID: "seller1",
		StartBid:          100,
		CurrentBid:        0,
		BuyoutPrice:       1000,
		Status:            auction.StatusActive,
		ExpiresAt:         now.Add(time.Hour),
	}

	repo := &mockAuctionRepo{
		getFn: func(_ context.Context, id string) (auction.AuctionListing, error) {
			return listing, nil
		},
		bidFn: func(_ context.Context, id, bidder string, amount int) (auction.AuctionListing, error) {
			listing.CurrentBid = amount
			listing.HighestBidderID = &bidder
			return listing, nil
		},
	}
	svc, _ := auction.NewService(repo)

	// Seller cannot bid on own listing
	if _, err := svc.PlaceBid(ctx, "seller1", "auc1", 200); err != auction.ErrSellerCannotBid {
		t.Errorf("expected ErrSellerCannotBid, got %v", err)
	}

	// Bid below start bid
	if _, err := svc.PlaceBid(ctx, "bidder1", "auc1", 50); err != auction.ErrInvalidBidAmount {
		t.Errorf("expected ErrInvalidBidAmount, got %v", err)
	}

	// Valid bid
	updated, err := svc.PlaceBid(ctx, "bidder1", "auc1", 200)
	if err != nil {
		t.Fatalf("PlaceBid failed: %v", err)
	}
	if updated.CurrentBid != 200 || *updated.HighestBidderID != "bidder1" {
		t.Errorf("current bid = %d, highest bidder = %v", updated.CurrentBid, *updated.HighestBidderID)
	}

	// Lower or equal bid from another bidder fails
	if _, err := svc.PlaceBid(ctx, "bidder2", "auc1", 200); err != auction.ErrInvalidBidAmount {
		t.Errorf("expected ErrInvalidBidAmount for duplicate/lower bid, got %v", err)
	}
}
