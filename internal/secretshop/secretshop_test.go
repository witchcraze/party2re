package secretshop_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/depot"
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

type mockDepotRepo struct {
	depots map[string]depot.Depot
}

func newMockDepotRepo() *mockDepotRepo {
	return &mockDepotRepo{
		depots: make(map[string]depot.Depot),
	}
}

func (m *mockDepotRepo) FindByCharacterIDForUpdate(ctx context.Context, characterID string) (depot.Depot, error) {
	d, ok := m.depots[characterID]
	if !ok {
		return depot.Depot{}, depot.ErrNotFound
	}
	return d, nil
}

func (m *mockDepotRepo) Save(ctx context.Context, value depot.Depot) error {
	m.depots[value.CharacterID] = value
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

func setupTest(t *testing.T, opts ...secretshop.Option) (*secretshop.Service, *mockCharacterRepo, *mockInventoryRepo, *secretshop.Catalog) {
	t.Helper()

	catalog, err := secretshop.LoadDefaultCatalog()
	if err != nil {
		t.Fatalf("failed to load default catalog: %v", err)
	}

	charRepo := newMockCharacterRepo()
	invRepo := newMockInventoryRepo()

	defaultOpts := []secretshop.Option{
		secretshop.WithTransactionProvider(&mockTxProvider{}),
	}
	defaultOpts = append(defaultOpts, opts...)

	svc, err := secretshop.NewService(
		charRepo,
		invRepo,
		catalog,
		defaultOpts...,
	)
	if err != nil {
		t.Fatalf("failed to create secret shop service: %v", err)
	}

	return svc, charRepo, invRepo, catalog
}

func createTestCharacter(id, name string, level, jobLevel, money int) corecharacter.Character {
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
		Level:    level,
		JobLevel: jobLevel,
		Money:    money,
	}
}

func TestCatalog(t *testing.T) {
	catalog, err := secretshop.LoadDefaultCatalog()
	if err != nil {
		t.Fatalf("LoadDefaultCatalog failed: %v", err)
	}

	items := catalog.Items()
	if len(items) != 8 {
		t.Fatalf("expected exactly 8 items, got %d", len(items))
	}

	// Verify legacy items and 3x prices
	expectedItems := []struct {
		id       string
		defID    string
		name     string
		expPrice int
	}{
		{"secret_item_herbal_root", "item-010", "薬草の根っこ", 750},
		{"secret_item_magic_mirror", "item-015", "魔法の鏡", 900},
		{"secret_item_ruby_of_protection", "item-080", "守りのルビー", 1500},
		{"secret_item_silver_harp", "item-078", "銀のたてごと", 4200},
		{"secret_item_staff_of_change", "item-043", "へんげの杖", 3000},
		{"secret_item_philosophers_enlightenment", "item-027", "賢者の悟り", 30000},
		{"secret_item_spirits_ward", "item-030", "精霊の守り", 15000},
		{"secret_item_counts_blood", "item-031", "伯爵の血", 15000},
	}

	for _, exp := range expectedItems {
		it, ok := catalog.FindByID(exp.id)
		if !ok {
			t.Fatalf("expected item %s not found in catalog", exp.id)
		}
		if it.ItemDefinitionID != exp.defID {
			t.Errorf("item %s: expected defID %s, got %s", exp.id, exp.defID, it.ItemDefinitionID)
		}
		if it.Name != exp.name {
			t.Errorf("item %s: expected name %s, got %s", exp.id, exp.name, it.Name)
		}
		if it.Price != exp.expPrice {
			t.Errorf("item %s: expected price %d, got %d", exp.id, exp.expPrice, it.Price)
		}

		byDef, ok := catalog.FindByDefinitionID(exp.defID)
		if !ok || byDef.ID != exp.id {
			t.Errorf("FindByDefinitionID(%s) failed to return %s", exp.defID, exp.id)
		}
	}

	// Fictional Dragon Quest items must be purged
	fictionalIDs := []string{
		"secret_item_philosopher_stone",
		"secret_item_sacred_dew",
		"secret_item_sacred_leaf",
		"secret_item_elven_elixir",
		"secret_item_prayer_ring",
		"secret_item_dark_rosary",
		"secret_item_life_ring",
		"secret_item_holy_water",
	}
	for _, fID := range fictionalIDs {
		if _, ok := catalog.FindByID(fID); ok {
			t.Errorf("fictional item %s must not exist in catalog", fID)
		}
	}
}

