package store_test

import (
	"context"
	"testing"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/depot"
	"github.com/witchcraze/party2re/internal/store"
)

type strictUninitDepotRepo struct {
	depots map[string]depot.Depot
}

func newStrictUninitDepotRepo() *strictUninitDepotRepo {
	return &strictUninitDepotRepo{depots: make(map[string]depot.Depot)}
}

func (m *strictUninitDepotRepo) FindByCharacterID(_ context.Context, characterID string) (depot.Depot, error) {
	d, ok := m.depots[characterID]
	if !ok {
		return depot.Depot{}, depot.ErrNotFound
	}
	return d, nil
}

func (m *strictUninitDepotRepo) FindByCharacterIDForUpdate(ctx context.Context, characterID string) (depot.Depot, error) {
	return m.FindByCharacterID(ctx, characterID)
}

func (m *strictUninitDepotRepo) Save(_ context.Context, d depot.Depot) error {
	m.depots[d.CharacterID] = d
	return nil
}

type salesDepotTestFixture struct {
	svc       *store.Service
	storeRepo *mockStoreRepo
	charRepo  *mockCharRepo
	depotRepo *strictUninitDepotRepo
	catalog   *mockCatalog
}

func setupSalesDepotFixture(t *testing.T) *salesDepotTestFixture {
	t.Helper()
	storeRepo := newMockStoreRepo()
	charRepo := &mockCharRepo{chars: make(map[string]corecharacter.Character)}
	depotRepo := newStrictUninitDepotRepo()
	catalog := &mockCatalog{items: map[string]coreitem.Definition{
		"wpn_001": {ID: "wpn_001", Name: "ひのきのぼう", Price: 100},
	}}

	c1, _ := corecharacter.New("Seller")
	c1.ID = "c_seller"
	_ = c1.AddMoney(100000)
	charRepo.chars["c_seller"] = c1

	c2, _ := corecharacter.New("Buyer")
	c2.ID = "c_buyer"
	_ = c2.AddMoney(100000)
	charRepo.chars["c_buyer"] = c2

	svc := store.NewService(
		storeRepo,
		charRepo,
		depotRepo,
		catalog,
		&mockTxProvider{},
		store.WithNowFunc(func() time.Time { return time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC) }),
	)

	return &salesDepotTestFixture{
		svc:       svc,
		storeRepo: storeRepo,
		charRepo:  charRepo,
		depotRepo: depotRepo,
		catalog:   catalog,
	}
}

func TestBuyItem_WithUninitializedBuyerDepot(t *testing.T) {
	f := setupSalesDepotFixture(t)
	ctx := context.Background()

	// Seller opens a store and lists an item (seller has depot with item)
	_, err := f.svc.BuildStore(ctx, "c_seller", "town1", "001", "店一号")
	if err != nil {
		t.Fatalf("BuildStore failed: %v", err)
	}

	sellerDepot := depot.Depot{
		CharacterID: "c_seller",
		Capacity:    10,
		Items: []coreitem.Instance{
			{ID: "item-inst-1", DefinitionID: "wpn_001", Quantity: 1},
		},
	}
	_ = f.depotRepo.Save(ctx, sellerDepot)

	sale, err := f.svc.ListGoldItem(ctx, "c_seller", "item-inst-1", 500)
	if err != nil {
		t.Fatalf("ListGoldItem failed: %v", err)
	}

	// Ensure buyer depot is completely uninitialized (not present in repo)
	if _, err := f.depotRepo.FindByCharacterID(ctx, "c_buyer"); err != depot.ErrNotFound {
		t.Fatalf("expected buyer depot to be uninitialized")
	}

	// Buyer purchases item
	if err := f.svc.BuyItem(ctx, "c_buyer", sale.ID); err != nil {
		t.Fatalf("BuyItem failed on uninitialized buyer depot: %v", err)
	}

	// Verify buyer depot was auto-created and contains the purchased item
	bDepot, err := f.depotRepo.FindByCharacterID(ctx, "c_buyer")
	if err != nil {
		t.Fatalf("buyer depot was not created: %v", err)
	}
	if len(bDepot.Items) != 1 || bDepot.Items[0].DefinitionID != "wpn_001" {
		t.Fatalf("expected purchased item in buyer depot, got %+v", bDepot.Items)
	}
}

func TestCancelSale_WithUninitializedDepot(t *testing.T) {
	f := setupSalesDepotFixture(t)
	ctx := context.Background()

	_, err := f.svc.BuildStore(ctx, "c_seller", "town1", "001", "店一号")
	if err != nil {
		t.Fatalf("BuildStore failed: %v", err)
	}

	sellerDepot := depot.Depot{
		CharacterID: "c_seller",
		Capacity:    10,
		Items: []coreitem.Instance{
			{ID: "item-inst-1", DefinitionID: "wpn_001", Quantity: 1},
		},
	}
	_ = f.depotRepo.Save(ctx, sellerDepot)

	sale, err := f.svc.ListGoldItem(ctx, "c_seller", "item-inst-1", 500)
	if err != nil {
		t.Fatalf("ListGoldItem failed: %v", err)
	}

	// Simulate depot record being deleted or missing
	delete(f.depotRepo.depots, "c_seller")

	// Withdraw listing should succeed and re-create depot under lock
	if err := f.svc.WithdrawListing(ctx, "c_seller", sale.ID); err != nil {
		t.Fatalf("WithdrawListing failed on missing seller depot: %v", err)
	}

	sDepot, err := f.depotRepo.FindByCharacterID(ctx, "c_seller")
	if err != nil {
		t.Fatalf("seller depot was not recreated: %v", err)
	}
	if len(sDepot.Items) != 1 || sDepot.Items[0].DefinitionID != "wpn_001" {
		t.Fatalf("expected returned item in seller depot, got %+v", sDepot.Items)
	}
}

func TestListGoldItem_WithUninitializedDepot(t *testing.T) {
	f := setupSalesDepotFixture(t)
	ctx := context.Background()

	_, err := f.svc.BuildStore(ctx, "c_seller", "town1", "001", "店一号")
	if err != nil {
		t.Fatalf("BuildStore failed: %v", err)
	}

	// Attempting to list item from uninitialized depot should fail with ErrItemNotFound, not unhandled depot.ErrNotFound
	_, err = f.svc.ListGoldItem(ctx, "c_seller", "nonexistent-item", 500)
	if err != store.ErrItemNotFound {
		t.Errorf("expected store.ErrItemNotFound, got %v", err)
	}
}

func TestTradeItem_WithUninitializedDepot(t *testing.T) {
	f := setupSalesDepotFixture(t)
	ctx := context.Background()

	_, err := f.svc.BuildStore(ctx, "c_seller", "town1", "001", "店一号")
	if err != nil {
		t.Fatalf("BuildStore failed: %v", err)
	}

	sellerDepot := depot.Depot{
		CharacterID: "c_seller",
		Capacity:    10,
		Items: []coreitem.Instance{
			{ID: "item-inst-1", DefinitionID: "wpn_001", Quantity: 1},
		},
	}
	_ = f.depotRepo.Save(ctx, sellerDepot)

	sale, err := f.svc.ListBarterItem(ctx, "c_seller", "item-inst-1", "ひのきのぼう")
	if err != nil {
		t.Fatalf("ListBarterItem failed: %v", err)
	}

	// Customer with uninitialized depot tries to trade
	err = f.svc.TradeItem(ctx, "c_buyer", sale.ID, "customer-item-inst")
	if err != store.ErrTradeItemMissing {
		t.Errorf("expected ErrTradeItemMissing, got %v", err)
	}
}
