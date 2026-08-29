package blacksmith

import (
	"context"
	"errors"
	"sync"
	"testing"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	"github.com/witchcraze/party2re/internal/core/item"
)

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

type fixedRandSource struct {
	value float64
}

func (f fixedRandSource) Float64() float64 {
	return f.value
}

func TestCalculateCost(t *testing.T) {
	gold, mat := CalculateCost(0, 100)
	if gold != 50 || mat != 1 {
		t.Errorf("level 0 cost = (%d, %d), want (50, 1)", gold, mat)
	}

	gold, mat = CalculateCost(3, 200)
	if gold != 400 || mat != 2 {
		t.Errorf("level 3 cost = (%d, %d), want (400, 2)", gold, mat)
	}

	gold, mat = CalculateCost(9, 500)
	if gold != 2500 || mat != 4 {
		t.Errorf("level 9 cost = (%d, %d), want (2500, 4)", gold, mat)
	}
}

func TestCalculateSuccessRate(t *testing.T) {
	if rate := CalculateSuccessRate(0); rate != 1.0 {
		t.Errorf("level 0 rate = %f, want 1.0", rate)
	}
	if rate := CalculateSuccessRate(9); rate != 0.20 {
		t.Errorf("level 9 rate = %f, want 0.20", rate)
	}
	if rate := CalculateSuccessRate(10); rate != 0.0 {
		t.Errorf("level 10 rate = %f, want 0.0", rate)
	}
}

func TestCalculateStatsBonus(t *testing.T) {
	atk, def := CalculateStatsBonus(1, 20, 10)
	if atk != 4 || def != 3 {
		t.Errorf("level 1 bonus = (%d, %d), want (4, 3)", atk, def)
	}

	atk, def = CalculateStatsBonus(10, 50, 40)
	if atk != 70 || def != 60 {
		t.Errorf("level 10 bonus = (%d, %d), want (70, 60)", atk, def)
	}
}

func TestEnhanceSuccess(t *testing.T) {
	ctx := context.Background()
	charRepo := newMemoryCharRepo()
	invRepo := newMemoryInvRepo()

	char, _ := corecharacter.New("Blacksmith Client")
	char.Money = 1000
	charRepo.characters[char.ID] = char

	swordDef, _ := item.NewEquipmentDefinition("test_sword", "Test Sword", 100, item.SlotMainHand)
	matDef, _ := item.NewDefinition(DefaultMaterialDefinitionID, "Upgrade Stone", 50)
	catalog, _ := item.NewCatalog([]item.Definition{swordDef, matDef})

	inv, _ := coreinventory.New(char.ID)
	swordInst, _ := item.NewInstance("test_sword", 1)
	matInst, _ := item.NewInstance(DefaultMaterialDefinitionID, 5)
	_ = inv.Add(swordInst)
	_ = inv.Add(matInst)
	invRepo.inventories[char.ID] = inv

	// Fixed random roll to guarantee success (0.1 < 1.0)
	service, err := NewServiceWithTransaction(charRepo, invRepo, nil, catalog, fixedRandSource{value: 0.1})
	if err != nil {
		t.Fatal(err)
	}

	res, err := service.Enhance(ctx, char.ID, swordInst.ID)
	if err != nil {
		t.Fatalf("Enhance error: %v", err)
	}

	if !res.Success || res.PreviousLevel != 0 || res.NewLevel != 1 {
		t.Fatalf("unexpected result: %#v", res)
	}
	if res.GoldCost != 50 || res.MaterialCost != 1 {
		t.Errorf("costs = (%d, %d), want (50, 1)", res.GoldCost, res.MaterialCost)
	}

	// Verify character money
	updatedChar, _ := charRepo.FindByID(ctx, char.ID)
	if updatedChar.Money != 950 {
		t.Errorf("character money = %d, want 950", updatedChar.Money)
	}

	// Verify inventory
	updatedInv, _ := invRepo.FindByCharacterID(ctx, char.ID)
	if updatedInv.Quantity(DefaultMaterialDefinitionID) != 4 {
		t.Errorf("remaining materials = %d, want 4", updatedInv.Quantity(DefaultMaterialDefinitionID))
	}
	storedSword, found := updatedInv.Find(swordInst.ID)
	if !found || storedSword.EnhancementLevel != 1 {
		t.Errorf("sword enhancement level = %d, want 1", storedSword.EnhancementLevel)
	}
}

func TestEnhanceFailure(t *testing.T) {
	ctx := context.Background()
	charRepo := newMemoryCharRepo()
	invRepo := newMemoryInvRepo()

	char, _ := corecharacter.New("Unlucky Hero")
	char.Money = 1000
	charRepo.characters[char.ID] = char

	swordDef, _ := item.NewEquipmentDefinition("test_sword", "Test Sword", 100, item.SlotMainHand)
	matDef, _ := item.NewDefinition(DefaultMaterialDefinitionID, "Upgrade Stone", 50)
	catalog, _ := item.NewCatalog([]item.Definition{swordDef, matDef})

	inv, _ := coreinventory.New(char.ID)
	swordInst, _ := item.NewInstanceWithEnhancement("test_sword", 1, 4) // +4 has 70% rate
	matInst, _ := item.NewInstance(DefaultMaterialDefinitionID, 5)
	_ = inv.Add(swordInst)
	_ = inv.Add(matInst)
	invRepo.inventories[char.ID] = inv

	// Fixed random roll to guarantee failure (0.95 >= 0.70)
	service, _ := NewServiceWithTransaction(charRepo, invRepo, nil, catalog, fixedRandSource{value: 0.95})

	res, err := service.Enhance(ctx, char.ID, swordInst.ID)
	if err != nil {
		t.Fatalf("Enhance error: %v", err)
	}

	if res.Success || res.PreviousLevel != 4 || res.NewLevel != 4 {
		t.Fatalf("unexpected result on failure: %#v", res)
	}

	// Verify character money and materials still consumed
	updatedChar, _ := charRepo.FindByID(ctx, char.ID)
	if updatedChar.Money >= 1000 {
		t.Errorf("character money was not deducted: %d", updatedChar.Money)
	}
	updatedInv, _ := invRepo.FindByCharacterID(ctx, char.ID)
	if updatedInv.Quantity(DefaultMaterialDefinitionID) >= 5 {
		t.Errorf("materials were not consumed")
	}
}

