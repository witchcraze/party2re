package depot

import (
	"context"
	"errors"
	"sync"
	"testing"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	"github.com/witchcraze/party2re/internal/core/item"
)

type memoryDepotRepo struct {
	mu     sync.Mutex
	depots map[string]Depot
}

func newMemoryDepotRepo() *memoryDepotRepo {
	return &memoryDepotRepo{depots: make(map[string]Depot)}
}

func (r *memoryDepotRepo) FindByCharacterID(_ context.Context, characterID string) (Depot, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	d, ok := r.depots[characterID]
	if !ok {
		return Depot{}, ErrNotFound
	}
	return d, nil
}

func (r *memoryDepotRepo) FindByCharacterIDForUpdate(ctx context.Context, characterID string) (Depot, error) {
	return r.FindByCharacterID(ctx, characterID)
}

func (r *memoryDepotRepo) Save(_ context.Context, value Depot) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.depots[value.CharacterID] = value
	return nil
}

type memoryCharRepo struct {
	mu         sync.Mutex
	characters map[string]corecharacter.Character
}

func newMemoryCharRepo() *memoryCharRepo {
	return &memoryCharRepo{characters: make(map[string]corecharacter.Character)}
}

func (r *memoryCharRepo) FindByID(_ context.Context, id string) (corecharacter.Character, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.characters[id]
	if !ok {
		return corecharacter.Character{}, corecharacter.ErrNotFound
	}
	return c, nil
}

func (r *memoryCharRepo) FindByIDForUpdate(ctx context.Context, id string) (corecharacter.Character, error) {
	return r.FindByID(ctx, id)
}

func (r *memoryCharRepo) Update(_ context.Context, character corecharacter.Character) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.characters[character.ID] = character
	return nil
}

type memoryInvRepo struct {
	mu          sync.Mutex
	inventories map[string]coreinventory.Inventory
}

func newMemoryInvRepo() *memoryInvRepo {
	return &memoryInvRepo{inventories: make(map[string]coreinventory.Inventory)}
}

func (r *memoryInvRepo) FindByCharacterID(_ context.Context, characterID string) (coreinventory.Inventory, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	inv, ok := r.inventories[characterID]
	if !ok {
		return coreinventory.New(characterID)
	}
	return inv, nil
}

func (r *memoryInvRepo) FindByCharacterIDForUpdate(ctx context.Context, characterID string) (coreinventory.Inventory, error) {
	return r.FindByCharacterID(ctx, characterID)
}

func (r *memoryInvRepo) Save(_ context.Context, inventory coreinventory.Inventory) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.inventories[inventory.CharacterID] = inventory
	return nil
}

type memoryTxProvider struct{}

func (p *memoryTxProvider) RunInTx(ctx context.Context, fn func(ctx context.Context) error) error {
	return fn(ctx)
}

type memoryItemCatalog struct {
	items map[string]item.Definition
}

func newMemoryItemCatalog() *memoryItemCatalog {
	c := &memoryItemCatalog{items: make(map[string]item.Definition)}
	defWeapon, _ := item.NewEquipmentDefinition("wea-01", "Iron Sword", 1000, item.SlotMainHand)
	defShield, _ := item.NewEquipmentDefinition("arm-01", "Iron Shield", 800, item.SlotOffHand)
	defPotion, _ := item.NewDefinition("item-001", "Herb", 100)
	c.items[defWeapon.ID] = defWeapon
	c.items[defShield.ID] = defShield
	c.items[defPotion.ID] = defPotion
	return c
}

func (c *memoryItemCatalog) FindByID(id string) (item.Definition, error) {
	def, ok := c.items[id]
	if !ok {
		return item.Definition{}, errors.New("item not found")
	}
	return def, nil
}

type memoryCollectionRecorder struct {
	mu       sync.Mutex
	recorded []string
}

func (m *memoryCollectionRecorder) RecordItemDiscovered(_ context.Context, _, itemID, _, _ string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.recorded = append(m.recorded, itemID)
	return nil
}

