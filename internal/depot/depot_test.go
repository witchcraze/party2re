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

func TestDepot_RefreshCapacity(t *testing.T) {
	d, err := NewDepot("char-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d.Capacity != MinDepotCapacity {
		t.Errorf("expected initial capacity %d, got %d", MinDepotCapacity, d.Capacity)
	}

	// Refresh with JobLevel 20 -> 20 * 5 + 5 = 105
	d.RefreshCapacity(20, 0)
	if d.Capacity != 105 {
		t.Errorf("expected capacity 105 for JobLevel 20, got %d", d.Capacity)
	}

	// Refresh with OverDepot 2 -> 105 + 100 = 205
	d.RefreshCapacity(20, 2)
	if d.Capacity != 205 {
		t.Errorf("expected capacity 205 for JobLevel 20 + OverDepot 2, got %d", d.Capacity)
	}

	// Refresh with ExDepot 5, JobLevel 30 (clamped to base 150) -> 150 + 25 + 50 = 225
	d.ExDepot = 5
	d.RefreshCapacity(30, 1)
	if d.Capacity != 225 {
		t.Errorf("expected capacity 225, got %d", d.Capacity)
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

func TestDepot_AddItem_EnhancementLevels(t *testing.T) {
	dep, _ := NewDepot("char-enh-test")
	dep.Capacity = 4

	sword0, _ := item.NewInstanceWithEnhancement("sword-1", 1, 0)
	sword5, _ := item.NewInstanceWithEnhancement("sword-1", 1, 5)
	sword5Second, _ := item.NewInstanceWithEnhancement("sword-1", 1, 5)

	// 1. Add +0 sword
	if err := dep.AddItem(sword0); err != nil {
		t.Fatalf("failed to add +0 sword: %v", err)
	}
	if len(dep.Items) != 1 || dep.Items[0].EnhancementLevel != 0 {
		t.Fatalf("expected 1 item with +0, got %+v", dep.Items)
	}

	// 2. Add +5 sword: MUST NOT collapse into +0 slot
	if err := dep.AddItem(sword5); err != nil {
		t.Fatalf("failed to add +5 sword: %v", err)
	}
	if len(dep.Items) != 2 {
		t.Fatalf("expected 2 distinct slots for +0 and +5 swords, got %d", len(dep.Items))
	}
	if dep.Items[0].EnhancementLevel != 0 || dep.Items[0].Quantity != 1 {
		t.Errorf("slot 0 corrupted: %+v", dep.Items[0])
	}
	if dep.Items[1].EnhancementLevel != 5 || dep.Items[1].Quantity != 1 {
		t.Errorf("slot 1 corrupted: %+v", dep.Items[1])
	}

	// 3. Add second +5 sword: upgraded equipment must NOT merge into existing +5 slot
	if err := dep.AddItem(sword5Second); err != nil {
		t.Fatalf("failed to add second +5 sword: %v", err)
	}
	if len(dep.Items) != 3 {
		t.Fatalf("expected 3 distinct slots, got %d", len(dep.Items))
	}
	if dep.Items[2].EnhancementLevel != 5 || dep.Items[2].Quantity != 1 {
		t.Errorf("slot 2 corrupted: %+v", dep.Items[2])
	}

	// 4. Add unenhanced consumable: should merge with identical unenhanced consumable
	herbA, _ := item.NewInstanceWithEnhancement("herb-1", 2, 0)
	herbB, _ := item.NewInstanceWithEnhancement("herb-1", 3, 0)
	if err := dep.AddItem(herbA); err != nil {
		t.Fatalf("failed to add herbA: %v", err)
	}
	if len(dep.Items) != 4 {
		t.Fatalf("expected 4 slots (3 swords + 1 herb), got %d", len(dep.Items))
	}

	// Depot is now at full capacity (4/4).
	// Adding more unenhanced herbs MUST succeed because they stack into slot 3.
	if err := dep.AddItem(herbB); err != nil {
		t.Fatalf("expected stacking herbB to succeed at full capacity, got %v", err)
	}
	if dep.Items[3].Quantity != 5 {
		t.Errorf("expected herb quantity 5, got %d", dep.Items[3].Quantity)
	}

	// Adding another +5 sword at full capacity MUST fail with ErrDepotFull
	swordExtra, _ := item.NewInstanceWithEnhancement("sword-1", 1, 5)
	if err := dep.AddItem(swordExtra); !errors.Is(err, ErrDepotFull) {
		t.Errorf("expected ErrDepotFull for enhanced equipment at full capacity, got %v", err)
	}
}

// TestDepot_AddItem_EquipmentNeverStacks verifies Issue #563:
// Equipment items (weapons, armor, shields, accessories) must NEVER stack into a single slot
// even if both instances are unenhanced (+0). Each equipment piece occupies an individual slot.
func TestDepot_AddItem_EquipmentNeverStacks(t *testing.T) {
	catalog := newMemoryItemCatalog() // wea-01 (main-hand), arm-01 (off-hand), item-001 (consumable)

	dep, _ := NewDepot("char-equip-stack-test")
	dep.Capacity = 3
	dep.SetItemDefinitionProvider(catalog)

	// 1. Add first unenhanced +0 sword (wea-01)
	swordA, _ := item.NewInstanceWithEnhancement("wea-01", 1, 0)
	if err := dep.AddItem(swordA); err != nil {
		t.Fatalf("failed to add first +0 sword: %v", err)
	}
	if len(dep.Items) != 1 {
		t.Fatalf("expected 1 item slot, got %d", len(dep.Items))
	}

	// 2. Add second identical unenhanced +0 sword (wea-01)
	// MUST NOT merge into slot 0! Must create a distinct second slot.
	swordB, _ := item.NewInstanceWithEnhancement("wea-01", 1, 0)
	if err := dep.AddItem(swordB); err != nil {
		t.Fatalf("failed to add second +0 sword: %v", err)
	}
	if len(dep.Items) != 2 {
		t.Fatalf("expected 2 distinct slots for two +0 swords, got %d", len(dep.Items))
	}
	if dep.Items[0].Quantity != 1 || dep.Items[1].Quantity != 1 {
		t.Errorf("expected both swords to have Quantity=1, got slot0=%d, slot1=%d",
			dep.Items[0].Quantity, dep.Items[1].Quantity)
	}

	// 3. Add third identical +0 sword (wea-01) -> fills depot (3/3)
	swordC, _ := item.NewInstanceWithEnhancement("wea-01", 1, 0)
	if err := dep.AddItem(swordC); err != nil {
		t.Fatalf("failed to add third +0 sword: %v", err)
	}
	if len(dep.Items) != 3 {
		t.Fatalf("expected 3 distinct slots, got %d", len(dep.Items))
	}

	// 4. Add fourth identical +0 sword at capacity (3/3) -> MUST fail with ErrDepotFull
	swordD, _ := item.NewInstanceWithEnhancement("wea-01", 1, 0)
	if err := dep.AddItem(swordD); !errors.Is(err, ErrDepotFull) {
		t.Fatalf("expected ErrDepotFull for non-stackable equipment at full capacity, got %v", err)
	}

	// 5. Test with default catalog fallback (without explicit ItemDefs provider set)
	depDefault, _ := NewDepot("char-default-cat")
	depDefault.Capacity = 3

	// "weapon-01" is in embedded default catalog (weapons.json)
	defSword1, _ := item.NewInstance("weapon-01", 1)
	defSword2, _ := item.NewInstance("weapon-01", 1)

	if err := depDefault.AddItem(defSword1); err != nil {
		t.Fatalf("failed to add first default catalog sword: %v", err)
	}
	if err := depDefault.AddItem(defSword2); err != nil {
		t.Fatalf("failed to add second default catalog sword: %v", err)
	}
	if len(depDefault.Items) != 2 {
		t.Fatalf("expected 2 distinct slots for default catalog weapons, got %d", len(depDefault.Items))
	}

	// Consumables ("item-001") in default catalog MUST merge
	herb1, _ := item.NewInstance("item-001", 2)
	herb2, _ := item.NewInstance("item-001", 3)
	if err := depDefault.AddItem(herb1); err != nil {
		t.Fatalf("failed to add first herb: %v", err)
	}
	if len(depDefault.Items) != 3 {
		t.Fatalf("expected 3 slots (2 swords + 1 herb), got %d", len(depDefault.Items))
	}
	// Depot is now at full capacity (3/3). Adding more herbs should merge into slot 2!
	if err := depDefault.AddItem(herb2); err != nil {
		t.Fatalf("expected stacking herb to succeed at full capacity, got %v", err)
	}
	if depDefault.Items[2].Quantity != 5 {
		t.Errorf("expected herb quantity to be 5, got %d", depDefault.Items[2].Quantity)
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

func TestDepositAndWithdrawItem_EnhancedEquipment(t *testing.T) {
	ctx := context.Background()
	depotRepo := newMemoryDepotRepo()
	charRepo := newMemoryCharRepo()
	invRepo := newMemoryInvRepo()
	catalog := newMemoryItemCatalog()

	char, _ := corecharacter.New("Blacksmith Customer")
	charRepo.characters[char.ID] = char

	inv, _ := coreinventory.New(char.ID)
	sword0, _ := item.NewInstanceWithEnhancement("wea-01", 1, 0)
	sword7, _ := item.NewInstanceWithEnhancement("wea-01", 1, 7)
	_ = inv.Add(sword0)
	_ = inv.Add(sword7)
	invRepo.inventories[char.ID] = inv

	service, _ := NewService(
		depotRepo, charRepo, invRepo,
		WithItemDefinitionProvider(catalog),
	)

	// Deposit +0 sword
	dep, err := service.DepositItem(ctx, char.ID, sword0.ID)
	if err != nil {
		t.Fatalf("DepositItem sword0 error: %v", err)
	}
	if len(dep.Items) != 1 || dep.Items[0].EnhancementLevel != 0 {
		t.Fatalf("unexpected depot state after sword0: %+v", dep.Items)
	}

	// Deposit +7 sword
	dep, err = service.DepositItem(ctx, char.ID, sword7.ID)
	if err != nil {
		t.Fatalf("DepositItem sword7 error: %v", err)
	}
	// Verify depot now has 2 distinct slots preserving enhancement levels
	if len(dep.Items) != 2 {
		t.Fatalf("expected 2 distinct slots in depot, got %d", len(dep.Items))
	}
	if dep.Items[0].EnhancementLevel != 0 || dep.Items[0].Quantity != 1 {
		t.Errorf("slot 0 corrupted: %+v", dep.Items[0])
	}
	if dep.Items[1].EnhancementLevel != 7 || dep.Items[1].Quantity != 1 {
		t.Errorf("slot 1 corrupted: %+v", dep.Items[1])
	}

	// Withdraw the +7 sword
	dep, err = service.WithdrawItem(ctx, char.ID, sword7.ID)
	if err != nil {
		t.Fatalf("WithdrawItem sword7 error: %v", err)
	}
	if len(dep.Items) != 1 || dep.Items[0].EnhancementLevel != 0 {
		t.Fatalf("expected 1 item with +0 remaining in depot, got %+v", dep.Items)
	}

	// Verify inventory now has the +7 sword restored
	currentInv, _ := invRepo.FindByCharacterID(ctx, char.ID)
	withdrawnInst, found := currentInv.Find(sword7.ID)
	if !found {
		t.Fatalf("expected sword7 in inventory, not found")
	}
	if withdrawnInst.EnhancementLevel != 7 {
		t.Errorf("expected withdrawn sword to have enhancement level 7, got %d", withdrawnInst.EnhancementLevel)
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

func TestDepot_ConsumeOne(t *testing.T) {
	d, err := NewDepot("char-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	d.Items = []item.Instance{
		{ID: "inst-1", DefinitionID: "itm-herb", Quantity: 3},
		{ID: "inst-2", DefinitionID: "wea-sword", Quantity: 1},
	}

	// Consume 1 from stacking item (3 -> 2)
	consumed, err := d.ConsumeOne("inst-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if consumed.Quantity != 1 || consumed.DefinitionID != "itm-herb" {
		t.Errorf("expected consumed quantity 1, got %d", consumed.Quantity)
	}
	if d.Items[0].Quantity != 2 {
		t.Errorf("expected remaining quantity 2, got %d", d.Items[0].Quantity)
	}

	// Consume remaining 1 from non-stacking item (removes item)
	consumed, err = d.ConsumeOne("inst-2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if consumed.ID != "inst-2" {
		t.Errorf("expected inst-2, got %s", consumed.ID)
	}
	if len(d.Items) != 1 {
		t.Errorf("expected 1 item left in depot, got %d", len(d.Items))
	}

	// Consume non-existent item
	_, err = d.ConsumeOne("missing")
	if !errors.Is(err, ErrItemNotFound) {
		t.Errorf("expected ErrItemNotFound, got %v", err)
	}
}
