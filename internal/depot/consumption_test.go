package depot_test

import (
	"context"
	"errors"
	"testing"

	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	"github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/depot"
)

type lockOrderRecorder struct {
	calls []string
}

type mockConsumptionInvRepo struct {
	inv      coreinventory.Inventory
	findErr  error
	saveErr  error
	savedInv *coreinventory.Inventory
	recorder *lockOrderRecorder
}

func (m *mockConsumptionInvRepo) FindByCharacterIDForUpdate(ctx context.Context, characterID string) (coreinventory.Inventory, error) {
	if m.recorder != nil {
		m.recorder.calls = append(m.recorder.calls, "inventory:FindByCharacterIDForUpdate")
	}
	if m.findErr != nil {
		return coreinventory.Inventory{}, m.findErr
	}
	return m.inv, nil
}

func (m *mockConsumptionInvRepo) Save(ctx context.Context, inv coreinventory.Inventory) error {
	if m.saveErr != nil {
		return m.saveErr
	}
	m.savedInv = &inv
	return nil
}

type mockConsumptionDepotRepo struct {
	dep      depot.Depot
	findErr  error
	saveErr  error
	savedDep *depot.Depot
	recorder *lockOrderRecorder
}

func (m *mockConsumptionDepotRepo) FindByCharacterIDForUpdate(ctx context.Context, characterID string) (depot.Depot, error) {
	if m.recorder != nil {
		m.recorder.calls = append(m.recorder.calls, "depot:FindByCharacterIDForUpdate")
	}
	if m.findErr != nil {
		return depot.Depot{}, m.findErr
	}
	return m.dep, nil
}

func (m *mockConsumptionDepotRepo) Save(ctx context.Context, value depot.Depot) error {
	if m.saveErr != nil {
		return m.saveErr
	}
	m.savedDep = &value
	return nil
}

func TestResolveItem(t *testing.T) {
	charID := "char-1"
	invItem := item.Instance{ID: "inv-item-1", DefinitionID: "herb", Quantity: 2}
	depItem := item.Instance{ID: "dep-item-1", DefinitionID: "herb", Quantity: 5}
	uniqueDepItem := item.Instance{ID: "dep-item-2", DefinitionID: "potion", Quantity: 1}

	inv := coreinventory.Inventory{
		CharacterID: charID,
		Items:       []item.Instance{invItem},
	}
	dp, err := depot.NewDepot(charID)
	if err != nil {
		t.Fatalf("NewDepot failed: %v", err)
	}
	dp.Items = []item.Instance{depItem, uniqueDepItem}

	t.Run("inventory first picks inventory when present in both", func(t *testing.T) {
		res, err := depot.ResolveItem(&inv, &dp, depot.QueryByDefinitionID("herb"), depot.PriorityInventoryFirst)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.Location != depot.LocationInventory {
			t.Errorf("expected LocationInventory, got %v", res.Location)
		}
		if res.Item.ID != "inv-item-1" {
			t.Errorf("expected inv-item-1, got %v", res.Item.ID)
		}
	})

	t.Run("depot first picks depot when present in both", func(t *testing.T) {
		res, err := depot.ResolveItem(&inv, &dp, depot.QueryByDefinitionID("herb"), depot.PriorityDepotFirst)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.Location != depot.LocationDepot {
			t.Errorf("expected LocationDepot, got %v", res.Location)
		}
		if res.Item.ID != "dep-item-1" {
			t.Errorf("expected dep-item-1, got %v", res.Item.ID)
		}
	})

	t.Run("inventory first falls back to depot", func(t *testing.T) {
		res, err := depot.ResolveItem(&inv, &dp, depot.QueryByDefinitionID("potion"), depot.PriorityInventoryFirst)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.Location != depot.LocationDepot {
			t.Errorf("expected LocationDepot, got %v", res.Location)
		}
		if res.Item.ID != "dep-item-2" {
			t.Errorf("expected dep-item-2, got %v", res.Item.ID)
		}
	})

	t.Run("depot first falls back to inventory", func(t *testing.T) {
		invOnly := coreinventory.Inventory{
			CharacterID: charID,
			Items:       []item.Instance{{ID: "inv-special", DefinitionID: "seed", Quantity: 1}},
		}
		res, err := depot.ResolveItem(&invOnly, &dp, depot.QueryByDefinitionID("seed"), depot.PriorityDepotFirst)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.Location != depot.LocationInventory {
			t.Errorf("expected LocationInventory, got %v", res.Location)
		}
		if res.Item.ID != "inv-special" {
			t.Errorf("expected inv-special, got %v", res.Item.ID)
		}
	})

	t.Run("find by instance ID", func(t *testing.T) {
		res, err := depot.ResolveItem(&inv, &dp, depot.QueryByInstanceID("dep-item-2"), depot.PriorityInventoryFirst)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.Location != depot.LocationDepot || res.Item.ID != "dep-item-2" {
			t.Errorf("expected dep-item-2 in depot, got %+v", res)
		}
	})

	t.Run("find by custom match predicate", func(t *testing.T) {
		res, err := depot.ResolveItem(&inv, &dp, depot.QueryByMatch(func(inst item.Instance) bool {
			return inst.Quantity == 5
		}), depot.PriorityInventoryFirst)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.Location != depot.LocationDepot || res.Item.ID != "dep-item-1" {
			t.Errorf("expected dep-item-1, got %+v", res)
		}
	})

	t.Run("item missing returns ErrItemNotFound", func(t *testing.T) {
		_, err := depot.ResolveItem(&inv, &dp, depot.QueryByDefinitionID("nonexistent"), depot.PriorityInventoryFirst)
		if !errors.Is(err, depot.ErrItemNotFound) {
			t.Errorf("expected ErrItemNotFound, got %v", err)
		}
	})

	t.Run("handles nil containers gracefully", func(t *testing.T) {
		_, err := depot.ResolveItem(nil, nil, depot.QueryByDefinitionID("herb"), depot.PriorityInventoryFirst)
		if !errors.Is(err, depot.ErrItemNotFound) {
			t.Errorf("expected ErrItemNotFound, got %v", err)
		}
	})
}