func TestCalculateCapacity(t *testing.T) {
	tests := []struct {
		name      string
		jobLv     int
		exDepot   int
		overDepot int
		want      int
	}{
		{"Initial state (all zero)", 0, 0, 0, 5},
		{"JobLv 1", 1, 0, 0, 10},
		{"JobLv 2", 2, 0, 0, 15},
		{"JobLv 28", 28, 0, 0, 145},
		{"JobLv 29 (cap at 150)", 29, 0, 0, 150},
		{"JobLv 100", 100, 0, 0, 150},
		{"ExDepot 1", 0, 1, 0, 10},
		{"ExDepot 20 (max +100)", 0, 20, 0, 105},
		{"ExDepot 25 (clamped to 20)", 0, 25, 0, 105},
		{"OverDepot 1 (+50)", 0, 0, 1, 55},
		{"OverDepot 5 (max +250)", 0, 0, 5, 255},
		{"OverDepot 10 (clamped to 5)", 0, 0, 10, 255},
		{"Max Theoretical (150 + 100 + 250 = 500)", 29, 20, 5, 500},
		{"Negative numbers clamped", -5, -2, -1, 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CalculateCapacity(tt.jobLv, tt.exDepot, tt.overDepot)
			if got != tt.want {
				t.Errorf("CalculateCapacity(%d, %d, %d) = %d, want %d", tt.jobLv, tt.exDepot, tt.overDepot, got, tt.want)
			}
		})
	}
}

func TestExpansionCost(t *testing.T) {
	cost0, err := ExpansionCost(0)
	if err != nil || cost0 != 200000 {
		t.Errorf("expected 200000 at 0, got %d, err %v", cost0, err)
	}
	cost2, err := ExpansionCost(2)
	if err != nil || cost2 != 400000 {
		t.Errorf("expected 400000 at 2, got %d, err %v", cost2, err)
	}
	cost8, err := ExpansionCost(8)
	if err != nil || cost8 != 999999 {
		t.Errorf("expected 999999 at 8, got %d, err %v", cost8, err)
	}
	cost19, err := ExpansionCost(19)
	if err != nil || cost19 != 999999 {
		t.Errorf("expected 999999 at 19, got %d, err %v", cost19, err)
	}
	_, err = ExpansionCost(20)
	if !errors.Is(err, ErrDepotMaxExpanded) {
		t.Errorf("expected ErrDepotMaxExpanded at 20, got %v", err)
	}
}

// TestDepot_AddItem_StackingAndCapacity verifies Issue #452 fix:
// Stacking items merge even when at full capacity; non-stacking items are rejected.
func TestDepot_AddItem_StackingAndCapacity(t *testing.T) {
	dep, _ := NewDepot("char-1")
	dep.Capacity = 2

	itemA, _ := item.NewInstance("item-001", 1)
	itemB, _ := item.NewInstance("item-002", 1)
	itemAExtra, _ := item.NewInstance("item-001", 3)
	itemC, _ := item.NewInstance("item-003", 1)

	// Fill depot to capacity (2/2)
	if err := dep.AddItem(itemA); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := dep.AddItem(itemB); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(dep.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(dep.Items))
	}

	// Issue #452: Adding existing itemA when depot is at full capacity (2/2) MUST succeed!
	if err := dep.AddItem(itemAExtra); err != nil {
		t.Fatalf("expected stacking item to succeed at full capacity, got: %v", err)
	}
	if dep.Items[0].Quantity != 4 {
		t.Errorf("expected quantity 4, got %d", dep.Items[0].Quantity)
	}

	// Adding new non-existing itemC MUST fail with ErrDepotFull
	if err := dep.AddItem(itemC); !errors.Is(err, ErrDepotFull) {
		t.Fatalf("expected ErrDepotFull for new item, got: %v", err)
	}
}

func TestDepositAndWithdrawItem(t *testing.T) {
	ctx := context.Background()
	depotRepo := newMemoryDepotRepo()
	charRepo := newMemoryCharRepo()
	invRepo := newMemoryInvRepo()
	catalog := newMemoryItemCatalog()
	collector := &memoryCollectionRecorder{}

	char, _ := corecharacter.New("Item Collector")
	charRepo.characters[char.ID] = char

	inv, _ := coreinventory.New(char.ID)
	potion, _ := item.NewInstance("item-001", 3)
	sword, _ := item.NewInstance("wea-01", 1)
	_ = inv.Add(potion)
	_ = inv.Add(sword)
	invRepo.inventories[char.ID] = inv

	service, _ := NewService(
		depotRepo, charRepo, invRepo,
		WithItemDefinitionProvider(catalog),
		WithCollectionRecorder(collector),
	)

	// Deposit potion
	dep, err := service.DepositItem(ctx, char.ID, potion.ID)
	if err != nil {
		t.Fatalf("DepositItem error: %v", err)
	}
	if len(dep.Items) != 1 || dep.Items[0].DefinitionID != "item-001" {
		t.Fatalf("unexpected depot items: %#v", dep.Items)
	}

	// Withdraw potion
	dep, err = service.WithdrawItem(ctx, char.ID, potion.ID)
	if err != nil {
		t.Fatalf("WithdrawItem error: %v", err)
	}
	if len(dep.Items) != 0 {
		t.Fatalf("expected depot items count = 0, got %d", len(dep.Items))
	}

	// Verify collection was recorded on withdrawal
	if len(collector.recorded) != 1 || collector.recorded[0] != "item-001" {
		t.Errorf("expected item-001 recorded in collection, got %v", collector.recorded)
	}
}