func TestCheckEligibility(t *testing.T) {
	tests := []struct {
		name     string
		level    int
		jobLevel int
		expected bool
	}{
		{"job level 0 (novice)", 50, 0, false},
		{"job level 6 (high level novice)", 99, 6, false},
		{"job level 7 minimum qualified", 1, 7, true},
		{"job level 8 qualified", 10, 8, true},
		{"job level 30 master", 50, 30, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := createTestCharacter("c1", "Hero", tt.level, tt.jobLevel, 1000)
			eligible := secretshop.CheckEligibility(c)
			if eligible != tt.expected {
				t.Errorf("expected eligibility %v for jobLevel=%d, got %v", tt.expected, tt.jobLevel, eligible)
			}
		})
	}
}

func TestGetShopStatus(t *testing.T) {
	svc, charRepo, _, _ := setupTest(t)
	ctx := context.Background()

	// Ineligible character (JobLevel < 7)
	ineligible := createTestCharacter("char-low", "Novice", 50, 6, 5000)
	_ = charRepo.Update(ctx, ineligible)

	_, err := svc.GetShopStatus(ctx, "char-low")
	if !errors.Is(err, secretshop.ErrAccessDenied) {
		t.Fatalf("expected ErrAccessDenied, got %v", err)
	}

	// Eligible character (JobLevel >= 7)
	eligible := createTestCharacter("char-high", "Veteran", 1, 7, 50000)
	_ = charRepo.Update(ctx, eligible)

	status, err := svc.GetShopStatus(ctx, "char-high")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !status.IsEligible || len(status.Items) != 8 {
		t.Fatalf("invalid status: %+v", status)
	}
	if status.NPCName != secretshop.NPCName || status.LocationName != secretshop.LocationName {
		t.Fatalf("mismatched NPC or location name: %+v", status)
	}
}

func TestHelperQuestFilter(t *testing.T) {
	filter := &mockHelperFilter{
		activeHelperItemIDs: []string{"item-010"}, // Exclude herbal root
	}

	svc, charRepo, _, _ := setupTest(t, secretshop.WithHelperFilter(filter))

	ctx := context.Background()
	eligible := createTestCharacter("char-high", "Veteran", 20, 7, 50000)
	_ = charRepo.Update(ctx, eligible)

	status, err := svc.GetShopStatus(ctx, "char-high")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, it := range status.Items {
		if it.ItemDefinitionID == "item-010" {
			t.Fatalf("item-010 should have been filtered out by helper quest filter")
		}
	}

	// Attempting to purchase filtered item returns ErrItemUnavailableInHelperQuest
	_, err = svc.PurchaseItem(ctx, "char-high", "secret_item_herbal_root", 1)
	if !errors.Is(err, secretshop.ErrItemUnavailableInHelperQuest) {
		t.Fatalf("expected ErrItemUnavailableInHelperQuest, got %v", err)
	}
}

func TestNPCInteractions(t *testing.T) {
	svc, charRepo, _, _ := setupTest(t)
	ctx := context.Background()

	eligible := createTestCharacter("char-high", "Veteran", 20, 7, 50000)
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

	// PuffPuff - Legacy parity: returns flavor message only, NO HEALING
	initialHP := eligible.Stats.HP
	initialMP := eligible.Stats.MP
	puffResult, err := svc.PuffPuff(ctx, "char-high")
	if err != nil {
		t.Fatalf("PuffPuff failed: %v", err)
	}
	expectedPuffMsg := secretshop.FormatPuffPuffMessage("Veteran")
	if puffResult.Message != expectedPuffMsg {
		t.Fatalf("unexpected puff message: got %q, want %q", puffResult.Message, expectedPuffMsg)
	}

	// Verify character state was NOT mutated (no HP/MP healing)
	charAfter, err := charRepo.FindByID(ctx, "char-high")
	if err != nil {
		t.Fatalf("FindByID failed: %v", err)
	}
	if charAfter.Stats.HP != initialHP || charAfter.Stats.MP != initialMP {
		t.Fatalf("puff-puff must not heal stats: HP %d->%d, MP %d->%d",
			initialHP, charAfter.Stats.HP, initialMP, charAfter.Stats.MP)
	}
}

