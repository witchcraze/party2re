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

func (r *memoryInvRepo) Save(_ context.Context, inventory coreinventory.Inventory) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.inventories[inventory.CharacterID] = inventory
	return nil
}

func TestNewDepot(t *testing.T) {
	dep, err := NewDepot("char-123")
	if err != nil {
		t.Fatalf("NewDepot error: %v", err)
	}
	if dep.CharacterID != "char-123" || dep.Capacity != DefaultDepotCapacity || dep.Gold != 0 || len(dep.Items) != 0 {
		t.Fatalf("unexpected depot: %#v", dep)
	}

	_, err = NewDepot("")
	if !errors.Is(err, ErrInvalidCharacterID) {
		t.Fatalf("expected ErrInvalidCharacterID, got %v", err)
	}
}

func TestDepotAddItemAndRemoveItem(t *testing.T) {
	dep, _ := NewDepot("char-123")
	item1, _ := item.NewInstance("potion", 2)
	item2, _ := item.NewInstance("potion", 3)
	item3, _ := item.NewInstance("sword", 1)

	if err := dep.AddItem(item1); err != nil {
		t.Fatalf("AddItem error: %v", err)
	}
	if len(dep.Items) != 1 || dep.Items[0].Quantity != 2 {
		t.Fatalf("unexpected items: %#v", dep.Items)
	}

	// Stacking item of same definition
	if err := dep.AddItem(item2); err != nil {
		t.Fatalf("AddItem error: %v", err)
	}
	if len(dep.Items) != 1 || dep.Items[0].Quantity != 5 {
		t.Fatalf("unexpected items: %#v", dep.Items)
	}

	// Adding different item
	if err := dep.AddItem(item3); err != nil {
		t.Fatalf("AddItem error: %v", err)
	}
	if len(dep.Items) != 2 {
		t.Fatalf("unexpected items length: %d", len(dep.Items))
	}

	// Remove item
	removed, err := dep.RemoveItem(item1.ID)
	if err != nil {
		t.Fatalf("RemoveItem error: %v", err)
	}
	if removed.DefinitionID != "potion" || len(dep.Items) != 1 {
		t.Fatalf("unexpected remove result: %#v, items: %#v", removed, dep.Items)
	}

	// Remove non-existent item
	_, err = dep.RemoveItem("nonexistent")
	if !errors.Is(err, ErrItemNotFound) {
		t.Fatalf("expected ErrItemNotFound, got %v", err)
	}
}

func TestDepotCapacityLimit(t *testing.T) {
	dep, _ := NewDepot("char-123")
	dep.Capacity = 2

	item1, _ := item.NewInstance("item-1", 1)
	item2, _ := item.NewInstance("item-2", 1)
	item3, _ := item.NewInstance("item-3", 1)

	_ = dep.AddItem(item1)
	_ = dep.AddItem(item2)

	err := dep.AddItem(item3)
	if !errors.Is(err, ErrDepotFull) {
		t.Fatalf("expected ErrDepotFull, got %v", err)
	}
}

func TestDepositAndWithdrawGold(t *testing.T) {
	ctx := context.Background()
	depotRepo := newMemoryDepotRepo()
	charRepo := newMemoryCharRepo()
	invRepo := newMemoryInvRepo()

	char, _ := corecharacter.New("Trader")
	char.Money = 500
	charRepo.characters[char.ID] = char

	service, err := NewService(depotRepo, charRepo, invRepo)
	if err != nil {
		t.Fatal(err)
	}

	// Deposit 200 gold
	dep, err := service.DepositGold(ctx, char.ID, 200)
	if err != nil {
		t.Fatalf("DepositGold error: %v", err)
	}
	if dep.Gold != 200 {
		t.Errorf("expected depot gold = 200, got %d", dep.Gold)
	}
	updatedChar, _ := charRepo.FindByID(ctx, char.ID)
	if updatedChar.Money != 300 {
		t.Errorf("expected character money = 300, got %d", updatedChar.Money)
	}

	// Deposit too much gold
	_, err = service.DepositGold(ctx, char.ID, 400)
	if !errors.Is(err, ErrInsufficientFunds) {
		t.Errorf("expected ErrInsufficientFunds, got %v", err)
	}

	// Deposit invalid amount
	_, err = service.DepositGold(ctx, char.ID, 0)
	if !errors.Is(err, ErrInvalidAmount) {
		t.Errorf("expected ErrInvalidAmount, got %v", err)
	}

	// Withdraw 150 gold
	dep, err = service.WithdrawGold(ctx, char.ID, 150)
	if err != nil {
		t.Fatalf("WithdrawGold error: %v", err)
	}
	if dep.Gold != 50 {
		t.Errorf("expected depot gold = 50, got %d", dep.Gold)
	}
	updatedChar, _ = charRepo.FindByID(ctx, char.ID)
	if updatedChar.Money != 450 {
		t.Errorf("expected character money = 450, got %d", updatedChar.Money)
	}

	// Withdraw too much gold
	_, err = service.WithdrawGold(ctx, char.ID, 100)
	if !errors.Is(err, ErrInsufficientDepotGold) {
		t.Errorf("expected ErrInsufficientDepotGold, got %v", err)
	}
}

