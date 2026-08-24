package auction_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/auction"
	"github.com/witchcraze/party2re/internal/database"
)

func TestAuctionServiceDatabaseIntegration_Buyout(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	auctionRepo, err := database.NewAuctionRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	svc, err := auction.NewService(auctionRepo)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	// 1. Create seller and buyer
	seller, err := database.CreateTestCharacter(ctx, db, "BuyoutSeller")
	if err != nil {
		t.Fatal(err)
	}
	buyer, err := database.CreateTestCharacter(ctx, db, "BuyoutBuyer")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := db.ExecContext(ctx, "UPDATE characters SET money = 10000 WHERE id IN (?, ?)", seller.ID, buyer.ID); err != nil {
		t.Fatal(err)
	}

	// 2. Create Listing with Buyout Price 3,000
	listing, err := svc.CreateListing(ctx, seller.ID, "arm_dragon_armor", "Dragon Armor", "ARMOR", 3, 1000, 3000, time.Hour)
	if err != nil {
		t.Fatalf("CreateListing failed: %v", err)
	}

	// 3. Buyer instantly buys out
	bought, err := svc.Buyout(ctx, buyer.ID, listing.ID)
	if err != nil {
		t.Fatalf("Buyout failed: %v", err)
	}
	if bought.Status != auction.StatusSold || bought.CurrentBid != 3000 || *bought.HighestBidderID != buyer.ID {
		t.Errorf("bought state: status=%v, bid=%d, buyer=%v", bought.Status, bought.CurrentBid, *bought.HighestBidderID)
	}

	// 4. Verify balances: Buyer has 7,000, Seller has 13,000
	var buyerMoney, sellerMoney int
	_ = db.QueryRowContext(ctx, "SELECT money FROM characters WHERE id = ?", buyer.ID).Scan(&buyerMoney)
	_ = db.QueryRowContext(ctx, "SELECT money FROM characters WHERE id = ?", seller.ID).Scan(&sellerMoney)

	if buyerMoney != 7000 || sellerMoney != 13000 {
		t.Errorf("buyer money = %d (want 7000), seller money = %d (want 13000)", buyerMoney, sellerMoney)
	}
}
