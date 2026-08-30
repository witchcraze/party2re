package secretshop_test

import (
	"context"
	"errors"
	"testing"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	"github.com/witchcraze/party2re/internal/secretshop"
)

type mockCharacterRepo struct {
	chars map[string]corecharacter.Character
}

func newMockCharacterRepo() *mockCharacterRepo {
	return &mockCharacterRepo{
		chars: make(map[string]corecharacter.Character),
	}
}

func (m *mockCharacterRepo) FindByID(ctx context.Context, id string) (corecharacter.Character, error) {
	c, ok := m.chars[id]
	if !ok {
		return corecharacter.Character{}, errors.New("character not found")
	}
	return c, nil
}

func (m *mockCharacterRepo) FindByIDForUpdate(ctx context.Context, id string) (corecharacter.Character, error) {
	return m.FindByID(ctx, id)
}

func (m *mockCharacterRepo) Update(ctx context.Context, value corecharacter.Character) error {
	m.chars[value.ID] = value
	return nil
}

type mockInventoryRepo struct {
	invs map[string]coreinventory.Inventory
}

func newMockInventoryRepo() *mockInventoryRepo {
	return &mockInventoryRepo{
		invs: make(map[string]coreinventory.Inventory),
	}
}

func (m *mockInventoryRepo) FindByCharacterID(ctx context.Context, characterID string) (coreinventory.Inventory, error) {
	inv, ok := m.invs[characterID]
	if !ok {
		return coreinventory.New(characterID)
	}
	return inv, nil
}

func (m *mockInventoryRepo) FindByCharacterIDForUpdate(ctx context.Context, characterID string) (coreinventory.Inventory, error) {
	return m.FindByCharacterID(ctx, characterID)
}

func (m *mockInventoryRepo) Save(ctx context.Context, value coreinventory.Inventory) error {
	m.invs[value.CharacterID] = value
	return nil
}

type mockHelperFilter struct {
	activeHelperItemIDs []string
}

func (m *mockHelperFilter) GetActiveHelperItemIDs(ctx context.Context) ([]string, error) {
	return m.activeHelperItemIDs, nil
}

type mockTxProvider struct{}

func (m *mockTxProvider) RunInTx(ctx context.Context, fn func(ctx context.Context) error) error {
	return fn(ctx)
}

func setupTest(t *testing.T) (*secretshop.Service, *mockCharacterRepo, *mockInventoryRepo, *secretshop.Catalog) {
	t.Helper()

	catalog, err := secretshop.LoadDefaultCatalog()
	if err != nil {
		t.Fatalf("failed to load default catalog: %v", err)
	}

	charRepo := newMockCharacterRepo()
	invRepo := newMockInventoryRepo()

	svc, err := secretshop.NewService(
		charRepo,
		invRepo,
		catalog,
		secretshop.WithTransactionProvider(&mockTxProvider{}),
	)
	if err != nil {
		t.Fatalf("failed to create secret shop service: %v", err)
	}

	return svc, charRepo, invRepo, catalog
}

func createTestCharacter(id, name string, level, money int, rebirth int) corecharacter.Character {
	return corecharacter.Character{
		ID:       id,
		PlayerID: "player-1",
		Name:     name,
		Stats: corecharacter.Stats{
			MaxHP:   100,
			MaxMP:   50,
			HP:      50,
			MP:      20,
			Attack:  10,
			Defense: 10,
			Agility: 10,
		},
		Level:        level,
		Money:        money,
		RebirthCount: rebirth,
	}
}

func TestCatalog(t *testing.T) {
	catalog, err := secretshop.LoadDefaultCatalog()
	if err != nil {
		t.Fatalf("LoadDefaultCatalog failed: %v", err)
	}

	items := catalog.Items()
	if len(items) == 0 {
		t.Fatal("expected non-empty items")
	}

	item, ok := catalog.FindByID("secret_item_philosopher_stone")
	if !ok {
		t.Fatal("expected to find secret_item_philosopher_stone")
	}
	if item.Price <= 0 || item.Name == "" {
		t.Fatalf("invalid item attributes: %+v", item)
	}

	itemByDef, ok := catalog.FindByDefinitionID("item-004")
	if !ok {
		t.Fatal("expected to find item by definition ID item-004")
	}
	if itemByDef.ID != item.ID {
		t.Fatalf("expected ID %s, got %s", item.ID, itemByDef.ID)
	}
}

