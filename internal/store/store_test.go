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

// In-memory test mocks
type mockStoreRepo struct {
	stores    map[string]store.Store
	sales     map[string]store.Sale
	interiors map[string][]store.Interior
}

func newMockStoreRepo() *mockStoreRepo {
	return &mockStoreRepo{
		stores:    make(map[string]store.Store),
		sales:     make(map[string]store.Sale),
		interiors: make(map[string][]store.Interior),
	}
}

func (m *mockStoreRepo) GetStoreByCharacterID(ctx context.Context, characterID string) (store.Store, error) {
	for _, s := range m.stores {
		if s.CharacterID == characterID {
			return s, nil
		}
	}
	return store.Store{}, store.ErrStoreNotFound
}

func (m *mockStoreRepo) GetStoreByID(ctx context.Context, storeID string) (store.Store, error) {
	s, ok := m.stores[storeID]
	if !ok {
		return store.Store{}, store.ErrStoreNotFound
	}
	return s, nil
}

func (m *mockStoreRepo) GetStoreByName(ctx context.Context, storeName string) (store.Store, error) {
	for _, s := range m.stores {
		if s.StoreName == storeName {
			return s, nil
		}
	}
	return store.Store{}, store.ErrStoreNotFound
}

func (m *mockStoreRepo) CountActiveTownStores(ctx context.Context, townID string, now time.Time) (int, error) {
	count := 0
	for _, s := range m.stores {
		if s.TownID == townID && s.IsActive(now) {
			count++
		}
	}
	return count, nil
}

func (m *mockStoreRepo) ListActiveStoresInTown(ctx context.Context, townID string, now time.Time) ([]store.Store, error) {
	var list []store.Store
	for _, s := range m.stores {
		if s.TownID == townID && s.IsActive(now) {
			list = append(list, s)
		}
	}
	return list, nil
}

func (m *mockStoreRepo) SaveStore(ctx context.Context, s store.Store) error {
	m.stores[s.ID] = s
	return nil
}

func (m *mockStoreRepo) DeleteStore(ctx context.Context, storeID string) error {
	delete(m.stores, storeID)
	return nil
}

func (m *mockStoreRepo) GetSalesByStoreID(ctx context.Context, storeID string) ([]store.Sale, error) {
	var list []store.Sale
	for _, s := range m.sales {
		if s.StoreID == storeID {
			list = append(list, s)
		}
	}
	return list, nil
}

func (m *mockStoreRepo) GetSaleByIDForUpdate(ctx context.Context, saleID string) (store.Sale, error) {
	s, ok := m.sales[saleID]
	if !ok {
		return store.Sale{}, store.ErrListingNotFound
	}
	return s, nil
}

func (m *mockStoreRepo) SaveSale(ctx context.Context, sale store.Sale) error {
	m.sales[sale.ID] = sale
	return nil
}

func (m *mockStoreRepo) DeleteSale(ctx context.Context, saleID string) error {
	delete(m.sales, saleID)
	return nil
}

func (m *mockStoreRepo) GetInteriorsByStoreID(ctx context.Context, storeID string) ([]store.Interior, error) {
	return m.interiors[storeID], nil
}

func (m *mockStoreRepo) SaveInterior(ctx context.Context, interior store.Interior) error {
	list := m.interiors[interior.StoreID]
	found := false
	for i, in := range list {
		if in.ID == interior.ID {
			list[i] = interior
			found = true
			break
		}
	}
	if !found {
		list = append(list, interior)
	}
	m.interiors[interior.StoreID] = list
	return nil
}

func (m *mockStoreRepo) DeleteInteriorsByStoreID(ctx context.Context, storeID string) error {
	delete(m.interiors, storeID)
	return nil
}

func (m *mockStoreRepo) UpdateInteriorName(ctx context.Context, interiorID string, name string) error {
	for storeID, list := range m.interiors {
		for i, in := range list {
			if in.ID == interiorID {
				list[i].Name = name
				m.interiors[storeID] = list
				return nil
			}
		}
	}
	return store.ErrInteriorNotFound
}

type mockCharRepo struct {
	chars map[string]corecharacter.Character
}