func TestEnhanceValidationErrors(t *testing.T) {
	ctx := context.Background()
	charRepo := newMemoryCharRepo()
	invRepo := newMemoryInvRepo()

	char, _ := corecharacter.New("Validator")
	char.Money = 10
	charRepo.characters[char.ID] = char

	swordDef, _ := item.NewEquipmentDefinition("sword", "Sword", 100, item.SlotMainHand)
	consumableDef, _ := item.NewDefinition("herb", "Herb", 30)
	matDef, _ := item.NewDefinition(DefaultMaterialDefinitionID, "Upgrade Stone", 50)
	catalog, _ := item.NewCatalog([]item.Definition{swordDef, consumableDef, matDef})

	inv, _ := coreinventory.New(char.ID)
	swordInst, _ := item.NewInstance("sword", 1)
	maxSwordInst, _ := item.NewInstanceWithEnhancement("sword", 1, MaxEnhancementLevel)
	herbInst, _ := item.NewInstance("herb", 1)
	_ = inv.Add(swordInst)
	_ = inv.Add(maxSwordInst)
	_ = inv.Add(herbInst)
	invRepo.inventories[char.ID] = inv

	service, _ := NewService(charRepo, invRepo, catalog)

	// 1. Non-equipment
	_, err := service.Enhance(ctx, char.ID, herbInst.ID)
	if !errors.Is(err, ErrItemNotEquipment) {
		t.Errorf("expected ErrItemNotEquipment, got %v", err)
	}

	// 2. Max level reached
	_, err = service.Enhance(ctx, char.ID, maxSwordInst.ID)
	if !errors.Is(err, ErrMaxEnhancementReached) {
		t.Errorf("expected ErrMaxEnhancementReached, got %v", err)
	}

	// 3. Insufficient funds (char has 10G, cost is 50G)
	_, err = service.Enhance(ctx, char.ID, swordInst.ID)
	if !errors.Is(err, ErrInsufficientFunds) {
		t.Errorf("expected ErrInsufficientFunds, got %v", err)
	}

	// 4. Insufficient materials (char has funds but 0 materials)
	char.Money = 500
	charRepo.characters[char.ID] = char
	_, err = service.Enhance(ctx, char.ID, swordInst.ID)
	if !errors.Is(err, ErrInsufficientMaterials) {
		t.Errorf("expected ErrInsufficientMaterials, got %v", err)
	}

	// 5. Item not found
	_, err = service.Enhance(ctx, char.ID, "nonexistent")
	if !errors.Is(err, ErrItemNotFound) {
		t.Errorf("expected ErrItemNotFound, got %v", err)
	}
}

type dummyTxProvider struct{}

func (d dummyTxProvider) RunInTx(ctx context.Context, fn func(ctx context.Context) error) error {
	return fn(ctx)
}

func TestConcurrentEnhance_MaterialConsumptionAtomic(t *testing.T) {
	ctx := context.Background()
	charRepo := newMemoryCharRepo()
	invRepo := newMemoryInvRepo()

	swordDef, _ := item.NewEquipmentDefinition("test_sword", "Test Sword", 100, item.SlotMainHand)
	matDef, _ := item.NewDefinition(DefaultMaterialDefinitionID, "Upgrade Stone", 50)
	catalog, _ := item.NewCatalog([]item.Definition{swordDef, matDef})

	char, _ := corecharacter.New("Hero")
	char.Money = 10000
	charRepo.characters[char.ID] = char

	swordInst, _ := item.NewInstance("test_sword", 1)
	matInst, _ := item.NewInstance(DefaultMaterialDefinitionID, 5) // exactly 5 materials
	inv, _ := coreinventory.New(char.ID)
	_ = inv.Add(swordInst)
	_ = inv.Add(matInst)
	invRepo.inventories[char.ID] = inv

	service, _ := NewService(charRepo, invRepo, catalog,
		WithTransactionProvider(dummyTxProvider{}),
		WithRandomSource(fixedRandSource{value: 0.0}), // always succeeds
	)

	// Attempt multiple enhancements
	var wg sync.WaitGroup
	results := make(chan error, 10)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := service.Enhance(ctx, char.ID, swordInst.ID)
			results <- err
		}()
	}
	wg.Wait()
	close(results)

	var successCount, failCount int
	for err := range results {
		if err == nil {
			successCount++
		} else {
			failCount++
		}
	}

	finalInv, _ := invRepo.FindByCharacterID(ctx, char.ID)
	remainingMaterials := finalInv.Quantity(DefaultMaterialDefinitionID)
	if remainingMaterials < 0 {
		t.Fatalf("materials became negative: %d", remainingMaterials)
	}
}