func TestCheckEligibility(t *testing.T) {
	tests := []struct {
		name     string
		level    int
		rebirth  int
		expected bool
	}{
		{"low level no rebirth", 5, 0, false},
		{"level 14 no rebirth", 14, 0, false},
		{"level 15 qualified", 15, 0, true},
		{"level 50 qualified", 50, 0, true},
		{"level 1 with rebirth qualified", 1, 1, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := createTestCharacter("c1", "Hero", tt.level, 1000, tt.rebirth)
			eligible := secretshop.CheckEligibility(c)
			if eligible != tt.expected {
				t.Errorf("expected eligibility %v, got %v", tt.expected, eligible)
			}
		})
	}
}

func TestGetShopStatus(t *testing.T) {
	svc, charRepo, _, _ := setupTest(t)
	ctx := context.Background()

	// Ineligible character
	ineligible := createTestCharacter("char-low", "Novice", 10, 5000, 0)
	_ = charRepo.Update(ctx, ineligible)

	_, err := svc.GetShopStatus(ctx, "char-low")
	if !errors.Is(err, secretshop.ErrAccessDenied) {
		t.Fatalf("expected ErrAccessDenied, got %v", err)
	}

	// Eligible character
	eligible := createTestCharacter("char-high", "Veteran", 20, 50000, 0)
	_ = charRepo.Update(ctx, eligible)

	status, err := svc.GetShopStatus(ctx, "char-high")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !status.IsEligible || len(status.Items) == 0 {
		t.Fatalf("invalid status: %+v", status)
	}
	if status.NPCName != secretshop.NPCName || status.LocationName != secretshop.LocationName {
		t.Fatalf("mismatched NPC or location name: %+v", status)
	}
}

func TestHelperQuestFilter(t *testing.T) {
	catalog, _ := secretshop.LoadDefaultCatalog()
	charRepo := newMockCharacterRepo()
	invRepo := newMockInventoryRepo()
	filter := &mockHelperFilter{
		activeHelperItemIDs: []string{"item-004"}, // Exclude philosopher stone
	}

	svc, err := secretshop.NewService(
		charRepo,
		invRepo,
		catalog,
		secretshop.WithHelperFilter(filter),
	)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	ctx := context.Background()
	eligible := createTestCharacter("char-high", "Veteran", 20, 50000, 0)
	_ = charRepo.Update(ctx, eligible)

	status, err := svc.GetShopStatus(ctx, "char-high")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, it := range status.Items {
		if it.ItemDefinitionID == "item-004" {
			t.Fatalf("item-004 should have been filtered out by helper quest filter")
		}
	}

	// Attempting to purchase filtered item returns ErrItemUnavailableInHelperQuest
	_, err = svc.PurchaseItem(ctx, "char-high", "secret_item_philosopher_stone", 1)
	if !errors.Is(err, secretshop.ErrItemUnavailableInHelperQuest) {
		t.Fatalf("expected ErrItemUnavailableInHelperQuest, got %v", err)
	}
}

func TestNPCInteractions(t *testing.T) {
	svc, charRepo, _, _ := setupTest(t)
	ctx := context.Background()

	eligible := createTestCharacter("char-high", "Veteran", 20, 50000, 0)
	_ = charRepo.Update(ctx, eligible)

	// Talk
	talkMsg, err := svc.Talk(ctx, "char-high")
	if err != nil {
		t.Fatalf("Talk failed: %v", err)
	}
	if talkMsg == "" {
		t.Fatal("expected non-empty talk message")
	}

	// Inspect
	inspectMsg, err := svc.Inspect(ctx, "char-high")
	if err != nil {
		t.Fatalf("Inspect failed: %v", err)
	}
	if inspectMsg != secretshop.InspectDialogue {
		t.Fatalf("unexpected inspect message: %s", inspectMsg)
	}

	// PuffPuff
	puffResult, err := svc.PuffPuff(ctx, "char-high")
	if err != nil {
		t.Fatalf("PuffPuff failed: %v", err)
	}
	if puffResult.Message != secretshop.PuffPuffDialogue {
		t.Fatalf("unexpected puff message: %s", puffResult.Message)
	}
	if puffResult.HPHealed != 10 || puffResult.MPHealed != 5 {
		t.Fatalf("expected healing (+10 HP, +5 MP), got HP: %d, MP: %d", puffResult.HPHealed, puffResult.MPHealed)
	}
}

