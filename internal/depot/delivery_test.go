package depot_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	"github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/depot"
)

type mockInvRepo struct {
	inv     coreinventory.Inventory
	findErr error
	saveErr error
	saved   bool
}

func (m *mockInvRepo) FindByCharacterID(ctx context.Context, characterID string) (coreinventory.Inventory, error) {
	return m.FindByCharacterIDForUpdate(ctx, characterID)
}

func (m *mockInvRepo) FindByCharacterIDForUpdate(ctx context.Context, characterID string) (coreinventory.Inventory, error) {
	if m.findErr != nil {
		return coreinventory.Inventory{}, m.findErr
	}
	return m.inv, nil
}

func (m *mockInvRepo) Save(ctx context.Context, inv coreinventory.Inventory) error {
	if m.saveErr != nil {
		return m.saveErr
	}
	m.inv = inv
	m.saved = true
	return nil
}

type mockDepotRepo struct {
	depot   depot.Depot
	findErr error
	saveErr error
	saved   bool
}

func (m *mockDepotRepo) FindByCharacterID(ctx context.Context, characterID string) (depot.Depot, error) {
	return m.FindByCharacterIDForUpdate(ctx, characterID)
}

func (m *mockDepotRepo) FindByCharacterIDForUpdate(ctx context.Context, characterID string) (depot.Depot, error) {
	if m.findErr != nil {
		return depot.Depot{}, m.findErr
	}
	return m.depot, nil
}

func (m *mockDepotRepo) Save(ctx context.Context, dep depot.Depot) error {
	if m.saveErr != nil {
		return m.saveErr
	}
	m.depot = dep
	m.saved = true
	return nil
}

func TestDeliverRewardItem_EmptyInventory(t *testing.T) {
	ctx := context.Background()
	charID := "char-1"
	char := corecharacter.Character{ID: charID, JobLevel: 5}

	inv, err := coreinventory.New(charID)
	if err != nil {
		t.Fatalf("failed to create inventory: %v", err)
	}
	invRepo := &mockInvRepo{inv: inv}
	depotRepo := &mockDepotRepo{findErr: depot.ErrNotFound}

	res, err := depot.DeliverRewardItem(ctx, invRepo, depotRepo, char, "item-herb", 1, depot.PolicyAbortOnDepotFull)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.DeliveredTo != depot.DeliveredToInventory {
		t.Errorf("expected delivered to inventory, got %s", res.DeliveredTo)
	}
	if res.Item.DefinitionID != "item-herb" || res.Item.Quantity != 1 {
		t.Errorf("unexpected item instance: %+v", res.Item)
	}
	if !invRepo.saved {
		t.Error("expected invRepo.Save to have been called")
	}
	if len(invRepo.inv.Items) != 1 {
		t.Errorf("expected 1 item in inventory, got %d", len(invRepo.inv.Items))
	}
	if depotRepo.saved {
		t.Error("expected depotRepo.Save NOT to have been called")
	}
}