func (m *mockCharRepo) FindByID(ctx context.Context, id string) (corecharacter.Character, error) {
	c, ok := m.chars[id]
	if !ok {
		return corecharacter.Character{}, corecharacter.ErrNotFound
	}
	return c, nil
}

func (m *mockCharRepo) FindByIDForUpdate(ctx context.Context, id string) (corecharacter.Character, error) {
	return m.FindByID(ctx, id)
}

func (m *mockCharRepo) Save(ctx context.Context, char corecharacter.Character) error {
	m.chars[char.ID] = char
	return nil
}

type mockDepotRepo struct {
	depots map[string]depot.Depot
}

func (m *mockDepotRepo) FindByCharacterID(ctx context.Context, characterID string) (depot.Depot, error) {
	d, ok := m.depots[characterID]
	if !ok {
		return depot.Depot{CharacterID: characterID, Capacity: 10, Items: []coreitem.Instance{}}, nil
	}
	return d, nil
}

func (m *mockDepotRepo) FindByCharacterIDForUpdate(ctx context.Context, characterID string) (depot.Depot, error) {
	return m.FindByCharacterID(ctx, characterID)
}

func (m *mockDepotRepo) Save(ctx context.Context, dep depot.Depot) error {
	m.depots[dep.CharacterID] = dep
	return nil
}

type mockCatalog struct {
	items map[string]coreitem.Definition
}

func (m *mockCatalog) FindByID(id string) (coreitem.Definition, error) {
	it, ok := m.items[id]
	if !ok {
		return coreitem.Definition{}, coreitem.ErrDefinitionNotFound
	}
	return it, nil
}

func (m *mockCatalog) FindByName(name string) (coreitem.Definition, error) {
	for _, it := range m.items {
		if it.Name == name {
			return it, nil
		}
	}
	return coreitem.Definition{}, coreitem.ErrDefinitionNotFound
}

type mockTxProvider struct{}

func (m *mockTxProvider) RunInTx(ctx context.Context, fn func(ctx context.Context) error) error {
	return fn(ctx)
}

func setupTestService(t *testing.T) (*store.Service, *mockStoreRepo, *mockCharRepo, *mockDepotRepo, *mockCatalog) {
	storeRepo := newMockStoreRepo()
	charRepo := &mockCharRepo{chars: make(map[string]corecharacter.Character)}
	depotRepo := &mockDepotRepo{depots: make(map[string]depot.Depot)}
	catalog := &mockCatalog{items: map[string]coreitem.Definition{
		"wpn_001": {ID: "wpn_001", Name: "ひのきのぼう", Price: 100},
		"wpn_002": {ID: "wpn_002", Name: "どうのつるぎ", Price: 500},
		"itm_001": {ID: "itm_001", Name: "やくそう", Price: 10},
	}}

	c1, _ := corecharacter.New("Player1")
	c1.ID = "c1"
	_ = c1.DeductMoney(c1.Money)
	_ = c1.AddMoney(100000)
	charRepo.chars["c1"] = c1

	c2, _ := corecharacter.New("Player2")
	c2.ID = "c2"
	_ = c2.DeductMoney(c2.Money)
	_ = c2.AddMoney(100000)
	charRepo.chars["c2"] = c2

	svc := store.NewService(
		storeRepo,
		charRepo,
		depotRepo,
		catalog,
		&mockTxProvider{},
		store.WithNowFunc(func() time.Time { return time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC) }),
	)

	return svc, storeRepo, charRepo, depotRepo, catalog
}