func TestPurchaseItemSuccess(t *testing.T) {
	svc, charRepo, invRepo, _ := setupTest(t)
	ctx := context.Background()

	eligible := createTestCharacter("char-high", "Veteran", 20, 100000, 0)
	_ = charRepo.Update(ctx, eligible)

	result, err := svc.PurchaseItem(ctx, "char-high", "secret_item_philosopher_stone", 2)
	if err != nil {
		t.Fatalf("PurchaseItem failed: %v", err)
	}

	expectedTotal := 4500 * 2
	if result.TotalPrice != expectedTotal {
		t.Fatalf("expected total price %d, got %d", expectedTotal, result.TotalPrice)
	}
	if result.RemainingGold != 100000-expectedTotal {
		t.Fatalf("expected remaining gold %d, got %d", 100000-expectedTotal, result.RemainingGold)
	}
	if result.InventoryInstanceID == "" {
		t.Fatal("expected non-empty inventory instance ID")
	}

	// Verify inventory
	inv, _ := invRepo.FindByCharacterID(ctx, "char-high")
	if len(inv.Items) != 1 {
		t.Fatalf("expected 1 inventory item stack, got %d", len(inv.Items))
	}
	if inv.Items[0].DefinitionID != "item-004" || inv.Items[0].Quantity != 2 {
		t.Fatalf("expected 2 of item-004, got %+v", inv.Items[0])
	}
}

func TestPurchaseItemValidationErrors(t *testing.T) {
	svc, charRepo, _, _ := setupTest(t)
	ctx := context.Background()

	// Ineligible
	ineligible := createTestCharacter("char-low", "Novice", 5, 100000, 0)
	_ = charRepo.Update(ctx, ineligible)
	_, err := svc.PurchaseItem(ctx, "char-low", "secret_item_philosopher_stone", 1)
	if !errors.Is(err, secretshop.ErrAccessDenied) {
		t.Fatalf("expected ErrAccessDenied, got %v", err)
	}

	// Eligible but insufficient funds
	broke := createTestCharacter("char-broke", "Veteran", 20, 100, 0)
	_ = charRepo.Update(ctx, broke)
	_, err = svc.PurchaseItem(ctx, "char-broke", "secret_item_philosopher_stone", 1)
	if !errors.Is(err, secretshop.ErrInsufficientFunds) {
		t.Fatalf("expected ErrInsufficientFunds, got %v", err)
	}

	// Invalid quantity
	eligible := createTestCharacter("char-high", "Veteran", 20, 100000, 0)
	_ = charRepo.Update(ctx, eligible)
	_, err = svc.PurchaseItem(ctx, "char-high", "secret_item_philosopher_stone", 0)
	if !errors.Is(err, secretshop.ErrInvalidQuantity) {
		t.Fatalf("expected ErrInvalidQuantity for 0, got %v", err)
	}
	_, err = svc.PurchaseItem(ctx, "char-high", "secret_item_philosopher_stone", 100)
	if !errors.Is(err, secretshop.ErrInvalidQuantity) {
		t.Fatalf("expected ErrInvalidQuantity for 100, got %v", err)
	}

	// Non-existent item
	_, err = svc.PurchaseItem(ctx, "char-high", "non_existent_item", 1)
	if !errors.Is(err, secretshop.ErrItemNotFound) {
		t.Fatalf("expected ErrItemNotFound, got %v", err)
	}
}
