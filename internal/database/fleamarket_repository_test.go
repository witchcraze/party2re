package database_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/fleamarket"
	"github.com/witchcraze/party2re/internal/id"
)

// Requires a dedicated DB: ActiveListingNumbers and ListActiveListings assert market-wide counts.
func TestFleaMarketRepository_Database(t *testing.T) {
	db := database.CreateTestDatabase(t, "players", "characters", "character_depots", "fleamarket_listings")

	ctx := context.Background()

	seller, err := database.CreateTestCharacter(ctx, db, "FleaSeller")
	if err != nil {
		t.Fatal(err)
	}
	buyer, err := database.CreateTestCharacter(ctx, db, "FleaBuyer")
	if err != nil {
		t.Fatal(err)
	}

	repo, err := database.NewFleaMarketRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC().Truncate(time.Second)

	// 1. CreateListing
	listingID := id.New()
	listing := fleamarket.Listing{
		ID:                listingID,
		Number:            7,
		SellerCharacterID: seller.ID,
		SellerName:        seller.Name,
		ItemID:            "wea-sword",
		ItemName:          "銅の剣",
		ItemCategory:      "main-hand",
		Price:             250,
		Status:            fleamarket.StatusActive,
		CreatedAt:         now,
	}

	if err := repo.CreateListing(ctx, listing); err != nil {
		t.Fatalf("failed to create listing: %v", err)
	}

	// 2. GetListingByID
	fetched, err := repo.GetListingByID(ctx, listingID)
	if err != nil {
		t.Fatalf("GetListingByID failed: %v", err)
	}
	if fetched.ID != listingID || fetched.SellerName != seller.Name || fetched.Price != 250 || fetched.Number != 7 {
		t.Errorf("unexpected fetched listing: %+v", fetched)
	}

	// 3. CountActiveListingsBySeller and ActiveListingNumbers
	count, err := repo.CountActiveListingsBySeller(ctx, seller.ID)
	if err != nil {
		t.Fatalf("CountActiveListingsBySeller failed: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 active listing, got %d", count)
	}

	numbersBefore, err := repo.ActiveListingNumbers(ctx)
	if err != nil {
		t.Fatalf("ActiveListingNumbers failed: %v", err)
	}
	if len(numbersBefore) != 1 || numbersBefore[0] != listing.Number {
		t.Errorf("expected the listing's number %d, got %v", listing.Number, numbersBefore)
	}

	// 4. ListActiveListings
	activeList, total, err := repo.ListActiveListings(ctx, 10, 0)
	if err != nil {
		t.Fatalf("ListActiveListings failed: %v", err)
	}
	if total != 1 || len(activeList) != 1 || activeList[0].ID != listingID {
		t.Errorf("expected listing %s (total 1), got total %d, list %+v", listingID, total, activeList)
	}

	// 5. GetActiveListingsBySeller
	sellerListings, err := repo.GetActiveListingsBySeller(ctx, seller.ID)
	if err != nil {
		t.Fatalf("GetActiveListingsBySeller failed: %v", err)
	}
	if len(sellerListings) != 1 {
		t.Errorf("expected 1 seller listing, got %d", len(sellerListings))
	}

	// 6. UpdateListing (Sold)
	soldAt := now.Add(5 * time.Minute)
	fetched.Status = fleamarket.StatusSold
	fetched.BuyerCharacterID = &buyer.ID
	fetched.BuyerName = &buyer.Name
	fetched.SoldAt = &soldAt

	if err := repo.UpdateListing(ctx, fetched); err != nil {
		t.Fatalf("UpdateListing failed: %v", err)
	}

	updated, err := repo.GetListingByID(ctx, listingID)
	if err != nil {
		t.Fatalf("GetListingByID after update failed: %v", err)
	}
	if updated.Status != fleamarket.StatusSold || updated.BuyerName == nil || *updated.BuyerName != buyer.Name {
		t.Errorf("unexpected updated listing: %+v", updated)
	}

	// Count should now be 0
	countAfter, err := repo.CountActiveListingsBySeller(ctx, seller.ID)
	if err != nil {
		t.Fatalf("CountActiveListingsBySeller after sell failed: %v", err)
	}
	if countAfter != 0 {
		t.Errorf("expected 0 active listings after sell, got %d", countAfter)
	}

	numbersAfter, err := repo.ActiveListingNumbers(ctx)
	if err != nil {
		t.Fatalf("ActiveListingNumbers after sell failed: %v", err)
	}
	if len(numbersAfter) != 0 {
		t.Errorf("expected no active numbers after sell, got %v", numbersAfter)
	}

	// 7. Stale UpdateListing on already-sold listing should return ErrListingNotActive (CAS failure)
	err = repo.UpdateListing(ctx, fetched)
	if !errors.Is(err, fleamarket.ErrListingNotActive) {
		t.Errorf("expected ErrListingNotActive on stale update, got %v", err)
	}

	// 8. UpdateListing on non-existent ID should return ErrListingNotFound
	missing := fetched
	missing.ID = "non-existent-listing-id"
	err = repo.UpdateListing(ctx, missing)
	if !errors.Is(err, fleamarket.ErrListingNotFound) {
		t.Errorf("expected ErrListingNotFound on missing listing, got %v", err)
	}
}
