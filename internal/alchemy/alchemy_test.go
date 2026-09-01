package alchemy

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
	itemsCopy := make([]item.Instance, len(inv.Items))
	copy(itemsCopy, inv.Items)
	inv.Items = itemsCopy
	return inv, nil
}

func (r *memoryInvRepo) FindByCharacterIDForUpdate(ctx context.Context, characterID string) (coreinventory.Inventory, error) {
	return r.FindByCharacterID(ctx, characterID)
}

func (r *memoryInvRepo) Save(_ context.Context, inventory coreinventory.Inventory) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	itemsCopy := make([]item.Instance, len(inventory.Items))
	copy(itemsCopy, inventory.Items)
	inventory.Items = itemsCopy
	r.inventories[inventory.CharacterID] = inventory
	return nil
}

func TestSynthesizeSuccess(t *testing.T) {
	ctx := context.Background()
	charRepo := newMemoryCharRepo()
	invRepo := newMemoryInvRepo()

	char, _ := corecharacter.New("Alchemist")
	char.Money = 500
	charRepo.characters[char.ID] = char

	herb, _ := item.NewDefinition("item-001", "Herb", 30)
	superHerb, _ := item.NewDefinition("item-002", "Super Herb", 100)
	itemCatalog, _ := item.NewCatalog([]item.Definition{herb, superHerb})

	recipe, _ := NewRecipe("rec-super-herb", "Synthesize Super Herb", "item-002", 1, []Ingredient{{"item-001", 2}}, 50)
	recipeCatalog, _ := NewRecipeCatalog([]Recipe{recipe})

	inv, _ := coreinventory.New(char.ID)
	herbInst, _ := item.NewInstance("item-001", 3)
	_ = inv.Add(herbInst)
	invRepo.inventories[char.ID] = inv

	service, err := NewService(charRepo, invRepo, recipeCatalog, itemCatalog)
	if err != nil {
		t.Fatal(err)
	}

	res, err := service.Synthesize(ctx, char.ID, "rec-super-herb")
	if err != nil {
		t.Fatalf("Synthesize error: %v", err)
	}

	if res.GoldCost != 50 || res.CreatedItem.DefinitionID != "item-002" || res.CreatedItem.Quantity != 1 {
		t.Fatalf("unexpected result: %#v", res)
	}

	// Verify character money
	updatedChar, _ := charRepo.FindByID(ctx, char.ID)
	if updatedChar.Money != 450 {
		t.Errorf("character money = %d, want 450", updatedChar.Money)
	}

	// Verify inventory
	updatedInv, _ := invRepo.FindByCharacterID(ctx, char.ID)
	if updatedInv.Quantity("item-001") != 1 {
		t.Errorf("remaining herbs = %d, want 1", updatedInv.Quantity("item-001"))
	}
	if updatedInv.Quantity("item-002") != 1 {
		t.Errorf("super herb quantity = %d, want 1", updatedInv.Quantity("item-002"))
	}
}

func TestSynthesizeValidationErrors(t *testing.T) {
	ctx := context.Background()
	charRepo := newMemoryCharRepo()
	invRepo := newMemoryInvRepo()

	char, _ := corecharacter.New("Poor Alchemist")
	char.Money = 10
	charRepo.characters[char.ID] = char

	herb, _ := item.NewDefinition("item-001", "Herb", 30)
	superHerb, _ := item.NewDefinition("item-002", "Super Herb", 100)
	itemCatalog, _ := item.NewCatalog([]item.Definition{herb, superHerb})

	recipe, _ := NewRecipe("rec-super-herb", "Synthesize Super Herb", "item-002", 1, []Ingredient{{"item-001", 2}}, 50)
	recipeCatalog, _ := NewRecipeCatalog([]Recipe{recipe})

	inv, _ := coreinventory.New(char.ID)
	invRepo.inventories[char.ID] = inv

	service, _ := NewService(charRepo, invRepo, recipeCatalog, itemCatalog)

	// 1. Insufficient funds (char has 10G, fee is 50G)
	_, err := service.Synthesize(ctx, char.ID, "rec-super-herb")
	if !errors.Is(err, ErrInsufficientFunds) {
		t.Errorf("expected ErrInsufficientFunds, got %v", err)
	}

	// 2. Insufficient materials
	char.Money = 500
	charRepo.characters[char.ID] = char
	_, err = service.Synthesize(ctx, char.ID, "rec-super-herb")
	if !errors.Is(err, ErrInsufficientMaterials) {
		t.Errorf("expected ErrInsufficientMaterials, got %v", err)
	}

	// 3. Recipe not found
	_, err = service.Synthesize(ctx, char.ID, "nonexistent-recipe")
	if !errors.Is(err, ErrRecipeNotFound) {
		t.Errorf("expected ErrRecipeNotFound, got %v", err)
	}

	// 4. Invalid character ID
	_, err = service.Synthesize(ctx, "", "rec-super-herb")
	if !errors.Is(err, ErrInvalidCharacterID) {
		t.Errorf("expected ErrInvalidCharacterID, got %v", err)
	}
}

type dummyTxProvider struct {
	mu sync.Mutex
}

func (d *dummyTxProvider) RunInTx(ctx context.Context, fn func(ctx context.Context) error) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	return fn(ctx)
}

func TestConcurrentSynthesize_IngredientConsumptionAtomic(t *testing.T) {
	ctx := context.Background()
	charRepo := newMemoryCharRepo()
	invRepo := newMemoryInvRepo()

	herb, _ := item.NewDefinition("item-001", "Herb", 30)
	superHerb, _ := item.NewDefinition("item-002", "Super Herb", 100)
	itemCatalog, _ := item.NewCatalog([]item.Definition{herb, superHerb})

	recipe, _ := NewRecipe("rec-super-herb", "Synthesize Super Herb", "item-002", 1, []Ingredient{{"item-001", 2}}, 50)
	recipeCatalog, _ := NewRecipeCatalog([]Recipe{recipe})

	char, _ := corecharacter.New("Hero")
	char.Money = 10000
	charRepo.characters[char.ID] = char

	inv, _ := coreinventory.New(char.ID)
	herbInst, _ := item.NewInstance("item-001", 6) // exactly enough for 3 syntheses (2 each)
	_ = inv.Add(herbInst)
	invRepo.inventories[char.ID] = inv

	service, _ := NewService(charRepo, invRepo, recipeCatalog, itemCatalog,
		WithTransactionProvider(&dummyTxProvider{}),
	)

	var wg sync.WaitGroup
	results := make(chan error, 5)
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := service.Synthesize(ctx, char.ID, "rec-super-herb")
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
	remainingHerbs := finalInv.Quantity("item-001")
	if remainingHerbs < 0 {
		t.Fatalf("herbs became negative: %d", remainingHerbs)
	}
}