func TestBuildStore(t *testing.T) {
	svc, _, charRepo, _, _ := setupTestService(t)
	ctx := context.Background()

	// Success
	res, err := svc.BuildStore(ctx, "c1", "town1", "001", "マイショップ")
	if err != nil {
		t.Fatalf("BuildStore failed: %v", err)
	}
	if res.StoreName != "マイショップ" {
		t.Errorf("expected store name マイショップ, got %s", res.StoreName)
	}
	c1, _ := charRepo.FindByID(ctx, "c1")
	if c1.Money != 50000 {
		t.Errorf("expected 50000 gold remaining, got %d", c1.Money)
	}

	// Already owns store
	_, err = svc.BuildStore(ctx, "c1", "town1", "001", "二号店")
	if err != store.ErrAlreadyOwnsStore {
		t.Errorf("expected ErrAlreadyOwnsStore, got %v", err)
	}

	// Invalid house style
	_, err = svc.BuildStore(ctx, "c2", "town1", "999", "店")
	if err != store.ErrInvalidHouseStyle {
		t.Errorf("expected ErrInvalidHouseStyle, got %v", err)
	}

	// Invalid town
	_, err = svc.BuildStore(ctx, "c2", "town99", "001", "店")
	if err != store.ErrInvalidTownID {
		t.Errorf("expected ErrInvalidTownID, got %v", err)
	}

	// Insufficient funds
	c2, _ := charRepo.FindByID(ctx, "c2")
	_ = c2.DeductMoney(95000)
	_ = charRepo.Save(ctx, c2)
	_, err = svc.BuildStore(ctx, "c2", "town1", "001", "店")
	if err != store.ErrInsufficientFunds {
		t.Errorf("expected ErrInsufficientFunds, got %v", err)
	}
}

func TestStoreSalesAndTrading(t *testing.T) {
	svc, _, charRepo, depotRepo, _ := setupTestService(t)
	ctx := context.Background()

	// Build store for c1
	_, err := svc.BuildStore(ctx, "c1", "town1", "001", "店一号")
	if err != nil {
		t.Fatalf("BuildStore failed: %v", err)
	}

	// Put item in c1 depot
	dep1, _ := depotRepo.FindByCharacterID(ctx, "c1")
	dep1.Items = []coreitem.Instance{
		{ID: "inst1", DefinitionID: "wpn_001", Quantity: 1},
		{ID: "inst2", DefinitionID: "wpn_002", Quantity: 2},
	}
	_ = depotRepo.Save(ctx, dep1)

	// ListGoldItem
	sale, err := svc.ListGoldItem(ctx, "c1", "inst1", 3000)
	if err != nil {
		t.Fatalf("ListGoldItem failed: %v", err)
	}
	if sale.Price != 3000 || sale.ItemName != "ひのきのぼう" {
		t.Errorf("unexpected sale: %+v", sale)
	}

	// Depot should have 1 item left (inst2 with quantity 2)
	dep1, _ = depotRepo.FindByCharacterID(ctx, "c1")
	if len(dep1.Items) != 1 || dep1.Items[0].ID != "inst2" {
		t.Fatalf("item not removed from depot properly: %+v", dep1.Items)
	}

	// c2 buys the item
	err = svc.BuyItem(ctx, "c2", sale.ID)
	if err != nil {
		t.Fatalf("BuyItem failed: %v", err)
	}

	c1, _ := charRepo.FindByID(ctx, "c1")
	c2, _ := charRepo.FindByID(ctx, "c2")
	if c1.Money != 53000 { // 50,000 + 3,000
		t.Errorf("expected c1 money 53000, got %d", c1.Money)
	}
	if c2.Money != 97000 { // 100,000 - 3,000
		t.Errorf("expected c2 money 97000, got %d", c2.Money)
	}

	dep2, _ := depotRepo.FindByCharacterID(ctx, "c2")
	if len(dep2.Items) != 1 || dep2.Items[0].DefinitionID != "wpn_001" {
		t.Fatalf("c2 depot should have bought item: %+v", dep2.Items)
	}

	// ListBarterItem
	barterSale, err := svc.ListBarterItem(ctx, "c1", "inst2", "やくそう")
	if err != nil {
		t.Fatalf("ListBarterItem failed: %v", err)
	}
	if barterSale.WishItemName != "やくそう" {
		t.Errorf("expected wish item やくそう, got %s", barterSale.WishItemName)
	}

	// c2 tries to barter without herb in depot
	err = svc.TradeItem(ctx, "c2", barterSale.ID, "inst_missing")
	if err != store.ErrTradeItemMissing {
		t.Errorf("expected ErrTradeItemMissing, got %v", err)
	}

	// Add herb to c2 depot
	dep2.Items = append(dep2.Items, coreitem.Instance{ID: "c2_herb", DefinitionID: "itm_001", Quantity: 1})
	_ = depotRepo.Save(ctx, dep2)

	// Perform barter trade
	err = svc.TradeItem(ctx, "c2", barterSale.ID, "c2_herb")
	if err != nil {
		t.Fatalf("TradeItem failed: %v", err)
	}

	// Verify items swapped
	dep1, _ = depotRepo.FindByCharacterID(ctx, "c1")
	foundHerb := false
	for _, it := range dep1.Items {
		if it.DefinitionID == "itm_001" {
			foundHerb = true
		}
	}
	if !foundHerb {
		t.Errorf("seller c1 should have received herb in depot")
	}

	dep2, _ = depotRepo.FindByCharacterID(ctx, "c2")
	foundSword := false
	for _, it := range dep2.Items {
		if it.DefinitionID == "wpn_002" {
			foundSword = true
		}
	}
	if !foundSword {
		t.Errorf("buyer c2 should have received sword in depot")
	}
}