func TestSellItemAndBatch(t *testing.T) {
	ctx := context.Background()
	depotRepo := newMemoryDepotRepo()
	charRepo := newMemoryCharRepo()
	invRepo := newMemoryInvRepo()
	catalog := newMemoryItemCatalog()

	char, _ := corecharacter.New("Seller")
	char.Money = 100
	charRepo.characters[char.ID] = char

	service, _ := NewService(depotRepo, charRepo, invRepo, WithItemDefinitionProvider(catalog))

	dep, _ := NewDepot(char.ID)
	sword, _ := item.NewInstance("wea-01", 1)     // Price 1000 -> 50% = 500
	shield, _ := item.NewInstance("arm-01", 1)    // Price 800 -> 50% = 400
	potions, _ := item.NewInstance("item-001", 2) // Price 100 -> 50% = 50 * 2 = 100
	_ = dep.AddItem(sword)
	_ = dep.AddItem(shield)
	_ = dep.AddItem(potions)
	_ = depotRepo.Save(ctx, dep)

	// Single item sell: sword
	dep, earned, err := service.SellItem(ctx, char.ID, sword.ID)
	if err != nil {
		t.Fatalf("SellItem error: %v", err)
	}
	if earned != 500 {
		t.Errorf("expected earned 500, got %d", earned)
	}
	updatedChar, _ := charRepo.FindByID(ctx, char.ID)
	if updatedChar.Money != 600 {
		t.Errorf("expected char money 600, got %d", updatedChar.Money)
	}
	if len(dep.Items) != 2 {
		t.Errorf("expected 2 items remaining, got %d", len(dep.Items))
	}

	// Batch sell: shield and potions
	dep, batchEarned, err := service.SellItems(ctx, char.ID, []string{shield.ID, potions.ID})
	if err != nil {
		t.Fatalf("SellItems error: %v", err)
	}
	if batchEarned != 500 { // 400 + 100
		t.Errorf("expected batchEarned 500, got %d", batchEarned)
	}
	updatedChar, _ = charRepo.FindByID(ctx, char.ID)
	if updatedChar.Money != 1100 {
		t.Errorf("expected char money 1100, got %d", updatedChar.Money)
	}
	if len(dep.Items) != 0 {
		t.Errorf("expected depot items empty, got %d", len(dep.Items))
	}
}

func TestSortItems(t *testing.T) {
	ctx := context.Background()
	depotRepo := newMemoryDepotRepo()
	charRepo := newMemoryCharRepo()
	invRepo := newMemoryInvRepo()
	catalog := newMemoryItemCatalog()

	char, _ := corecharacter.New("Sorter")
	charRepo.characters[char.ID] = char

	service, _ := NewService(depotRepo, charRepo, invRepo, WithItemDefinitionProvider(catalog))

	dep, _ := NewDepot(char.ID)
	potion, _ := item.NewInstance("item-001", 1) // Kind 3
	shield, _ := item.NewInstance("arm-01", 1)   // Kind 2
	sword, _ := item.NewInstance("wea-01", 1)    // Kind 1
	// Add in reverse order
	_ = dep.AddItem(potion)
	_ = dep.AddItem(shield)
	_ = dep.AddItem(sword)
	_ = depotRepo.Save(ctx, dep)

	sortedDep, err := service.SortItems(ctx, char.ID)
	if err != nil {
		t.Fatalf("SortItems error: %v", err)
	}
	if len(sortedDep.Items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(sortedDep.Items))
	}
	// Kind 1 (weapon) -> Kind 2 (armor) -> Kind 3 (item)
	if sortedDep.Items[0].DefinitionID != "wea-01" {
		t.Errorf("expected first item wea-01, got %s", sortedDep.Items[0].DefinitionID)
	}
	if sortedDep.Items[1].DefinitionID != "arm-01" {
		t.Errorf("expected second item arm-01, got %s", sortedDep.Items[1].DefinitionID)
	}
	if sortedDep.Items[2].DefinitionID != "item-001" {
		t.Errorf("expected third item item-001, got %s", sortedDep.Items[2].DefinitionID)
	}
}