func TestConsumeItem(t *testing.T) {
	t.Run("invalid quantity returns ErrInvalidQuantity", func(t *testing.T) {
		inv := coreinventory.Inventory{Items: []item.Instance{{ID: "1", DefinitionID: "a", Quantity: 1}}}
		_, err := depot.ConsumeItem(&inv, nil, depot.QueryByDefinitionID("a"), depot.PriorityInventoryFirst, 0)
		if !errors.Is(err, depot.ErrInvalidQuantity) {
			t.Errorf("expected ErrInvalidQuantity, got %v", err)
		}
	})

	t.Run("consume partial quantity from inventory", func(t *testing.T) {
		inv := coreinventory.Inventory{
			Items: []item.Instance{{ID: "i1", DefinitionID: "a", Quantity: 3}},
		}
		res, err := depot.ConsumeItem(&inv, nil, depot.QueryByDefinitionID("a"), depot.PriorityInventoryFirst, 2)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.Location != depot.LocationInventory {
			t.Errorf("expected LocationInventory, got %v", res.Location)
		}
		if res.Item.Quantity != 2 {
			t.Errorf("expected consumed quantity 2, got %d", res.Item.Quantity)
		}
		if len(inv.Items) != 1 || inv.Items[0].Quantity != 1 {
			t.Errorf("expected 1 item remaining with qty 1, got %+v", inv.Items)
		}
	})

	t.Run("consume full quantity from depot removes slot", func(t *testing.T) {
		dp, _ := depot.NewDepot("c1")
		dp.Items = []item.Instance{{ID: "d1", DefinitionID: "b", Quantity: 1}}
		res, err := depot.ConsumeItem(nil, &dp, depot.QueryByDefinitionID("b"), depot.PriorityDepotFirst, 1)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.Location != depot.LocationDepot {
			t.Errorf("expected LocationDepot, got %v", res.Location)
		}
		if len(dp.Items) != 0 {
			t.Errorf("expected depot items to be empty, got %+v", dp.Items)
		}
	})

	t.Run("insufficient quantity in target returns ErrInvalidQuantity", func(t *testing.T) {
		inv := coreinventory.Inventory{
			Items: []item.Instance{{ID: "i1", DefinitionID: "a", Quantity: 1}},
		}
		_, err := depot.ConsumeItem(&inv, nil, depot.QueryByDefinitionID("a"), depot.PriorityInventoryFirst, 5)
		if !errors.Is(err, depot.ErrInvalidQuantity) {
			t.Errorf("expected ErrInvalidQuantity, got %v", err)
		}
	})
}