func TestDeliverRewardItem_FullInventory_DepotOverflow_WithRefreshedCapacity(t *testing.T) {
	ctx := context.Background()
	charID := "char-1"
	// Character leveled up to JobLevel 10 (base capacity 55)
	char := corecharacter.Character{ID: charID, JobLevel: 10}

	// Inventory is already full (DefaultMaxCapacity = 1)
	inv, _ := coreinventory.New(charID)
	existingInst, _ := item.NewInstance("item-sword", 1)
	_ = inv.Add(existingInst)
	invRepo := &mockInvRepo{inv: inv}

	// Existing depot has stale capacity 5 (from JobLevel 0), and already holds 5 items.
	// Without RefreshCapacity, depot would be considered full and reject the item!
	existingDepot, _ := depot.NewDepotWithCapacity(charID, 0, 0, 0)
	for i := 0; i < 5; i++ {
		staleItem, _ := item.NewInstance(fmt.Sprintf("item-dummy-%d", i), 1)
		_ = existingDepot.AddItem(staleItem)
	}
	if len(existingDepot.Items) != 5 || existingDepot.Capacity != 5 {
		t.Fatalf("test setup failed: expected 5 items and capacity 5, got %d / %d", len(existingDepot.Items), existingDepot.Capacity)
	}

	depotRepo := &mockDepotRepo{depot: existingDepot}

	res, err := depot.DeliverRewardItem(ctx, invRepo, depotRepo, char, "item-potion", 1, depot.PolicyAbortOnDepotFull)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.DeliveredTo != depot.DeliveredToDepot {
		t.Errorf("expected delivered to depot, got %s", res.DeliveredTo)
	}
	if res.Item.DefinitionID != "item-potion" {
		t.Errorf("unexpected item definition ID: %s", res.Item.DefinitionID)
	}
	if !depotRepo.saved {
		t.Error("expected depotRepo.Save to have been called")
	}
	// Verify depot capacity was refreshed to 55 (jobLv 10 * 5 + 5)
	if depotRepo.depot.Capacity != 55 {
		t.Errorf("expected refreshed capacity 55, got %d", depotRepo.depot.Capacity)
	}
	if len(depotRepo.depot.Items) != 6 {
		t.Errorf("expected 6 items in depot, got %d", len(depotRepo.depot.Items))
	}
}

func TestDeliverRewardItem_FullDepot_PolicyTreatOverflowAsLost(t *testing.T) {
	ctx := context.Background()
	charID := "char-1"
	char := corecharacter.Character{ID: charID, JobLevel: 0} // cap = 5

	// Full inventory
	inv, _ := coreinventory.New(charID)
	existingInst, _ := item.NewInstance("item-shield", 1)
	_ = inv.Add(existingInst)
	invRepo := &mockInvRepo{inv: inv}

	// Truly full depot (5 items, capacity 5)
	fullDepot, _ := depot.NewDepotWithCapacity(charID, 0, 0, 0)
	for i := 0; i < 5; i++ {
		dummy, _ := item.NewInstance(fmt.Sprintf("item-dummy-%d", i), 1)
		_ = fullDepot.AddItem(dummy)
	}
	depotRepo := &mockDepotRepo{depot: fullDepot}

	res, err := depot.DeliverRewardItem(ctx, invRepo, depotRepo, char, "item-rare-gem", 1, depot.PolicyTreatOverflowAsLost)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.DeliveredTo != depot.DeliveredToLost {
		t.Errorf("expected delivered to lost, got %s", res.DeliveredTo)
	}
	if res.Item.DefinitionID != "item-rare-gem" {
		t.Errorf("unexpected item: %+v", res.Item)
	}
	// Depot items count should remain 5
	if len(depotRepo.depot.Items) != 5 {
		t.Errorf("expected 5 items in depot, got %d", len(depotRepo.depot.Items))
	}
}

func TestDeliverRewardItem_FullDepot_PolicyAbortOnDepotFull(t *testing.T) {
	ctx := context.Background()
	charID := "char-1"
	char := corecharacter.Character{ID: charID, JobLevel: 0} // cap = 5

	// Full inventory
	inv, _ := coreinventory.New(charID)
	existingInst, _ := item.NewInstance("item-shield", 1)
	_ = inv.Add(existingInst)
	invRepo := &mockInvRepo{inv: inv}

	// Truly full depot (5 items, capacity 5)
	fullDepot, _ := depot.NewDepotWithCapacity(charID, 0, 0, 0)
	for i := 0; i < 5; i++ {
		dummy, _ := item.NewInstance(fmt.Sprintf("item-dummy-%d", i), 1)
		_ = fullDepot.AddItem(dummy)
	}
	depotRepo := &mockDepotRepo{depot: fullDepot}

	_, err := depot.DeliverRewardItem(ctx, invRepo, depotRepo, char, "item-quest-reward", 1, depot.PolicyAbortOnDepotFull)
	if !errors.Is(err, depot.ErrDepotFull) {
		t.Fatalf("expected ErrDepotFull, got %v", err)
	}
}

