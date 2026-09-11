package fleamarket_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/depot"
	"github.com/witchcraze/party2re/internal/fleamarket"
)

func TestFleaMarketService_PurchaseListing(t *testing.T) {
	ctx := context.Background()
	repo := newMockFleaMarketRepo()
	charRepo := newMockCharRepo()
	depotRepo := newMockDepotRepo()
	itemDefs := &mockItemDefs{}

	svc, _ := fleamarket.NewService(repo, charRepo, depotRepo, fleamarket.WithItemDefinitionProvider(itemDefs))

	sellerID := "char-seller-2"
	buyerID := "char-buyer-1"

	charRepo.characters[sellerID] = corecharacter.Character{
		ID:    sellerID,
		Name:  "SellerAlice",
		Money: 1000,
	}
	charRepo.characters[buyerID] = corecharacter.Character{
		ID:    buyerID,
		Name:  "BuyerBob",
		Money: 2000,
	}

	sellerDepot, _ := depot.NewDepot(sellerID)
	sellerDepot.Capacity = 20
	inst, _ := coreitem.NewInstance("wea-sword", 1)
	_ = sellerDepot.AddItem(inst)
	_ = depotRepo.Save(ctx, sellerDepot)

	now := time.Now().UTC()
	listing, err := svc.CreateListing(ctx, sellerID, "wea-sword", 450, now)
	if err != nil {
		t.Fatalf("CreateListing failed: %v", err)
	}

	// 1. Seller cannot buy own listing
	_, err = svc.PurchaseListing(ctx, sellerID, listing.ID, now)
	if !errors.Is(err, fleamarket.ErrCannotBuyOwnListing) {
		t.Errorf("expected ErrCannotBuyOwnListing, got %v", err)
	}

	// 2. Buyer with insufficient funds
	poorBuyerID := "char-poor"
	charRepo.characters[poorBuyerID] = corecharacter.Character{
		ID:    poorBuyerID,
		Name:  "PoorGuy",
		Money: 100,
	}
	_, err = svc.PurchaseListing(ctx, poorBuyerID, listing.ID, now)
	if !errors.Is(err, fleamarket.ErrInsufficientGold) {
		t.Errorf("expected ErrInsufficientGold, got %v", err)
	}

	// 3. Successful Purchase
	result, err := svc.PurchaseListing(ctx, buyerID, listing.ID, now)
	if err != nil {
		t.Fatalf("PurchaseListing failed: %v", err)
	}

	if result.Listing.Status != fleamarket.StatusSold {
		t.Errorf("expected StatusSold, got %s", result.Listing.Status)
	}
	if result.BuyerGold != 1550 {
		t.Errorf("expected buyer gold 1550, got %d", result.BuyerGold)
	}
	if result.SellerGold != 1450 {
		t.Errorf("expected seller gold 1450, got %d", result.SellerGold)
	}

	// Verify buyer received item in their depot
	buyerDepot, _ := depotRepo.FindByCharacterID(ctx, buyerID)
	if len(buyerDepot.Items) != 1 || buyerDepot.Items[0].DefinitionID != "wea-sword" {
		t.Errorf("expected buyer depot to contain 1 wea-sword, got %+v", buyerDepot.Items)
	}

	// 4. Double purchase on already sold listing fails
	_, err = svc.PurchaseListing(ctx, buyerID, listing.ID, now)
	if !errors.Is(err, fleamarket.ErrListingNotActive) {
		t.Errorf("expected ErrListingNotActive for sold listing, got %v", err)
	}
}