func TestSaveConsumptionResult(t *testing.T) {
	ctx := context.Background()
	invRepo := &mockConsumptionInvRepo{}
	depotRepo := &mockConsumptionDepotRepo{}

	inv := coreinventory.Inventory{CharacterID: "c1"}
	dp, _ := depot.NewDepot("c1")

	t.Run("saves inventory when consumed from inventory", func(t *testing.T) {
		res := depot.ConsumptionResult{Location: depot.LocationInventory}
		err := depot.SaveConsumptionResult(ctx, invRepo, depotRepo, res, inv, dp)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if invRepo.savedInv == nil {
			t.Error("expected inventory to be saved")
		}
		if depotRepo.savedDep != nil {
			t.Error("depot should not be saved")
		}
	})

	t.Run("saves depot when consumed from depot", func(t *testing.T) {
		invRepo.savedInv = nil
		depotRepo.savedDep = nil
		res := depot.ConsumptionResult{Location: depot.LocationDepot}
		err := depot.SaveConsumptionResult(ctx, invRepo, depotRepo, res, inv, dp)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if depotRepo.savedDep == nil {
			t.Error("expected depot to be saved")
		}
		if invRepo.savedInv != nil {
			t.Error("inventory should not be saved")
		}
	})
}

func TestConsumeDualSource(t *testing.T) {
	ctx := context.Background()

	t.Run("enforces Rank 3 inventory then Rank 5 depot lock order", func(t *testing.T) {
		recorder := &lockOrderRecorder{}
		invRepo := &mockConsumptionInvRepo{
			inv: coreinventory.Inventory{
				CharacterID: "char-1",
				Items:       []item.Instance{{ID: "i1", DefinitionID: "fertilizer", Quantity: 1}},
			},
			recorder: recorder,
		}
		depotRepo := &mockConsumptionDepotRepo{
			dep: depot.Depot{
				CharacterID: "char-1",
				Items:       []item.Instance{},
			},
			recorder: recorder,
		}

		res, err := depot.ConsumeDualSource(
			ctx,
			invRepo,
			depotRepo,
			"char-1",
			depot.QueryByDefinitionID("fertilizer"),
			depot.PriorityInventoryFirst,
			1,
		)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.Location != depot.LocationInventory {
			t.Errorf("expected LocationInventory, got %v", res.Location)
		}

		// Verify lock order Rank 3 -> Rank 5
		expectedCalls := []string{
			"inventory:FindByCharacterIDForUpdate",
			"depot:FindByCharacterIDForUpdate",
		}
		if len(recorder.calls) != len(expectedCalls) {
			t.Fatalf("expected calls %v, got %v", expectedCalls, recorder.calls)
		}
		for i := range expectedCalls {
			if recorder.calls[i] != expectedCalls[i] {
				t.Errorf("call %d: expected %s, got %s", i, expectedCalls[i], recorder.calls[i])
			}
		}

		if invRepo.savedInv == nil {
			t.Error("expected inventory to be saved")
		}
	})

	t.Run("returns ErrItemNotFound if item missing in both", func(t *testing.T) {
		invRepo := &mockConsumptionInvRepo{
			inv: coreinventory.Inventory{CharacterID: "char-1"},
		}
		depotRepo := &mockConsumptionDepotRepo{
			dep: depot.Depot{CharacterID: "char-1"},
		}

		_, err := depot.ConsumeDualSource(
			ctx,
			invRepo,
			depotRepo,
			"char-1",
			depot.QueryByDefinitionID("missing"),
			depot.PriorityInventoryFirst,
			1,
		)
		if !errors.Is(err, depot.ErrItemNotFound) {
			t.Errorf("expected ErrItemNotFound, got %v", err)
		}
	})

	t.Run("returns ErrInvalidCharacterID for empty character", func(t *testing.T) {
		_, err := depot.ConsumeDualSource(ctx, nil, nil, "", depot.QueryByDefinitionID("a"), depot.PriorityInventoryFirst, 1)
		if !errors.Is(err, depot.ErrInvalidCharacterID) {
			t.Errorf("expected ErrInvalidCharacterID, got %v", err)
		}
	})
}
