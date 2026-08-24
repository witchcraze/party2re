package database_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/auction"
	"github.com/witchcraze/party2re/internal/database"
)

func TestAuctionRepository_Integration(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	repo, err := database.NewAuctionRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	// 1. Create seller and bidder
	seller, err := database.CreateTestCharacter(ctx, db, "AuctionSeller")
	if err != nil {
		t.Fatal(err)
	}
	bidder1, err := database.CreateTestCharacter(ctx, db, "AuctionBidder1")
	if err != nil {
		t.Fatal(err)
	}
	bidder2, err := database.CreateTestCharacter(ctx, db, "AuctionBidder2")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := db.ExecContext(ctx, "UPDATE characters SET money = 10000 WHERE id IN (?, ?, ?)", seller.ID, bidder1.ID, bidder2.ID); err != nil {
		t.Fatal(err)
	}

	// 2. Create Listing
	now := time.Now().UTC()
	listing := auction.AuctionListing{
		SellerCharacterID: seller.ID,
		ItemID:            "wea_excalibur",
		ItemName:          "Holy Excalibur",
		ItemCategory:      "WEAPON",
		EnhancementLevel:  5,
		StartBid:          500,
		BuyoutPrice:       2000,
		ExpiresAt:         now.Add(time.Hour),
	}
	created, err := repo.CreateListing(ctx, listing)
	if err != nil {
		t.Fatalf("CreateListing failed: %v", err)
	}
	if created.ID == "" {
		t.Fatal("expected non-empty listing ID")
	}

	// 3. Bidder 1 places bid of 600
	bid1, err := repo.PlaceBid(ctx, created.ID, bidder1.ID, 600)
	if err != nil {
		t.Fatalf("Bid 1 failed: %v", err)
	}
	if bid1.CurrentBid != 600 || *bid1.HighestBidderID != bidder1.ID {
		t.Errorf("bid 1 state: current bid=%d, bidder=%v", bid1.CurrentBid, *bid1.HighestBidderID)
	}

	// Verify bidder 1 money deducted (10,000 - 600 = 9,400)
	var money int
	_ = db.QueryRowContext(ctx, "SELECT money FROM characters WHERE id = ?", bidder1.ID).Scan(&money)
	if money != 9400 {
		t.Errorf("bidder 1 money = %d, want 9400", money)
	}

	// 4. Bidder 2 outbids with 800 -> Bidder 1 should be refunded 600!
	bid2, err := repo.PlaceBid(ctx, created.ID, bidder2.ID, 800)
	if err != nil {
		t.Fatalf("Bid 2 failed: %v", err)
	}
	if bid2.CurrentBid != 800 || *bid2.HighestBidderID != bidder2.ID {
		t.Errorf("bid 2 state: current bid=%d, bidder=%v", bid2.CurrentBid, *bid2.HighestBidderID)
	}

	// Verify bidder 1 refunded to 10,000 and bidder 2 deducted to 9,200
	_ = db.QueryRowContext(ctx, "SELECT money FROM characters WHERE id = ?", bidder1.ID).Scan(&money)
	if money != 10000 {
		t.Errorf("bidder 1 refunded money = %d, want 10000", money)
	}
	_ = db.QueryRowContext(ctx, "SELECT money FROM characters WHERE id = ?", bidder2.ID).Scan(&money)
	if money != 9200 {
		t.Errorf("bidder 2 money = %d, want 9200", money)
	}

	// 5. Settle listing -> Seller receives 800 gold
	settled, err := repo.SettleListing(ctx, created.ID)
	if err != nil {
		t.Fatalf("SettleListing failed: %v", err)
	}
	if settled.Status != auction.StatusSold {
		t.Errorf("settled status = %v, want SOLD", settled.Status)
	}

	_ = db.QueryRowContext(ctx, "SELECT money FROM characters WHERE id = ?", seller.ID).Scan(&money)
	if money != 10800 {
		t.Errorf("seller money = %d, want 10800", money)
	}
}
