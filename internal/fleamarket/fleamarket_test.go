package fleamarket_test

import (
	"context"
	"errors"
	"testing"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/fleamarket"
)

type mockFleaMarketRepo struct {
	listings map[string]fleamarket.Listing
}

func newMockFleaMarketRepo() *mockFleaMarketRepo {
	return &mockFleaMarketRepo{
		listings: make(map[string]fleamarket.Listing),
	}
}

func (m *mockFleaMarketRepo) CreateListing(ctx context.Context, listing fleamarket.Listing) error {
	m.listings[listing.ID] = listing
	return nil
}

func (m *mockFleaMarketRepo) GetListingByID(ctx context.Context, id string) (fleamarket.Listing, error) {
	l, ok := m.listings[id]
	if !ok {
		return fleamarket.Listing{}, fleamarket.ErrListingNotFound
	}
	return l, nil
}

func (m *mockFleaMarketRepo) GetListingByIDForUpdate(ctx context.Context, id string) (fleamarket.Listing, error) {
	return m.GetListingByID(ctx, id)
}

func (m *mockFleaMarketRepo) ListActiveListings(ctx context.Context, limit, offset int) ([]fleamarket.Listing, int, error) {
	var active []fleamarket.Listing
	for _, l := range m.listings {
		if l.Status == fleamarket.StatusActive {
			active = append(active, l)
		}
	}
	total := len(active)
	if offset >= len(active) {
		return []fleamarket.Listing{}, total, nil
	}
	end := offset + limit
	if end > len(active) {
		end = len(active)
	}
	return active[offset:end], total, nil
}

func (m *mockFleaMarketRepo) GetListingsBySeller(ctx context.Context, sellerID string) ([]fleamarket.Listing, error) {
	var results []fleamarket.Listing
	for _, l := range m.listings {
		if l.SellerCharacterID == sellerID {
			results = append(results, l)
		}
	}
	return results, nil
}

func (m *mockFleaMarketRepo) CountActiveListingsBySeller(ctx context.Context, sellerID string) (int, error) {
	count := 0
	for _, l := range m.listings {
		if l.SellerCharacterID == sellerID && l.Status == fleamarket.StatusActive {
			count++
		}
	}
	return count, nil
}

func (m *mockFleaMarketRepo) UpdateListing(ctx context.Context, listing fleamarket.Listing) error {
	if _, ok := m.listings[listing.ID]; !ok {
		return fleamarket.ErrListingNotFound
	}
	m.listings[listing.ID] = listing
	return nil
}

type mockCharRepo struct {
	characters map[string]corecharacter.Character
}

func newMockCharRepo() *mockCharRepo {
	return &mockCharRepo{characters: make(map[string]corecharacter.Character)}
}

func (m *mockCharRepo) FindByID(ctx context.Context, id string) (corecharacter.Character, error) {
	c, ok := m.characters[id]
	if !ok {
		return corecharacter.Character{}, corecharacter.ErrNotFound
	}
	return c, nil
}

func (m *mockCharRepo) FindByIDForUpdate(ctx context.Context, id string) (corecharacter.Character, error) {
	return m.FindByID(ctx, id)
}

func (m *mockCharRepo) Update(ctx context.Context, c corecharacter.Character) error {
	m.characters[c.ID] = c
	return nil
}

type mockInvRepo struct {
	inventories map[string]coreinventory.Inventory
}

func newMockInvRepo() *mockInvRepo {
	return &mockInvRepo{inventories: make(map[string]coreinventory.Inventory)}
}

func (m *mockInvRepo) FindByCharacterID(ctx context.Context, characterID string) (coreinventory.Inventory, error) {
	inv, ok := m.inventories[characterID]
	if !ok {
		inv, _ = coreinventory.New(characterID)
		m.inventories[characterID] = inv
	}
	return inv, nil
}

func (m *mockInvRepo) FindByCharacterIDForUpdate(ctx context.Context, characterID string) (coreinventory.Inventory, error) {
	return m.FindByCharacterID(ctx, characterID)
}

func (m *mockInvRepo) Save(ctx context.Context, inv coreinventory.Inventory) error {
	m.inventories[inv.CharacterID] = inv
	return nil
}

type mockItemDefs struct{}

func (m *mockItemDefs) FindByID(id string) (coreitem.Definition, error) {
	if id == "item-herb" {
		return coreitem.Definition{
			ID:    "item-herb",
			Name:  "薬草",
			Price: 10,
		}, nil
	}
	if id == "wea-sword" {
		return coreitem.Definition{
			ID:    "wea-sword",
			Name:  "銅の剣",
			Price: 150,
			Slot:  coreitem.SlotMainHand,
		}, nil
	}
	return coreitem.Definition{}, errors.New("item not found")
}