func TestFleaMarketService_PurchaseListing_DepotFull(t *testing.T) {
	ctx := context.Background()
	repo := newMockFleaMarketRepo()
	charRepo := newMockCharRepo()
	depotRepo := newMockDepotRepo()
	itemDefs := &mockItemDefs{}

	svc, _ := fleamarket.NewService(repo, charRepo, depotRepo, fleamarket.WithItemDefinitionProvider(itemDefs))

	sellerID := "char-seller-full"
	buyerID := "char-buyer-full"

	charRepo.characters[sellerID] = corecharacter.Character{ID: sellerID, Name: "Seller", Money: 1000}
	charRepo.characters[buyerID] = corecharacter.Character{ID: buyerID, Name: "Buyer", Money: 5000}

	sellerDepot, _ := depot.NewDepot(sellerID)
	sellerDepot.Capacity = 20
	inst, _ := coreitem.NewInstance("wea-sword", 1)
	_ = sellerDepot.AddItem(inst)
	_ = depotRepo.Save(ctx, sellerDepot)

	// Fill buyer depot to maximum capacity (5 items)
	buyerDepot, _ := depot.NewDepot(buyerID)
	buyerDepot.Capacity = 5
	for i := 0; i < buyerDepot.Capacity; i++ {
		fillInst, _ := coreitem.NewInstance(fmt.Sprintf("item-fill-%d", i), 1)
		_ = buyerDepot.AddItem(fillInst)
	}
	_ = depotRepo.Save(ctx, buyerDepot)

	now := time.Now().UTC()
	listing, err := svc.CreateListing(ctx, sellerID, "wea-sword", 500, now)
	if err != nil {
		t.Fatalf("CreateListing failed: %v", err)
	}

	_, err = svc.PurchaseListing(ctx, buyerID, listing.ID, now)
	if !errors.Is(err, fleamarket.ErrDepotFull) {
		t.Errorf("expected ErrDepotFull when buyer depot is full, got %v", err)
	}
}

func TestFleaMarketService_CancelListing(t *testing.T) {
	ctx := context.Background()
	repo := newMockFleaMarketRepo()
	charRepo := newMockCharRepo()
	depotRepo := newMockDepotRepo()
	itemDefs := &mockItemDefs{}

	svc, _ := fleamarket.NewService(repo, charRepo, depotRepo, fleamarket.WithItemDefinitionProvider(itemDefs))

	sellerID := "char-seller-3"
	otherID := "char-other"

	charRepo.characters[sellerID] = corecharacter.Character{
		ID:    sellerID,
		Name:  "SellerCharlie",
		Money: 100,
	}
	charRepo.characters[otherID] = corecharacter.Character{
		ID:    otherID,
		Name:  "OtherGuy",
		Money: 100,
	}

	sellerDepot, _ := depot.NewDepot(sellerID)
	sellerDepot.Capacity = 20
	inst, _ := coreitem.NewInstance("item-herb", 1)
	_ = sellerDepot.AddItem(inst)
	_ = depotRepo.Save(ctx, sellerDepot)

	now := time.Now().UTC()
	listing, err := svc.CreateListing(ctx, sellerID, "item-herb", 50, now)
	if err != nil {
		t.Fatalf("CreateListing failed: %v", err)
	}

	// 1. Non-seller cannot cancel
	_, err = svc.CancelListing(ctx, otherID, listing.ID)
	if !errors.Is(err, fleamarket.ErrUnauthorizedSeller) {
		t.Errorf("expected ErrUnauthorizedSeller, got %v", err)
	}

	// 2. Seller cancels listing
	cancelled, err := svc.CancelListing(ctx, sellerID, listing.ID)
	if err != nil {
		t.Fatalf("CancelListing failed: %v", err)
	}
	if cancelled.Status != fleamarket.StatusCancelled {
		t.Errorf("expected StatusCancelled, got %s", cancelled.Status)
	}

	// Item returned to seller depot
	updatedDepot, _ := depotRepo.FindByCharacterID(ctx, sellerID)
	if len(updatedDepot.Items) != 1 || updatedDepot.Items[0].DefinitionID != "item-herb" {
		t.Errorf("expected 1 item-herb in seller depot, got %+v", updatedDepot.Items)
	}

	// 3. Cancelling already cancelled listing fails
	_, err = svc.CancelListing(ctx, sellerID, listing.ID)
	if !errors.Is(err, fleamarket.ErrListingNotActive) {
		t.Errorf("expected ErrListingNotActive, got %v", err)
	}
}

