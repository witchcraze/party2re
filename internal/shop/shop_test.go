package shop_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	"github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/shop"
)

type characterRepoStub struct {
	mu    sync.Mutex
	chars map[string]corecharacter.Character
}

func newCharacterRepoStub() *characterRepoStub {
	return &characterRepoStub{chars: make(map[string]corecharacter.Character)}
}

func (r *characterRepoStub) FindByID(_ context.Context, id string) (corecharacter.Character, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.chars[id]
	if !ok {
		return corecharacter.Character{}, corecharacter.ErrNotFound
	}
	return c, nil
}

func (r *characterRepoStub) Update(_ context.Context, value corecharacter.Character) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.chars[value.ID] = value
	return nil
}

type inventoryRepoStub struct {
	mu          sync.Mutex
	inventories map[string]coreinventory.Inventory
}

func newInventoryRepoStub() *inventoryRepoStub {
	return &inventoryRepoStub{inventories: make(map[string]coreinventory.Inventory)}
}

func (r *inventoryRepoStub) FindByCharacterID(_ context.Context, characterID string) (coreinventory.Inventory, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	inv, ok := r.inventories[characterID]
	if !ok {
		return coreinventory.New(characterID)
	}
	return inv, nil
}

func (r *inventoryRepoStub) Save(_ context.Context, value coreinventory.Inventory) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.inventories[value.CharacterID] = value
	return nil
}

func newTestSetup(t *testing.T) (*shop.Service, *characterRepoStub, *inventoryRepoStub, *item.Catalog) {
	t.Helper()
	sword, err := item.NewEquipmentDefinition("bronze_sword", "Bronze Sword", 100, item.SlotMainHand)
	if err != nil {
		t.Fatal(err)
	}
	potion, err := item.NewDefinition("herb", "Herb", 30)
	if err != nil {
		t.Fatal(err)
	}
	oddItem, err := item.NewDefinition("odd_item", "Odd Item", 75)
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := item.NewCatalog([]item.Definition{sword, potion, oddItem})
	if err != nil {
		t.Fatal(err)
	}

	charRepo := newCharacterRepoStub()
	invRepo := newInventoryRepoStub()

	service, err := shop.NewService(charRepo, invRepo, catalog)
	if err != nil {
		t.Fatal(err)
	}

	return service, charRepo, invRepo, catalog
}

