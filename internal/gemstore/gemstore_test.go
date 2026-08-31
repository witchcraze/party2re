package gemstore_test

import (
	"context"
	"errors"
	"testing"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/gemstore"
)

// -------------------------------------------------------------------
// Mock Repositories
// -------------------------------------------------------------------

type mockCharacterRepo struct {
	characters map[string]corecharacter.Character
	updateFn   func(ctx context.Context, char corecharacter.Character) error
}

func newMockCharacterRepo() *mockCharacterRepo {
	return &mockCharacterRepo{
		characters: make(map[string]corecharacter.Character),
	}
}

func (m *mockCharacterRepo) FindByID(ctx context.Context, id string) (corecharacter.Character, error) {
	c, ok := m.characters[id]
	if !ok {
		return corecharacter.Character{}, corecharacter.ErrNotFound
	}
	return c, nil
}

func (m *mockCharacterRepo) FindByIDForUpdate(ctx context.Context, id string) (corecharacter.Character, error) {
	return m.FindByID(ctx, id)
}

func (m *mockCharacterRepo) Update(ctx context.Context, character corecharacter.Character) error {
	if m.updateFn != nil {
		return m.updateFn(ctx, character)
	}
	m.characters[character.ID] = character
	return nil
}

type mockInventoryRepo struct {
	inventories map[string]coreinventory.Inventory
	saveFn      func(ctx context.Context, inv coreinventory.Inventory) error
}

func newMockInventoryRepo() *mockInventoryRepo {
	return &mockInventoryRepo{
		inventories: make(map[string]coreinventory.Inventory),
	}
}

func (m *mockInventoryRepo) FindByCharacterID(ctx context.Context, characterID string) (coreinventory.Inventory, error) {
	inv, ok := m.inventories[characterID]
	if !ok {
		inv, _ = coreinventory.New(characterID)
		m.inventories[characterID] = inv
	}
	return inv, nil
}

func (m *mockInventoryRepo) FindByCharacterIDForUpdate(ctx context.Context, characterID string) (coreinventory.Inventory, error) {
	return m.FindByCharacterID(ctx, characterID)
}

func (m *mockInventoryRepo) Save(ctx context.Context, inventory coreinventory.Inventory) error {
	if m.saveFn != nil {
		return m.saveFn(ctx, inventory)
	}
	m.inventories[inventory.CharacterID] = inventory
	return nil
}

type mockItemProvider struct {
	items map[string]coreitem.Definition
}

func (m *mockItemProvider) FindByID(id string) (coreitem.Definition, error) {
	d, ok := m.items[id]
	if !ok {
		return coreitem.Definition{}, errors.New("item not found")
	}
	return d, nil
}

// -------------------------------------------------------------------
// Unit Tests
// -------------------------------------------------------------------

func TestGemStore_BuyGem(t *testing.T) {
	catalog, err := gemstore.DefaultCatalog()
	if err != nil {
		t.Fatalf("failed to load default catalog: %v", err)
	}

	charRepo := newMockCharacterRepo()
	invRepo := newMockInventoryRepo()

	char := corecharacter.Character{
		ID:       "char_1",
		PlayerID: "player_1",
		Name:     "Hero",
		Level:    10,
		Money:    5000,
	}
	charRepo.characters[char.ID] = char

	svc, err := gemstore.NewService(catalog, charRepo, invRepo)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	ctx := context.Background()

	// 1. Success purchase Lv1 gem (price 60 * 5 = 300)
	res, err := svc.BuyGem(ctx, "char_1", "gem_atk_1")
	if err != nil {
		t.Fatalf("expected BuyGem success, got: %v", err)
	}
	if res.Cost != 300 {
		t.Errorf("expected cost 300, got %d", res.Cost)
	}
	if res.Character.Money != 4700 {
		t.Errorf("expected money 4700, got %d", res.Character.Money)
	}
	if len(res.Inventory.Items) != 1 {
		t.Errorf("expected 1 inventory item, got %d", len(res.Inventory.Items))
	}

	// 2. Failure: Level too low for Lv50 gem (gem_atk_2)
	_, err = svc.BuyGem(ctx, "char_1", "gem_atk_2")
	if err != gemstore.ErrLevelTooLow {
		t.Errorf("expected ErrLevelTooLow, got: %v", err)
	}

	// 3. Failure: Insufficient funds
	poorChar := corecharacter.Character{
		ID:       "char_poor",
		PlayerID: "player_2",
		Name:     "PoorHero",
		Level:    10,
		Money:    50,
	}
	charRepo.characters[poorChar.ID] = poorChar
	_, err = svc.BuyGem(ctx, "char_poor", "gem_atk_1")
	if err != gemstore.ErrInsufficientFunds {
		t.Errorf("expected ErrInsufficientFunds, got: %v", err)
	}

	// 4. Failure: Gem not found
	_, err = svc.BuyGem(ctx, "char_1", "gem_non_existent")
	if err != gemstore.ErrGemNotFound {
		t.Errorf("expected ErrGemNotFound, got: %v", err)
	}
}