func TestPurchaseItemDirectToInventory(t *testing.T) {
	depotRepo := newMockDepotRepo()
	svc, charRepo, invRepo, _ := setupTest(t, secretshop.WithDepotRepository(depotRepo))
	ctx := context.Background()

	eligible := createTestCharacter("char-high", "Veteran", 20, 7, 100000)
	_ = charRepo.Update(ctx, eligible)

	// Single purchase into empty inventory -> goes to inventory
	result, err := svc.PurchaseItem(ctx, "char-high", "secret_item_herbal_root", 1)
	if err != nil {
		t.Fatalf("PurchaseItem failed: %v", err)
	}

	if result.TotalPrice != 750 {
		t.Fatalf("expected total price 750, got %d", result.TotalPrice)
	}
	if result.RemainingGold != 100000-750 {
		t.Fatalf("expected remaining gold %d, got %d", 100000-750, result.RemainingGold)
	}
	if result.TransferredToDepot {
		t.Fatal("expected item to be in inventory, not depot")
	}
	expectedMsg := "薬草の根っこメェ〜。持ってけメェ〜"
	if result.NPCMessage != expectedMsg {
		t.Fatalf("expected NPCMessage %q, got %q", expectedMsg, result.NPCMessage)
	}

	// Verify inventory
	inv, _ := invRepo.FindByCharacterID(ctx, "char-high")
	if len(inv.Items) != 1 {
		t.Fatalf("expected 1 inventory item stack, got %d", len(inv.Items))
	}
	if inv.Items[0].DefinitionID != "item-010" || inv.Items[0].Quantity != 1 {
		t.Fatalf("expected 1 of item-010, got %+v", inv.Items[0])
	}
}

func TestPurchaseItemTransferToDepot(t *testing.T) {
	depotRepo := newMockDepotRepo()
	svc, charRepo, invRepo, _ := setupTest(t, secretshop.WithDepotRepository(depotRepo))
	ctx := context.Background()

	eligible := createTestCharacter("char-high", "Veteran", 20, 7, 100000)
	_ = charRepo.Update(ctx, eligible)

	// 1. Buy first item -> enters inventory
	_, err := svc.PurchaseItem(ctx, "char-high", "secret_item_herbal_root", 1)
	if err != nil {
		t.Fatalf("first PurchaseItem failed: %v", err)
	}

	// 2. Buy second item -> inventory consumable slot is occupied, transfers to depot
	res2, err := svc.PurchaseItem(ctx, "char-high", "secret_item_magic_mirror", 1)
	if err != nil {
		t.Fatalf("second PurchaseItem failed: %v", err)
	}

	if !res2.TransferredToDepot {
		t.Fatal("expected second item to be transferred to depot")
	}
	expectedMsg := "魔法の鏡はVeteranメェ〜の預かり所の方に投げましたメェ〜"
	if res2.NPCMessage != expectedMsg {
		t.Fatalf("expected NPCMessage %q, got %q", expectedMsg, res2.NPCMessage)
	}

	// Verify depot has the second item
	dep, err := depotRepo.FindByCharacterIDForUpdate(ctx, "char-high")
	if err != nil {
		t.Fatalf("FindByCharacterIDForUpdate failed: %v", err)
	}
	if len(dep.Items) != 1 {
		t.Fatalf("expected 1 depot item, got %d", len(dep.Items))
	}
	if dep.Items[0].DefinitionID != "item-015" {
		t.Fatalf("expected item-015 in depot, got %+v", dep.Items[0])
	}

	// Verify inventory still only has 1 item
	inv, _ := invRepo.FindByCharacterID(ctx, "char-high")
	if len(inv.Items) != 1 {
		t.Fatalf("expected 1 inventory item, got %d", len(inv.Items))
	}
}