func TestFleaMarketService_CancelListing_DepotFull(t *testing.T) {
	ctx := context.Background()
	repo := newMockFleaMarketRepo()
	charRepo := newMockCharRepo()
	depotRepo := newMockDepotRepo()
	itemDefs := &mockItemDefs{}

	svc, _ := fleamarket.NewService(repo, charRepo, depotRepo, fleamarket.WithItemDefinitionProvider(itemDefs))

	sellerID := "char-seller-cancelfull"
	charRepo.characters[sellerID] = corecharacter.Character{ID: sellerID, Name: "SellerCancel"}

	sellerDepot, _ := depot.NewDepot(sellerID)
	sellerDepot.Capacity = 5
	inst, _ := coreitem.NewInstance("item-herb", 1)
	_ = sellerDepot.AddItem(inst)
	_ = depotRepo.Save(ctx, sellerDepot)

	now := time.Now().UTC()
	listing, err := svc.CreateListing(ctx, sellerID, "item-herb", 50, now)
	if err != nil {
		t.Fatalf("CreateListing failed: %v", err)
	}

	// Now fill seller's depot to maximum (Capacity is 5, item was consumed so currently 0 items)
	fillDepot, _ := depotRepo.FindByCharacterID(ctx, sellerID)
	for i := 0; i < fillDepot.Capacity; i++ {
		fillInst, _ := coreitem.NewInstance(fmt.Sprintf("item-fill-%d", i), 1)
		_ = fillDepot.AddItem(fillInst)
	}
	_ = depotRepo.Save(ctx, fillDepot)

	_, err = svc.CancelListing(ctx, sellerID, listing.ID)
	if !errors.Is(err, fleamarket.ErrDepotFull) {
		t.Errorf("expected ErrDepotFull on CancelListing when depot is full, got %v", err)
	}
}

func TestFleaMarketService_QueriesAndPagination(t *testing.T) {
	ctx := context.Background()
	repo := newMockFleaMarketRepo()
	charRepo := newMockCharRepo()
	depotRepo := newMockDepotRepo()
	itemDefs := &mockItemDefs{}

	svc, _ := fleamarket.NewService(repo, charRepo, depotRepo, fleamarket.WithItemDefinitionProvider(itemDefs))

	sellerID := "char-seller-p"
	charRepo.characters[sellerID] = corecharacter.Character{ID: sellerID, Name: "SellerP"}
	sellerDepot, _ := depot.NewDepot(sellerID)
	sellerDepot.Capacity = 20
	inst, _ := coreitem.NewInstance("item-herb", 10)
	_ = sellerDepot.AddItem(inst)
	_ = depotRepo.Save(ctx, sellerDepot)

	now := time.Now().UTC()
	var createdIDs []string
	for i := 0; i < 3; i++ {
		l, err := svc.CreateListing(ctx, sellerID, "item-herb", 100+i*10, now)
		if err != nil {
			t.Fatalf("failed to create listing %d: %v", i, err)
		}
		createdIDs = append(createdIDs, l.ID)
	}

	// 1. ListActiveListings
	listings, total, err := svc.ListActiveListings(ctx, 2, 0)
	if err != nil {
		t.Fatalf("ListActiveListings failed: %v", err)
	}
	if total != 3 || len(listings) != 2 {
		t.Errorf("expected total 3, page length 2, got total %d, length %d", total, len(listings))
	}

	// 2. GetListing
	single, err := svc.GetListing(ctx, createdIDs[0])
	if err != nil {
		t.Fatalf("GetListing failed: %v", err)
	}
	if single.ID != createdIDs[0] {
		t.Errorf("expected ID %s, got %s", createdIDs[0], single.ID)
	}

	// 3. GetCharacterListings
	sellerListings, err := svc.GetCharacterListings(ctx, sellerID)
	if err != nil {
		t.Fatalf("GetCharacterListings failed: %v", err)
	}
	if len(sellerListings) != 3 {
		t.Errorf("expected 3 listings for seller, got %d", len(sellerListings))
	}

	// 4. Invalid input checks
	if _, err := svc.GetListing(ctx, ""); !errors.Is(err, fleamarket.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for empty ID, got %v", err)
	}
	if _, err := svc.GetCharacterListings(ctx, ""); !errors.Is(err, fleamarket.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for empty character ID, got %v", err)
	}
	if _, err := svc.CreateListing(ctx, "", "item-herb", 100, now); !errors.Is(err, fleamarket.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for empty seller ID, got %v", err)
	}
	if _, err := svc.PurchaseListing(ctx, "", createdIDs[0], now); !errors.Is(err, fleamarket.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for empty buyer ID, got %v", err)
	}
	if _, err := svc.CancelListing(ctx, "", createdIDs[0]); !errors.Is(err, fleamarket.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for empty seller ID in CancelListing, got %v", err)
	}
}