func TestDeliverRewardItems_MultipleItems_MixedRouting(t *testing.T) {
	ctx := context.Background()
	charID := "char-1"
	char := corecharacter.Character{ID: charID, JobLevel: 0} // cap = 5

	// Empty inventory (cap 1)
	inv, _ := coreinventory.New(charID)
	invRepo := &mockInvRepo{inv: inv}

	// Depot with 4 items (capacity 5, space for only 1 more)
	d, _ := depot.NewDepotWithCapacity(charID, 0, 0, 0)
	for i := 0; i < 4; i++ {
		dummy, _ := item.NewInstance(fmt.Sprintf("item-dummy-%d", i), 1)
		_ = d.AddItem(dummy)
	}
	depotRepo := &mockDepotRepo{depot: d}

	// 3 items to deliver:
	// Item 1 -> Inventory (now full)
	// Item 2 -> Depot (now full: 5 items)
	// Item 3 -> Lost (depot full)
	inst1, _ := item.NewInstance("item-1", 1)
	inst2, _ := item.NewInstance("item-2", 1)
	inst3, _ := item.NewInstance("item-3", 1)

	results, err := depot.DeliverRewardItems(ctx, invRepo, depotRepo, char, []item.Instance{inst1, inst2, inst3}, depot.PolicyTreatOverflowAsLost)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	if results[0].DeliveredTo != depot.DeliveredToInventory || results[0].Item.DefinitionID != "item-1" {
		t.Errorf("result 0 mismatch: %+v", results[0])
	}
	if results[1].DeliveredTo != depot.DeliveredToDepot || results[1].Item.DefinitionID != "item-2" {
		t.Errorf("result 1 mismatch: %+v", results[1])
	}
	if results[2].DeliveredTo != depot.DeliveredToLost || results[2].Item.DefinitionID != "item-3" {
		t.Errorf("result 2 mismatch: %+v", results[2])
	}
}

func TestDeliverRewardItems_NilInvRepo_DirectToDepot(t *testing.T) {
	ctx := context.Background()
	charID := "char-1"
	char := corecharacter.Character{ID: charID, JobLevel: 2} // cap 15

	depotRepo := &mockDepotRepo{findErr: depot.ErrNotFound}

	inst, _ := item.NewInstance("item-direct", 1)
	results, err := depot.DeliverRewardItems(ctx, nil, depotRepo, char, []item.Instance{inst}, depot.PolicyAbortOnDepotFull)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(results) != 1 || results[0].DeliveredTo != depot.DeliveredToDepot {
		t.Fatalf("expected 1 depot delivery, got %+v", results)
	}
	if !depotRepo.saved {
		t.Error("expected depot to be saved")
	}
	if depotRepo.depot.Capacity != 15 {
		t.Errorf("expected depot capacity 15, got %d", depotRepo.depot.Capacity)
	}
}

func TestDeliverRewardItems_NilDepotRepo_PolicyTreatOverflowAsLost(t *testing.T) {
	ctx := context.Background()
	charID := "char-1"
	char := corecharacter.Character{ID: charID}

	// Full inventory
	inv, _ := coreinventory.New(charID)
	fullItem, _ := item.NewInstance("item-full", 1)
	_ = inv.Add(fullItem)
	invRepo := &mockInvRepo{inv: inv}

	inst, _ := item.NewInstance("item-overflow", 1)
	results, err := depot.DeliverRewardItems(ctx, invRepo, nil, char, []item.Instance{inst}, depot.PolicyTreatOverflowAsLost)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(results) != 1 || results[0].DeliveredTo != depot.DeliveredToLost {
		t.Fatalf("expected 1 lost delivery, got %+v", results)
	}
}

func TestDeliverRewardItems_EmptyList(t *testing.T) {
	ctx := context.Background()
	charID := "char-1"
	char := corecharacter.Character{ID: charID}

	results, err := depot.DeliverRewardItems(ctx, nil, nil, char, nil, depot.PolicyAbortOnDepotFull)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}