func TestPurchaseItemQuantityGreaterThanOneTransfersToDepot(t *testing.T) {
	depotRepo := newMockDepotRepo()
	svc, charRepo, _, _ := setupTest(t, secretshop.WithDepotRepository(depotRepo))
	ctx := context.Background()

	eligible := createTestCharacter("char-high", "Veteran", 20, 7, 100000)
	_ = charRepo.Update(ctx, eligible)

	// Quantity = 2 -> always transferred to depot
	res, err := svc.PurchaseItem(ctx, "char-high", "secret_item_ruby_of_protection", 2)
	if err != nil {
		t.Fatalf("PurchaseItem failed: %v", err)
	}
	if !res.TransferredToDepot {
		t.Fatal("expected multi-quantity purchase to transfer to depot")
	}
	if res.TotalPrice != 1500*2 {
		t.Fatalf("expected total price %d, got %d", 1500*2, res.TotalPrice)
	}

	dep, err := depotRepo.FindByCharacterIDForUpdate(ctx, "char-high")
	if err != nil {
		t.Fatalf("depot lookup failed: %v", err)
	}
	if len(dep.Items) != 1 || dep.Items[0].Quantity != 2 {
		t.Fatalf("expected depot stack of 2, got %+v", dep.Items)
	}
}

func TestPurchaseItemDepotFull(t *testing.T) {
	depotRepo := newMockDepotRepo()
	cap := depot.CalculateCapacity(7, 0, 0)
	fullDepot, _ := depot.NewDepotWithCapacity("char-high", 7, 0, 0)
	for i := 0; i < cap; i++ {
		dummyItem, _ := coreitem.NewInstance(fmt.Sprintf("item-test-%03d", i), 1)
		_ = fullDepot.AddItem(dummyItem)
	}
	_ = depotRepo.Save(context.Background(), fullDepot)

	svc, charRepo, _, _ := setupTest(t, secretshop.WithDepotRepository(depotRepo))
	ctx := context.Background()

	eligible := createTestCharacter("char-high", "Veteran", 20, 7, 100000)
	_ = charRepo.Update(ctx, eligible)

	// Quantity 2 triggers depot route, but depot is full
	_, err := svc.PurchaseItem(ctx, "char-high", "secret_item_ruby_of_protection", 2)
	if !errors.Is(err, secretshop.ErrDepotFull) {
		t.Fatalf("expected ErrDepotFull, got %v", err)
	}
}

func TestPurchaseItemValidationErrors(t *testing.T) {
	svc, charRepo, _, _ := setupTest(t)
	ctx := context.Background()

	// Ineligible (JobLevel < 7)
	ineligible := createTestCharacter("char-low", "Novice", 99, 6, 100000)
	_ = charRepo.Update(ctx, ineligible)
	_, err := svc.PurchaseItem(ctx, "char-low", "secret_item_herbal_root", 1)
	if !errors.Is(err, secretshop.ErrAccessDenied) {
		t.Fatalf("expected ErrAccessDenied, got %v", err)
	}

	// Eligible (JobLevel >= 7) but insufficient funds
	broke := createTestCharacter("char-broke", "Veteran", 20, 7, 100)
	_ = charRepo.Update(ctx, broke)
	_, err = svc.PurchaseItem(ctx, "char-broke", "secret_item_herbal_root", 1)
	if !errors.Is(err, secretshop.ErrInsufficientFunds) {
		t.Fatalf("expected ErrInsufficientFunds, got %v", err)
	}

	// Invalid quantity
	eligible := createTestCharacter("char-high", "Veteran", 20, 7, 100000)
	_ = charRepo.Update(ctx, eligible)
	_, err = svc.PurchaseItem(ctx, "char-high", "secret_item_herbal_root", 0)
	if !errors.Is(err, secretshop.ErrInvalidQuantity) {
		t.Fatalf("expected ErrInvalidQuantity for 0, got %v", err)
	}
	_, err = svc.PurchaseItem(ctx, "char-high", "secret_item_herbal_root", 100)
	if !errors.Is(err, secretshop.ErrInvalidQuantity) {
		t.Fatalf("expected ErrInvalidQuantity for 100, got %v", err)
	}

	// Non-existent item
	_, err = svc.PurchaseItem(ctx, "char-high", "non_existent_item", 1)
	if !errors.Is(err, secretshop.ErrItemNotFound) {
		t.Fatalf("expected ErrItemNotFound, got %v", err)
	}
}