func TestFleaMarketService_CreateListing(t *testing.T) {
	ctx := context.Background()
	repo := newMockFleaMarketRepo()
	charRepo := newMockCharRepo()
	invRepo := newMockInvRepo()
	itemDefs := &mockItemDefs{}

	svc, err := fleamarket.NewService(repo, charRepo, invRepo, fleamarket.WithItemDefinitionProvider(itemDefs))
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	// Setup seller
	sellerID := "char-seller-1"
	charRepo.characters[sellerID] = corecharacter.Character{
		ID:    sellerID,
		Name:  "SellerHero",
		Money: 500,
	}
	sellerInv, _ := coreinventory.New(sellerID)
	inst, _ := coreitem.NewInstance("wea-sword", 2)
	_ = sellerInv.Add(inst)
	_ = invRepo.Save(ctx, sellerInv)

	now := time.Now().UTC()

	// Test successful listing
	listing, err := svc.CreateListing(ctx, sellerID, "wea-sword", 300, now)
	if err != nil {
		t.Fatalf("CreateListing failed: %v", err)
	}
	if listing.Price != 300 || listing.ItemID != "wea-sword" || listing.ItemName != "銅の剣" {
		t.Errorf("unexpected listing properties: %+v", listing)
	}
	if listing.Status != fleamarket.StatusActive {
		t.Errorf("expected StatusActive, got %s", listing.Status)
	}

	// Verify seller inventory decreased
	updatedInv, _ := invRepo.FindByCharacterID(ctx, sellerID)
	if updatedInv.Quantity("wea-sword") != 1 {
		t.Errorf("expected 1 wea-sword remaining, got %d", updatedInv.Quantity("wea-sword"))
	}

	// Test invalid price (0, negative, or > 999999)
	if _, err := svc.CreateListing(ctx, sellerID, "wea-sword", 0, now); !errors.Is(err, fleamarket.ErrInvalidPrice) {
		t.Errorf("expected ErrInvalidPrice for price 0, got %v", err)
	}
	if _, err := svc.CreateListing(ctx, sellerID, "wea-sword", 1000000, now); !errors.Is(err, fleamarket.ErrInvalidPrice) {
		t.Errorf("expected ErrInvalidPrice for price 1000000, got %v", err)
	}

	// Test unowned item
	if _, err := svc.CreateListing(ctx, sellerID, "item-herb", 50, now); !errors.Is(err, fleamarket.ErrItemNotInInventory) {
		t.Errorf("expected ErrItemNotInInventory, got %v", err)
	}

	// Test max listing limit (5 items)
	// Add more items to inventory to create up to 5 listings
	inst2, _ := coreitem.NewInstance("item-herb", 10)
	_ = updatedInv.Add(inst2)
	_ = invRepo.Save(ctx, updatedInv)

	for i := 0; i < 4; i++ {
		_, err := svc.CreateListing(ctx, sellerID, "item-herb", 50+i, now)
		if err != nil {
			t.Fatalf("listing %d failed: %v", i+2, err)
		}
	}
	// 6th listing must fail with ErrMaxListingsReached
	_, err = svc.CreateListing(ctx, sellerID, "item-herb", 100, now)
	if !errors.Is(err, fleamarket.ErrMaxListingsReached) {
		t.Errorf("expected ErrMaxListingsReached, got %v", err)
	}
}

func TestFleaMarketService_PurchaseListing(t *testing.T) {
	ctx := context.Background()
	repo := newMockFleaMarketRepo()
	charRepo := newMockCharRepo()
	invRepo := newMockInvRepo()
	itemDefs := &mockItemDefs{}

	svc, _ := fleamarket.NewService(repo, charRepo, invRepo, fleamarket.WithItemDefinitionProvider(itemDefs))

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

	sellerInv, _ := coreinventory.New(sellerID)
	inst, _ := coreitem.NewInstance("wea-sword", 1)
	_ = sellerInv.Add(inst)
	_ = invRepo.Save(ctx, sellerInv)

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

	// Verify buyer received item
	buyerInv, _ := invRepo.FindByCharacterID(ctx, buyerID)
	if buyerInv.Quantity("wea-sword") != 1 {
		t.Errorf("expected buyer to have 1 wea-sword, got %d", buyerInv.Quantity("wea-sword"))
	}

	// 4. Double purchase on already sold listing fails
	_, err = svc.PurchaseListing(ctx, buyerID, listing.ID, now)
	if !errors.Is(err, fleamarket.ErrListingNotActive) {
		t.Errorf("expected ErrListingNotActive for sold listing, got %v", err)
	}
}

func TestFleaMarketService_CancelListing(t *testing.T) {
	ctx := context.Background()
	repo := newMockFleaMarketRepo()
	charRepo := newMockCharRepo()
	invRepo := newMockInvRepo()
	itemDefs := &mockItemDefs{}

	svc, _ := fleamarket.NewService(repo, charRepo, invRepo, fleamarket.WithItemDefinitionProvider(itemDefs))

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

	sellerInv, _ := coreinventory.New(sellerID)
	inst, _ := coreitem.NewInstance("item-herb", 1)
	_ = sellerInv.Add(inst)
	_ = invRepo.Save(ctx, sellerInv)

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

	// Item returned to seller inventory
	updatedInv, _ := invRepo.FindByCharacterID(ctx, sellerID)
	if updatedInv.Quantity("item-herb") != 1 {
		t.Errorf("expected 1 item-herb returned to seller, got %d", updatedInv.Quantity("item-herb"))
	}

	// 3. Cancelling already cancelled listing fails
	_, err = svc.CancelListing(ctx, sellerID, listing.ID)
	if !errors.Is(err, fleamarket.ErrListingNotActive) {
		t.Errorf("expected ErrListingNotActive, got %v", err)
	}
}

func TestFleaMarketService_QueriesAndPagination(t *testing.T) {
	ctx := context.Background()
	repo := newMockFleaMarketRepo()
	charRepo := newMockCharRepo()
	invRepo := newMockInvRepo()
	itemDefs := &mockItemDefs{}

	svc, _ := fleamarket.NewService(repo, charRepo, invRepo, fleamarket.WithItemDefinitionProvider(itemDefs))

	sellerID := "char-seller-p"
	charRepo.characters[sellerID] = corecharacter.Character{ID: sellerID, Name: "SellerP"}
	sellerInv, _ := coreinventory.New(sellerID)
	inst, _ := coreitem.NewInstance("item-herb", 10)
	_ = sellerInv.Add(inst)
	_ = invRepo.Save(ctx, sellerInv)

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