func TestDepositAndWithdrawItem(t *testing.T) {
	ctx := context.Background()
	depotRepo := newMemoryDepotRepo()
	charRepo := newMemoryCharRepo()
	invRepo := newMemoryInvRepo()

	char, _ := corecharacter.New("Item Collector")
	charRepo.characters[char.ID] = char

	inv, _ := coreinventory.New(char.ID)
	potion, _ := item.NewInstance("item-001", 3)
	sword, _ := item.NewInstance("weapon-01", 1)
	_ = inv.Add(potion)
	_ = inv.Add(sword)
	invRepo.inventories[char.ID] = inv

	service, _ := NewService(depotRepo, charRepo, invRepo)

	// Deposit potion
	dep, err := service.DepositItem(ctx, char.ID, potion.ID)
	if err != nil {
		t.Fatalf("DepositItem error: %v", err)
	}
	if len(dep.Items) != 1 || dep.Items[0].DefinitionID != "item-001" {
		t.Fatalf("unexpected depot items: %#v", dep.Items)
	}
	updatedInv, _ := invRepo.FindByCharacterID(ctx, char.ID)
	if len(updatedInv.Items) != 1 {
		t.Fatalf("expected inventory items count = 1, got %d", len(updatedInv.Items))
	}

	// Deposit non-existent item
	_, err = service.DepositItem(ctx, char.ID, "nonexistent")
	if !errors.Is(err, ErrItemNotFound) {
		t.Fatalf("expected ErrItemNotFound, got %v", err)
	}

	// Withdraw potion
	dep, err = service.WithdrawItem(ctx, char.ID, potion.ID)
	if err != nil {
		t.Fatalf("WithdrawItem error: %v", err)
	}
	if len(dep.Items) != 0 {
		t.Fatalf("expected depot items count = 0, got %d", len(dep.Items))
	}
	updatedInv, _ = invRepo.FindByCharacterID(ctx, char.ID)
	if len(updatedInv.Items) != 2 {
		t.Fatalf("expected inventory items count = 2, got %d", len(updatedInv.Items))
	}

	// Withdraw non-existent item
	_, err = service.WithdrawItem(ctx, char.ID, "nonexistent")
	if !errors.Is(err, ErrItemNotFound) {
		t.Fatalf("expected ErrItemNotFound, got %v", err)
	}
}

func TestGetDepot(t *testing.T) {
	ctx := context.Background()
	depotRepo := newMemoryDepotRepo()
	charRepo := newMemoryCharRepo()
	invRepo := newMemoryInvRepo()

	service, _ := NewService(depotRepo, charRepo, invRepo)

	// Get depot when none exists returns default empty depot
	dep, err := service.GetDepot(ctx, "char-new")
	if err != nil {
		t.Fatalf("GetDepot error: %v", err)
	}
	if dep.CharacterID != "char-new" || dep.Capacity != DefaultDepotCapacity || dep.Gold != 0 {
		t.Fatalf("unexpected depot: %#v", dep)
	}

	// Invalid character ID
	_, err = service.GetDepot(ctx, "")
	if !errors.Is(err, ErrInvalidCharacterID) {
		t.Fatalf("expected ErrInvalidCharacterID, got %v", err)
	}
}