func TestGemStore_SellGem(t *testing.T) {
	catalog, err := gemstore.DefaultCatalog()
	if err != nil {
		t.Fatalf("failed to load catalog: %v", err)
	}

	charRepo := newMockCharacterRepo()
	invRepo := newMockInventoryRepo()

	char := corecharacter.Character{
		ID:       "char_1",
		PlayerID: "player_1",
		Name:     "Hero",
		Level:    10,
		Money:    100,
	}
	charRepo.characters[char.ID] = char

	inv, _ := coreinventory.New("char_1")
	inst, _ := coreitem.NewInstance("gem_atk_1", 1) // price 60 -> 50% = 30
	_ = inv.Add(inst)
	invRepo.inventories["char_1"] = inv

	svc, err := gemstore.NewService(catalog, charRepo, invRepo)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	ctx := context.Background()

	// 1. Success sell
	res, err := svc.SellGem(ctx, "char_1", inst.ID)
	if err != nil {
		t.Fatalf("expected SellGem success, got: %v", err)
	}
	if res.Payout != 30 {
		t.Errorf("expected payout 30, got %d", res.Payout)
	}
	if res.Character.Money != 130 {
		t.Errorf("expected money 130, got %d", res.Character.Money)
	}
	if len(res.Inventory.Items) != 0 {
		t.Errorf("expected 0 inventory items, got %d", len(res.Inventory.Items))
	}

	// 2. Failure: Item not in inventory
	_, err = svc.SellGem(ctx, "char_1", inst.ID)
	if err != gemstore.ErrItemNotOwned {
		t.Errorf("expected ErrItemNotOwned, got: %v", err)
	}
}

func TestGemStore_SendGem(t *testing.T) {
	catalog, err := gemstore.DefaultCatalog()
	if err != nil {
		t.Fatalf("failed to load catalog: %v", err)
	}

	charRepo := newMockCharacterRepo()
	invRepo := newMockInventoryRepo()

	char1 := corecharacter.Character{ID: "char_1", PlayerID: "p1", Name: "Sender"}
	char2 := corecharacter.Character{ID: "char_2", PlayerID: "p2", Name: "Recipient"}
	charRepo.characters[char1.ID] = char1
	charRepo.characters[char2.ID] = char2

	inv1, _ := coreinventory.New("char_1")
	inst, _ := coreitem.NewInstance("gem_heal_1", 1)
	_ = inv1.Add(inst)
	invRepo.inventories["char_1"] = inv1

	svc, err := gemstore.NewService(catalog, charRepo, invRepo)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	ctx := context.Background()

	// 1. Cannot send to self
	_, err = svc.SendGem(ctx, "char_1", "char_1", inst.ID)
	if err != gemstore.ErrCannotSendToSelf {
		t.Errorf("expected ErrCannotSendToSelf, got: %v", err)
	}

	// 2. Success send
	res, err := svc.SendGem(ctx, "char_1", "char_2", inst.ID)
	if err != nil {
		t.Fatalf("expected SendGem success, got: %v", err)
	}
	if res.Gem.ID != "gem_heal_1" {
		t.Errorf("expected sent gem_heal_1, got %s", res.Gem.ID)
	}

	// Verify inventory states
	senderInv, _ := invRepo.FindByCharacterID(ctx, "char_1")
	recipInv, _ := invRepo.FindByCharacterID(ctx, "char_2")
	if len(senderInv.Items) != 0 {
		t.Errorf("expected sender 0 items, got %d", len(senderInv.Items))
	}
	if len(recipInv.Items) != 1 {
		t.Errorf("expected recipient 1 item, got %d", len(recipInv.Items))
	}
}