func createTestCharacter(t *testing.T, charRepo *characterRepoStub, name string, money int) corecharacter.Character {
	t.Helper()
	c, err := corecharacter.New(name)
	if err != nil {
		t.Fatal(err)
	}
	c.Money = money
	if err := charRepo.Update(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	return c
}

func TestPurchaseSuccess(t *testing.T) {
	service, charRepo, invRepo, _ := newTestSetup(t)
	char := createTestCharacter(t, charRepo, "Hero", 200)

	result, err := service.Purchase(context.Background(), char.ID, "bronze_sword", 1)
	if err != nil {
		t.Fatalf("Purchase() error = %v", err)
	}

	if result.TotalPrice != 100 {
		t.Errorf("TotalPrice = %d, want 100", result.TotalPrice)
	}
	if result.Character.Money != 100 {
		t.Errorf("Character.Money = %d, want 100", result.Character.Money)
	}
	if len(result.Inventory.Items) != 1 {
		t.Fatalf("Inventory items count = %d, want 1", len(result.Inventory.Items))
	}
	if result.ItemInstance.DefinitionID != "bronze_sword" {
		t.Errorf("ItemInstance.DefinitionID = %s, want bronze_sword", result.ItemInstance.DefinitionID)
	}

	// Verify persistence in repos
	savedChar, err := charRepo.FindByID(context.Background(), char.ID)
	if err != nil {
		t.Fatal(err)
	}
	if savedChar.Money != 100 {
		t.Errorf("persisted Character.Money = %d, want 100", savedChar.Money)
	}

	savedInv, err := invRepo.FindByCharacterID(context.Background(), char.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(savedInv.Items) != 1 || savedInv.Items[0].DefinitionID != "bronze_sword" {
		t.Errorf("persisted Inventory = %#v", savedInv)
	}
}

func TestPurchaseMultipleQuantity(t *testing.T) {
	service, charRepo, _, _ := newTestSetup(t)
	char := createTestCharacter(t, charRepo, "Hero", 200)

	result, err := service.Purchase(context.Background(), char.ID, "herb", 3)
	if err != nil {
		t.Fatalf("Purchase() error = %v", err)
	}

	if result.TotalPrice != 90 {
		t.Errorf("TotalPrice = %d, want 90", result.TotalPrice)
	}
	if result.Character.Money != 110 {
		t.Errorf("Character.Money = %d, want 110", result.Character.Money)
	}
	if result.ItemInstance.Quantity != 3 {
		t.Errorf("ItemInstance.Quantity = %d, want 3", result.ItemInstance.Quantity)
	}
}

func TestPurchaseInsufficientFunds(t *testing.T) {
	service, charRepo, invRepo, _ := newTestSetup(t)
	char := createTestCharacter(t, charRepo, "Hero", 50)

	_, err := service.Purchase(context.Background(), char.ID, "bronze_sword", 1)
	if !errors.Is(err, shop.ErrInsufficientFunds) {
		t.Fatalf("Purchase() error = %v, want %v", err, shop.ErrInsufficientFunds)
	}

	// Verify unchanged state
	savedChar, _ := charRepo.FindByID(context.Background(), char.ID)
	if savedChar.Money != 50 {
		t.Errorf("Character.Money changed = %d, want 50", savedChar.Money)
	}
	savedInv, _ := invRepo.FindByCharacterID(context.Background(), char.ID)
	if len(savedInv.Items) != 0 {
		t.Errorf("Inventory items should be empty, got %d", len(savedInv.Items))
	}
}

func TestPurchaseInvalidItem(t *testing.T) {
	service, charRepo, _, _ := newTestSetup(t)
	char := createTestCharacter(t, charRepo, "Hero", 200)

	_, err := service.Purchase(context.Background(), char.ID, "nonexistent_item", 1)
	if !errors.Is(err, shop.ErrItemNotFound) {
		t.Fatalf("Purchase() error = %v, want %v", err, shop.ErrItemNotFound)
	}
}

func TestPurchaseInvalidQuantity(t *testing.T) {
	service, charRepo, _, _ := newTestSetup(t)
	char := createTestCharacter(t, charRepo, "Hero", 200)

	if _, err := service.Purchase(context.Background(), char.ID, "bronze_sword", 0); !errors.Is(err, shop.ErrInvalidQuantity) {
		t.Errorf("Purchase(0) error = %v, want %v", err, shop.ErrInvalidQuantity)
	}
	if _, err := service.Purchase(context.Background(), char.ID, "bronze_sword", -1); !errors.Is(err, shop.ErrInvalidQuantity) {
		t.Errorf("Purchase(-1) error = %v, want %v", err, shop.ErrInvalidQuantity)
	}
}

func TestPurchaseCharacterNotFound(t *testing.T) {
	service, _, _, _ := newTestSetup(t)

	if _, err := service.Purchase(context.Background(), "unknown_id", "bronze_sword", 1); !errors.Is(err, corecharacter.ErrNotFound) {
		t.Fatalf("Purchase(unknown) error = %v, want %v", err, corecharacter.ErrNotFound)
	}
}

func TestSellSuccess(t *testing.T) {
	service, charRepo, invRepo, _ := newTestSetup(t)
	char := createTestCharacter(t, charRepo, "Hero", 100)

	// Add item to inventory first
	inv, _ := invRepo.FindByCharacterID(context.Background(), char.ID)
	swordInst, err := item.NewInstance("bronze_sword", 1)
	if err != nil {
		t.Fatal(err)
	}
	_ = inv.Add(swordInst)
	_ = invRepo.Save(context.Background(), inv)

	result, err := service.Sell(context.Background(), char.ID, swordInst.ID, 1)
	if err != nil {
		t.Fatalf("Sell() error = %v", err)
	}

	// 100 G sword sells for 50 G (50%)
	if result.TotalPayout != 50 {
		t.Errorf("TotalPayout = %d, want 50", result.TotalPayout)
	}
	if result.Character.Money != 150 {
		t.Errorf("Character.Money = %d, want 150", result.Character.Money)
	}
	if len(result.Inventory.Items) != 0 {
		t.Errorf("Inventory should be empty after selling single instance, got %d items", len(result.Inventory.Items))
	}

	// Verify persistence
	savedChar, _ := charRepo.FindByID(context.Background(), char.ID)
	if savedChar.Money != 150 {
		t.Errorf("persisted Character.Money = %d, want 150", savedChar.Money)
	}
	savedInv, _ := invRepo.FindByCharacterID(context.Background(), char.ID)
	if len(savedInv.Items) != 0 {
		t.Errorf("persisted Inventory items = %d, want 0", len(savedInv.Items))
	}
}

func TestSellPartialQuantity(t *testing.T) {
	service, charRepo, invRepo, _ := newTestSetup(t)
	char := createTestCharacter(t, charRepo, "Hero", 100)

	inv, _ := invRepo.FindByCharacterID(context.Background(), char.ID)
	herbInst, err := item.NewInstance("herb", 5)
	if err != nil {
		t.Fatal(err)
	}
	_ = inv.Add(herbInst)
	_ = invRepo.Save(context.Background(), inv)

	// Herb price = 30 G -> sell price = 15 G each. Selling 2 = 30 G payout.
	result, err := service.Sell(context.Background(), char.ID, herbInst.ID, 2)
	if err != nil {
		t.Fatalf("Sell() error = %v", err)
	}

	if result.TotalPayout != 30 {
		t.Errorf("TotalPayout = %d, want 30", result.TotalPayout)
	}
	if result.Character.Money != 130 {
		t.Errorf("Character.Money = %d, want 130", result.Character.Money)
	}
	if len(result.Inventory.Items) != 1 || result.Inventory.Items[0].Quantity != 3 {
		t.Errorf("Inventory items = %#v, want 1 item with quantity 3", result.Inventory.Items)
	}
}

func TestSellUnownedItem(t *testing.T) {
	service, charRepo, _, _ := newTestSetup(t)
	char := createTestCharacter(t, charRepo, "Hero", 100)

	_, err := service.Sell(context.Background(), char.ID, "nonexistent_instance_id", 1)
	if !errors.Is(err, shop.ErrUnownedItem) {
		t.Fatalf("Sell() error = %v, want %v", err, shop.ErrUnownedItem)
	}
}

func TestSellQuantityGreaterThanOwned(t *testing.T) {
	service, charRepo, invRepo, _ := newTestSetup(t)
	char := createTestCharacter(t, charRepo, "Hero", 100)

	inv, _ := invRepo.FindByCharacterID(context.Background(), char.ID)
	herbInst, err := item.NewInstance("herb", 2)
	if err != nil {
		t.Fatal(err)
	}
	_ = inv.Add(herbInst)
	_ = invRepo.Save(context.Background(), inv)

	_, err = service.Sell(context.Background(), char.ID, herbInst.ID, 5)
	if !errors.Is(err, shop.ErrInvalidQuantity) {
		t.Fatalf("Sell() error = %v, want %v", err, shop.ErrInvalidQuantity)
	}
}

func TestSellInvalidQuantity(t *testing.T) {
	service, charRepo, _, _ := newTestSetup(t)
	char := createTestCharacter(t, charRepo, "Hero", 100)

	if _, err := service.Sell(context.Background(), char.ID, "inst_id", 0); !errors.Is(err, shop.ErrInvalidQuantity) {
		t.Errorf("Sell(0) error = %v, want %v", err, shop.ErrInvalidQuantity)
	}
	if _, err := service.Sell(context.Background(), char.ID, "inst_id", -1); !errors.Is(err, shop.ErrInvalidQuantity) {
		t.Errorf("Sell(-1) error = %v, want %v", err, shop.ErrInvalidQuantity)
	}
}

func TestCalculateSellPrice(t *testing.T) {
	service, _, _, _ := newTestSetup(t)

	tests := []struct {
		price int
		want  int
	}{
		{price: 100, want: 50},
		{price: 75, want: 37}, // integer floor
		{price: 30, want: 15},
		{price: 1, want: 0},
		{price: 0, want: 0},
	}

	for _, tt := range tests {
		got := service.CalculateSellPrice(tt.price)
		if got != tt.want {
			t.Errorf("CalculateSellPrice(%d) = %d, want %d", tt.price, got, tt.want)
		}
	}
}

func TestShopNewServiceNilDependencies(t *testing.T) {
	charRepo := newCharacterRepoStub()
	invRepo := newInventoryRepoStub()
	catalog, _ := item.NewCatalog([]item.Definition{})

	if _, err := shop.NewService(nil, invRepo, catalog); err == nil {
		t.Error("NewService(nil, inv, cat) expected error, got nil")
	}
	if _, err := shop.NewService(charRepo, nil, catalog); err == nil {
		t.Error("NewService(char, nil, cat) expected error, got nil")
	}
	if _, err := shop.NewService(charRepo, invRepo, nil); err == nil {
		t.Error("NewService(char, inv, nil) expected error, got nil")
	}
}

func TestPurchase_QuantityBounds(t *testing.T) {
	service, charRepo, _, _ := newTestSetup(t)
	char := createTestCharacter(t, charRepo, "Hero", 10_000_000)

	// MaxTransactionQuantity is 9999
	// 1. Exactly MaxTransactionQuantity -> succeeds (herb is 30 gold each)
	res, err := service.Purchase(context.Background(), char.ID, "herb", shop.MaxTransactionQuantity)
	if err != nil {
		t.Fatalf("Purchase(MaxTransactionQuantity) failed: %v", err)
	}
	if res.TotalPrice != 30*shop.MaxTransactionQuantity {
		t.Errorf("TotalPrice = %d, want %d", res.TotalPrice, 30*shop.MaxTransactionQuantity)
	}

	// 2. MaxTransactionQuantity + 1 -> fails with ErrInvalidQuantity
	_, err = service.Purchase(context.Background(), char.ID, "herb", shop.MaxTransactionQuantity+1)
	if !errors.Is(err, shop.ErrInvalidQuantity) {
		t.Errorf("Purchase(MaxTransactionQuantity + 1) error = %v, want %v", err, shop.ErrInvalidQuantity)
	}

	// 3. Negative & Zero -> fails with ErrInvalidQuantity
	for _, q := range []int{0, -1, -999} {
		_, err := service.Purchase(context.Background(), char.ID, "herb", q)
		if !errors.Is(err, shop.ErrInvalidQuantity) {
			t.Errorf("Purchase(%d) error = %v, want %v", q, err, shop.ErrInvalidQuantity)
		}
	}
}

func TestPurchase_IntegerOverflowProtection(t *testing.T) {
	charRepo := newCharacterRepoStub()
	invRepo := newInventoryRepoStub()
	char := createTestCharacter(t, charRepo, "Hero", 100)

	// Extreme price * quantity resulting in overflow
	// e.g. price > math.MaxInt / quantity
	hugeItem, err := item.NewDefinition("huge_item", "Huge Item", 5_000_000_000_000_000_000)
	if err != nil {
		t.Fatal(err)
	}
	catalog, _ := item.NewCatalog([]item.Definition{hugeItem})
	service, _ := shop.NewService(charRepo, invRepo, catalog)

	_, err = service.Purchase(context.Background(), char.ID, "huge_item", 2)
	if !errors.Is(err, shop.ErrPriceOverflow) {
		t.Errorf("Purchase with overflowing price returned %v, want %v", err, shop.ErrPriceOverflow)
	}
}

func TestSell_QuantityBounds(t *testing.T) {
	service, charRepo, invRepo, _ := newTestSetup(t)
	char := createTestCharacter(t, charRepo, "Hero", 100)

	inv, _ := invRepo.FindByCharacterID(context.Background(), char.ID)
	herbInst, err := item.NewInstance("herb", shop.MaxTransactionQuantity)
	if err != nil {
		t.Fatal(err)
	}
	_ = inv.Add(herbInst)
	_ = invRepo.Save(context.Background(), inv)

	// MaxTransactionQuantity + 1 -> fails with ErrInvalidQuantity
	_, err = service.Sell(context.Background(), char.ID, herbInst.ID, shop.MaxTransactionQuantity+1)
	if !errors.Is(err, shop.ErrInvalidQuantity) {
		t.Errorf("Sell(MaxTransactionQuantity + 1) error = %v, want %v", err, shop.ErrInvalidQuantity)
	}

	// Valid MaxTransactionQuantity -> succeeds
	res, err := service.Sell(context.Background(), char.ID, herbInst.ID, shop.MaxTransactionQuantity)
	if err != nil {
		t.Fatalf("Sell(MaxTransactionQuantity) error = %v", err)
	}
	if res.TotalPayout != 15*shop.MaxTransactionQuantity {
		t.Errorf("TotalPayout = %d, want %d", res.TotalPayout, 15*shop.MaxTransactionQuantity)
	}
}

func TestSell_IntegerOverflowProtection(t *testing.T) {
	// Item with base price 6_000_000_000_000_000_000 -> sell price = 3_000_000_000_000_000_000
	// 3_000_000_000_000_000_000 * 4 overflows int64
	hugeItem, err := item.NewDefinition("huge_gem", "Huge Gem", 6_000_000_000_000_000_000)
	if err != nil {
		t.Fatal(err)
	}
	catalog, _ := item.NewCatalog([]item.Definition{hugeItem})
	charRepo := newCharacterRepoStub()
	invRepo := newInventoryRepoStub()
	service, _ := shop.NewService(charRepo, invRepo, catalog)

	char := createTestCharacter(t, charRepo, "Hero", 100)
	inv, _ := invRepo.FindByCharacterID(context.Background(), char.ID)
	gemInst, _ := item.NewInstance("huge_gem", 10)
	_ = inv.Add(gemInst)
	_ = invRepo.Save(context.Background(), inv)

	_, err = service.Sell(context.Background(), char.ID, gemInst.ID, 4)
	if !errors.Is(err, shop.ErrPriceOverflow) {
		t.Errorf("Sell with overflowing payout returned %v, want %v", err, shop.ErrPriceOverflow)
	}
}