func TestExpand(t *testing.T) {
	ctx := context.Background()
	depotRepo := newMemoryDepotRepo()
	charRepo := newMemoryCharRepo()
	invRepo := newMemoryInvRepo()

	char, _ := corecharacter.New("Expander")
	char.Money = 500000 // 500k G
	charRepo.characters[char.ID] = char

	service, _ := NewService(depotRepo, charRepo, invRepo)

	// First expansion: costs 200k G
	dep, err := service.Expand(ctx, char.ID)
	if err != nil {
		t.Fatalf("Expand error: %v", err)
	}
	if dep.ExDepot != 1 {
		t.Errorf("expected ExDepot 1, got %d", dep.ExDepot)
	}
	if dep.Capacity != 10 { // base 5 + 1*5 = 10
		t.Errorf("expected capacity 10, got %d", dep.Capacity)
	}
	updatedChar, _ := charRepo.FindByID(ctx, char.ID)
	if updatedChar.Money != 300000 {
		t.Errorf("expected money 300000, got %d", updatedChar.Money)
	}

	// Second expansion: costs 200k G
	dep, err = service.Expand(ctx, char.ID)
	if err != nil {
		t.Fatalf("second Expand error: %v", err)
	}
	if dep.ExDepot != 2 || dep.Capacity != 15 {
		t.Errorf("expected ExDepot 2, capacity 15, got %d, %d", dep.ExDepot, dep.Capacity)
	}

	// Third expansion: costs 400k G, but character only has 100k G -> fails
	_, err = service.Expand(ctx, char.ID)
	if !errors.Is(err, ErrInsufficientFunds) {
		t.Errorf("expected ErrInsufficientFunds, got %v", err)
	}
}

func TestSendMoneyAndItem(t *testing.T) {
	ctx := context.Background()
	depotRepo := newMemoryDepotRepo()
	charRepo := newMemoryCharRepo()
	invRepo := newMemoryInvRepo()
	txProv := &memoryTxProvider{}

	sender, _ := corecharacter.New("Sender")
	sender.Money = 1000
	charRepo.characters[sender.ID] = sender

	recipient, _ := corecharacter.New("Recipient")
	recipient.Money = 50
	charRepo.characters[recipient.ID] = recipient

	inv, _ := coreinventory.New(sender.ID)
	sword, _ := item.NewInstance("wea-01", 1)
	_ = inv.Add(sword)
	invRepo.inventories[sender.ID] = inv

	service, _ := NewService(depotRepo, charRepo, invRepo, WithTransactionProvider(txProv))

	// Send money
	_, err := service.SendMoney(ctx, sender.ID, recipient.ID, 300)
	if err != nil {
		t.Fatalf("SendMoney error: %v", err)
	}
	upSender, _ := charRepo.FindByID(ctx, sender.ID)
	upRecipient, _ := charRepo.FindByID(ctx, recipient.ID)
	if upSender.Money != 700 || upRecipient.Money != 350 {
		t.Errorf("unexpected money balances: sender %d, recipient %d", upSender.Money, upRecipient.Money)
	}

	// Send money - self transfer rejected
	_, err = service.SendMoney(ctx, sender.ID, sender.ID, 100)
	if !errors.Is(err, ErrSelfTransferNotAllowed) {
		t.Errorf("expected ErrSelfTransferNotAllowed, got %v", err)
	}

	// Send item
	_, err = service.SendItem(ctx, sender.ID, recipient.ID, sword.ID)
	if err != nil {
		t.Fatalf("SendItem error: %v", err)
	}

	// Verify item removed from sender inventory
	upInv, _ := invRepo.FindByCharacterID(ctx, sender.ID)
	if len(upInv.Items) != 0 {
		t.Errorf("expected sender inventory empty, got %d", len(upInv.Items))
	}

	// Verify item added to recipient depot
	recDep, _ := depotRepo.FindByCharacterID(ctx, recipient.ID)
	if len(recDep.Items) != 1 || recDep.Items[0].DefinitionID != "wea-01" {
		t.Errorf("expected sword in recipient depot, got %#v", recDep.Items)
	}
}