func TestGemStore_SynthesizeGem(t *testing.T) {
	catalog, err := gemstore.DefaultCatalog()
	if err != nil {
		t.Fatalf("failed to load catalog: %v", err)
	}

	charRepo := newMockCharacterRepo()
	invRepo := newMockInventoryRepo()

	char := corecharacter.Character{ID: "char_1", PlayerID: "p1", Name: "Hero"}
	charRepo.characters[char.ID] = char

	// Inventory with 2 x gem_atk_1 ("攻撃の宝珠Ⅰ")
	inv, _ := coreinventory.New("char_1")
	inst1, _ := coreitem.NewInstance("gem_atk_1", 1)
	inst2, _ := coreitem.NewInstance("gem_atk_1", 1)
	_ = inv.Add(inst1)
	_ = inv.Add(inst2)
	invRepo.inventories["char_1"] = inv

	svc, err := gemstore.NewService(catalog, charRepo, invRepo)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	ctx := context.Background()

	// 1. Success synthesis: 攻撃の宝珠Ⅱ = 攻撃の宝珠Ⅰ × 攻撃の宝珠Ⅰ (recipe_atk_2)
	res, err := svc.SynthesizeGem(ctx, "char_1", "recipe_atk_2")
	if err != nil {
		t.Fatalf("expected SynthesizeGem success, got: %v", err)
	}
	if res.CreatedGem.ID != "gem_atk_2" {
		t.Errorf("expected created gem_atk_2, got %s", res.CreatedGem.ID)
	}
	if len(res.Inventory.Items) != 1 {
		t.Errorf("expected 1 item in inventory (the synthesized gem), got %d", len(res.Inventory.Items))
	}

	// 2. Failure: Insufficient materials (now only has gem_atk_2)
	_, err = svc.SynthesizeGem(ctx, "char_1", "recipe_atk_2")
	if err == nil {
		t.Error("expected error due to insufficient materials, got nil")
	}

	// 3. Failure: Invalid recipe ID
	_, err = svc.SynthesizeGem(ctx, "char_1", "invalid_recipe")
	if err != gemstore.ErrRecipeNotFound {
		t.Errorf("expected ErrRecipeNotFound, got: %v", err)
	}
}

func TestGemStore_AppraiseItem(t *testing.T) {
	catalog, err := gemstore.DefaultCatalog()
	if err != nil {
		t.Fatalf("failed to load catalog: %v", err)
	}

	charRepo := newMockCharacterRepo()
	invRepo := newMockInventoryRepo()

	char := corecharacter.Character{ID: "char_1", PlayerID: "p1", Name: "Hero"}
	charRepo.characters[char.ID] = char

	// Inventory with unidentified orb "光る宝珠" and normal sword
	itemProvider := &mockItemProvider{
		items: map[string]coreitem.Definition{
			"unidentified_glowing_orb": {ID: "unidentified_glowing_orb", Name: "光る宝珠"},
			"iron_sword":               {ID: "iron_sword", Name: "鉄の剣"},
		},
	}

	inv, _ := coreinventory.New("char_1")
	orbInst, _ := coreitem.NewInstance("unidentified_glowing_orb", 1)
	swordInst, _ := coreitem.NewInstance("iron_sword", 1)
	_ = inv.Add(orbInst)
	_ = inv.Add(swordInst)
	invRepo.inventories["char_1"] = inv

	svc, err := gemstore.NewService(
		catalog,
		charRepo,
		invRepo,
		gemstore.WithItemDefinitionProvider(itemProvider),
	)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	ctx := context.Background()

	// 1. Appraise unidentified orb -> converts to Gem
	resOrb, err := svc.AppraiseItem(ctx, "char_1", orbInst.ID)
	if err != nil {
		t.Fatalf("expected AppraiseItem success on orb, got: %v", err)
	}
	if !resOrb.IsGem {
		t.Errorf("expected IsGem to be true")
	}
	if resOrb.IdentifiedGem == nil || resOrb.IdentifiedGem.ID != "gem_atk_2" {
		t.Errorf("expected revealed gem_atk_2, got %+v", resOrb.IdentifiedGem)
	}

	// 2. Appraise regular item -> reveals name
	resSword, err := svc.AppraiseItem(ctx, "char_1", swordInst.ID)
	if err != nil {
		t.Fatalf("expected AppraiseItem success on sword, got: %v", err)
	}
	if resSword.IsGem {
		t.Errorf("expected IsGem to be false for sword")
	}
	if resSword.IdentifiedName != "鉄の剣" {
		t.Errorf("expected identified name 鉄の剣, got %s", resSword.IdentifiedName)
	}
}

func TestGemStore_GetCatalogAndRecipes(t *testing.T) {
	catalog, err := gemstore.DefaultCatalog()
	if err != nil {
		t.Fatalf("failed to load catalog: %v", err)
	}

	charRepo := newMockCharacterRepo()
	invRepo := newMockInventoryRepo()

	svc, err := gemstore.NewService(catalog, charRepo, invRepo)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	// Check catalog filtering by level
	gemsLv1 := svc.GetCatalog(1)
	gemsLv50 := svc.GetCatalog(50)
	gemsLv100 := svc.GetCatalog(100)

	if len(gemsLv1) >= len(gemsLv50) || len(gemsLv50) >= len(gemsLv100) {
		t.Errorf("expected increasing gem counts: lv1=%d, lv50=%d, lv100=%d",
			len(gemsLv1), len(gemsLv50), len(gemsLv100))
	}

	// Check recipes
	recipes := svc.GetRecipes()
	if len(recipes) < 50 {
		t.Errorf("expected at least 50 recipes, got %d", len(recipes))
	}

	// Check dialogue
	dialogues := svc.GetDialogue()
	if len(dialogues) == 0 {
		t.Error("expected non-empty dialogue list")
	}
}