func TestWithdrawListing(t *testing.T) {
	svc, _, _, depotRepo, _ := setupTestService(t)
	ctx := context.Background()

	_, _ = svc.BuildStore(ctx, "c1", "town1", "001", "店")
	dep, _ := depotRepo.FindByCharacterID(ctx, "c1")
	dep.Items = []coreitem.Instance{{ID: "i1", DefinitionID: "wpn_001", Quantity: 1}}
	_ = depotRepo.Save(ctx, dep)

	sale, err := svc.ListGoldItem(ctx, "c1", "i1", 1000)
	if err != nil {
		t.Fatalf("ListGoldItem failed: %v", err)
	}

	err = svc.WithdrawListing(ctx, "c1", sale.ID)
	if err != nil {
		t.Fatalf("WithdrawListing failed: %v", err)
	}

	dep, _ = depotRepo.FindByCharacterID(ctx, "c1")
	if len(dep.Items) != 1 || dep.Items[0].DefinitionID != "wpn_001" {
		t.Errorf("item should be restored to depot: %+v", dep.Items)
	}
}

func TestStoreCustomization(t *testing.T) {
	svc, _, charRepo, _, _ := setupTestService(t)
	ctx := context.Background()

	_, _ = svc.BuildStore(ctx, "c1", "town1", "001", "元店名")

	// ChangeStoreName
	err := svc.ChangeStoreName(ctx, "c1", "新店名")
	if err != nil {
		t.Fatalf("ChangeStoreName failed: %v", err)
	}
	c1, _ := charRepo.FindByID(ctx, "c1")
	if c1.Money != 45000 { // 50,000 - 5,000
		t.Errorf("expected 45000 gold, got %d", c1.Money)
	}

	// ChangeWallpaper
	err = svc.ChangeWallpaper(ctx, "c1", "casino")
	if err != nil {
		t.Fatalf("ChangeWallpaper failed: %v", err)
	}
	c1, _ = charRepo.FindByID(ctx, "c1")
	if c1.Money != 42500 { // 45,000 - 2,500
		t.Errorf("expected 42500 gold, got %d", c1.Money)
	}

	// AddInterior
	interior, err := svc.AddInterior(ctx, "c1", "001")
	if err != nil {
		t.Fatalf("AddInterior failed: %v", err)
	}
	if interior.FurnitureID != "001" {
		t.Errorf("unexpected furniture ID: %s", interior.FurnitureID)
	}
	c1, _ = charRepo.FindByID(ctx, "c1")
	if c1.Money != 41500 { // 42,500 - 1,000
		t.Errorf("expected 41500 gold, got %d", c1.Money)
	}

	// RenameInterior
	err = svc.RenameInterior(ctx, "c1", interior.ID, "かっこいい机")
	if err != nil {
		t.Fatalf("RenameInterior failed: %v", err)
	}

	// CleanInteriors
	err = svc.CleanInteriors(ctx, "c1")
	if err != nil {
		t.Fatalf("CleanInteriors failed: %v", err)
	}
}
