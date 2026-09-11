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

func (m *mockFleaMarketRepo) CountTotalActiveListings(ctx context.Context) (int, error) {
	count := 0
	for _, l := range m.listings {
		if l.Status == fleamarket.StatusActive {
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

type mockDepotRepo struct {
	depots map[string]depot.Depot
}

func newMockDepotRepo() *mockDepotRepo {
	return &mockDepotRepo{depots: make(map[string]depot.Depot)}
}

func (m *mockDepotRepo) FindByCharacterID(ctx context.Context, characterID string) (depot.Depot, error) {
	d, ok := m.depots[characterID]
	if !ok {
		d, _ = depot.NewDepot(characterID)
		d.Capacity = 20
		m.depots[characterID] = d
	}
	return d, nil
}

func (m *mockDepotRepo) FindByCharacterIDForUpdate(ctx context.Context, characterID string) (depot.Depot, error) {
	return m.FindByCharacterID(ctx, characterID)
}

func (m *mockDepotRepo) Save(ctx context.Context, d depot.Depot) error {
	m.depots[d.CharacterID] = d
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
	depotRepo := newMockDepotRepo()
	itemDefs := &mockItemDefs{}

	svc, err := fleamarket.NewService(repo, charRepo, depotRepo, fleamarket.WithItemDefinitionProvider(itemDefs))
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
	sellerDepot, _ := depot.NewDepot(sellerID)
	sellerDepot.Capacity = 20
	inst, _ := coreitem.NewInstance("wea-sword", 2)
	_ = sellerDepot.AddItem(inst)
	_ = depotRepo.Save(ctx, sellerDepot)

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

	// Verify seller depot decreased (1 remaining)
	updatedDepot, _ := depotRepo.FindByCharacterID(ctx, sellerID)
	if len(updatedDepot.Items) != 1 || updatedDepot.Items[0].Quantity != 1 {
		t.Errorf("expected 1 wea-sword remaining in depot, got %+v", updatedDepot.Items)
	}

	// Test invalid price (0, negative, or > 999999)
	if _, err := svc.CreateListing(ctx, sellerID, "wea-sword", 0, now); !errors.Is(err, fleamarket.ErrInvalidPrice) {
		t.Errorf("expected ErrInvalidPrice for price 0, got %v", err)
	}
	if _, err := svc.CreateListing(ctx, sellerID, "wea-sword", 1000000, now); !errors.Is(err, fleamarket.ErrInvalidPrice) {
		t.Errorf("expected ErrInvalidPrice for price 1000000, got %v", err)
	}

	// Test unowned item
	if _, err := svc.CreateListing(ctx, sellerID, "item-herb", 50, now); !errors.Is(err, fleamarket.ErrItemNotInDepot) {
		t.Errorf("expected ErrItemNotInDepot, got %v", err)
	}

	// Test max listing limit (5 items default)
	inst2, _ := coreitem.NewInstance("item-herb", 10)
	_ = updatedDepot.AddItem(inst2)
	_ = depotRepo.Save(ctx, updatedDepot)

	for i := 0; i < 4; i++ {
		_, err := svc.CreateListing(ctx, sellerID, "item-herb", 50+i, now)
		if err != nil {
			t.Fatalf("listing %d failed: %v", i+2, err)
		}
	}
	// 6th listing must fail with ErrMaxListingsReached (base 5 limit)
	_, err = svc.CreateListing(ctx, sellerID, "item-herb", 100, now)
	if !errors.Is(err, fleamarket.ErrMaxListingsReached) {
		t.Errorf("expected ErrMaxListingsReached, got %v", err)
	}
}

func TestFleaMarketService_OverFleaCapacity(t *testing.T) {
	ctx := context.Background()
	repo := newMockFleaMarketRepo()
	charRepo := newMockCharRepo()
	depotRepo := newMockDepotRepo()
	itemDefs := &mockItemDefs{}

	svc, err := fleamarket.NewService(repo, charRepo, depotRepo, fleamarket.WithItemDefinitionProvider(itemDefs))
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	now := time.Now().UTC()

	// 1. OverFlea = 3 -> allows 5 + 3 = 8 listings
	seller3ID := "char-overflea-3"
	charRepo.characters[seller3ID] = corecharacter.Character{
		ID:       seller3ID,
		Name:     "OverFlea3Seller",
		OverFlea: 3,
	}
	depot3, _ := depot.NewDepot(seller3ID)
	depot3.Capacity = 20
	inst3, _ := coreitem.NewInstance("item-herb", 10)
	_ = depot3.AddItem(inst3)
	_ = depotRepo.Save(ctx, depot3)

	for i := 0; i < 8; i++ {
		_, err := svc.CreateListing(ctx, seller3ID, "item-herb", 50+i, now)
		if err != nil {
			t.Fatalf("listing %d with OverFlea=3 failed: %v", i+1, err)
		}
	}
	// 9th listing fails
	_, err = svc.CreateListing(ctx, seller3ID, "item-herb", 100, now)
	if !errors.Is(err, fleamarket.ErrMaxListingsReached) {
		t.Errorf("expected ErrMaxListingsReached for 9th listing with OverFlea=3, got %v", err)
	}

	// 2. OverFlea = 5 -> allows 5 + 5 = 10 listings (legacy maximum)
	seller5ID := "char-overflea-5"
	charRepo.characters[seller5ID] = corecharacter.Character{
		ID:       seller5ID,
		Name:     "OverFlea5Seller",
		OverFlea: 5,
	}
	depot5, _ := depot.NewDepot(seller5ID)
	depot5.Capacity = 20
	inst5, _ := coreitem.NewInstance("item-herb", 15)
	_ = depot5.AddItem(inst5)
	_ = depotRepo.Save(ctx, depot5)

	for i := 0; i < 10; i++ {
		_, err := svc.CreateListing(ctx, seller5ID, "item-herb", 50+i, now)
		if err != nil {
			t.Fatalf("listing %d with OverFlea=5 failed: %v", i+1, err)
		}
	}
	// 11th listing fails
	_, err = svc.CreateListing(ctx, seller5ID, "item-herb", 100, now)
	if !errors.Is(err, fleamarket.ErrMaxListingsReached) {
		t.Errorf("expected ErrMaxListingsReached for 11th listing with OverFlea=5, got %v", err)
	}
}

func TestFleaMarketService_ServerCeiling(t *testing.T) {
	ctx := context.Background()
	repo := newMockFleaMarketRepo()
	charRepo := newMockCharRepo()
	depotRepo := newMockDepotRepo()
	itemDefs := &mockItemDefs{}

	svc, err := fleamarket.NewService(repo, charRepo, depotRepo, fleamarket.WithItemDefinitionProvider(itemDefs))
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	now := time.Now().UTC()

	// Pre-populate 120 listings from various characters
	for i := 0; i < fleamarket.ServerMaxListings; i++ {
		l := fleamarket.Listing{
			ID:                fmt.Sprintf("listing-server-%d", i),
			SellerCharacterID: fmt.Sprintf("seller-%d", i),
			SellerName:        "RandomSeller",
			ItemID:            "item-herb",
			ItemName:          "薬草",
			ItemCategory:      "consumable",
			Price:             100,
			Status:            fleamarket.StatusActive,
			CreatedAt:         now,
		}
		_ = repo.CreateListing(ctx, l)
	}

	// Try to create 121st listing
	newSellerID := "char-seller-new"
	charRepo.characters[newSellerID] = corecharacter.Character{ID: newSellerID, Name: "NewSeller"}
	dep, _ := depot.NewDepot(newSellerID)
	dep.Capacity = 20
	inst, _ := coreitem.NewInstance("item-herb", 5)
	_ = dep.AddItem(inst)
	_ = depotRepo.Save(ctx, dep)

	_, err = svc.CreateListing(ctx, newSellerID, "item-herb", 100, now)
	if !errors.Is(err, fleamarket.ErrServerMaxListingsReached) {
		t.Errorf("expected ErrServerMaxListingsReached at 120 server ceiling, got %v", err)
	}
}
