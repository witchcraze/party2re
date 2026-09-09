package home

import (
	"context"
	"testing"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/depot"
)

type mockInventoryManager struct {
	invs map[string]coreinventory.Inventory
}

func (m *mockInventoryManager) FindByCharacterID(ctx context.Context, characterID string) (coreinventory.Inventory, error) {
	inv, ok := m.invs[characterID]
	if !ok {
		return coreinventory.New(characterID)
	}
	return inv, nil
}

func (m *mockInventoryManager) Consume(ctx context.Context, characterID, instanceID string, quantity int) (coreinventory.Inventory, error) {
	inv, _ := m.FindByCharacterID(ctx, characterID)
	_ = inv.Consume(instanceID, quantity)
	m.invs[characterID] = inv
	return inv, nil
}

type mockDepotManager struct {
	depots map[string]depot.Depot
}

func (m *mockDepotManager) FindByCharacterID(ctx context.Context, characterID string) (depot.Depot, error) {
	dp, ok := m.depots[characterID]
	if !ok {
		return depot.NewDepot(characterID)
	}
	return dp, nil
}

func (m *mockDepotManager) RemoveItem(ctx context.Context, characterID, itemInstanceID string) error {
	dp, ok := m.depots[characterID]
	if !ok {
		return depot.ErrNotFound
	}
	for i, it := range dp.Items {
		if it.ID == itemInstanceID {
			dp.Items = append(dp.Items[:i], dp.Items[i+1:]...)
			m.depots[characterID] = dp
			return nil
		}
	}
	return depot.ErrItemNotFound
}

type mockCatalog struct {
	defs map[string]coreitem.Definition
}

func (m *mockCatalog) FindByID(id string) (coreitem.Definition, error) {
	d, ok := m.defs[id]
	if !ok {
		return coreitem.Definition{}, coreitem.ErrDefinitionNotFound
	}
	return d, nil
}

func TestItemUsage(t *testing.T) {
	ctx := context.Background()

	chars := map[string]corecharacter.Character{
		"char-1": {
			ID:       "char-1",
			PlayerID: "p1",
			Name:     "Hero",
			Stats: corecharacter.Stats{
				MaxHP:   100,
				HP:      50,
				MaxMP:   50,
				MP:      20,
				Attack:  20,
				Defense: 15,
				Agility: 10,
			},
			SP:          5,
			SmallMedals: 0,
		},
	}
	charReader := &mockCharReader{chars: chars}
	charUpdater := &mockCharUpdater{chars: chars}
	repo := newMockHomeRepo(chars)

	invMgr := &mockInventoryManager{invs: make(map[string]coreinventory.Inventory)}
	depotMgr := &mockDepotManager{depots: make(map[string]depot.Depot)}
	catalog := &mockCatalog{
		defs: map[string]coreitem.Definition{
			"item-wea":   {ID: "item-wea", Name: "銅の剣", Price: 100, Slot: coreitem.SlotMainHand},
			"item-arm":   {ID: "item-arm", Name: "革の鎧", Price: 150, Slot: coreitem.SlotBody},
			"item-seed1": {ID: "item-seed1", Name: "命の木の実", Price: 50, Slot: coreitem.SlotNone},
			"item-seed2": {ID: "item-seed2", Name: "不思議な木の実", Price: 50, Slot: coreitem.SlotNone},
			"item-seed3": {ID: "item-seed3", Name: "力の種", Price: 50, Slot: coreitem.SlotNone},
			"item-seed4": {ID: "item-seed4", Name: "守りの種", Price: 50, Slot: coreitem.SlotNone},
			"item-seed5": {ID: "item-seed5", Name: "素早さの種", Price: 50, Slot: coreitem.SlotNone},
			"item-seed6": {ID: "item-seed6", Name: "スキルの種", Price: 50, Slot: coreitem.SlotNone},
			"item-medal": {ID: "item-medal", Name: "小さなメダル", Price: 0, Slot: coreitem.SlotNone},
			"item-herb":  {ID: "item-herb", Name: "薬草", Price: 8, Slot: coreitem.SlotNone},
			"item-other": {ID: "item-other", Name: "爆弾岩の破片", Price: 200, Slot: coreitem.SlotNone},
		},
	}

	inv, _ := coreinventory.New("char-1")
	_ = inv.Add(coreitem.Instance{ID: "inst-wea", DefinitionID: "item-wea", Quantity: 1})
	_ = inv.Add(coreitem.Instance{ID: "inst-seed1", DefinitionID: "item-seed1", Quantity: 1})
	_ = inv.Add(coreitem.Instance{ID: "inst-herb", DefinitionID: "item-herb", Quantity: 2})
	_ = inv.Add(coreitem.Instance{ID: "inst-other", DefinitionID: "item-other", Quantity: 1})
	invMgr.invs["char-1"] = inv

	dp, _ := depot.NewDepot("char-1")
	dp.Items = append(dp.Items,
		coreitem.Instance{ID: "inst-arm", DefinitionID: "item-arm", Quantity: 1},
		coreitem.Instance{ID: "inst-seed2", DefinitionID: "item-seed2", Quantity: 1},
		coreitem.Instance{ID: "inst-medal", DefinitionID: "item-medal", Quantity: 1},
	)
	depotMgr.depots["char-1"] = dp

	svc, err := NewService(
		repo,
		charReader,
		WithCharacterUpdater(charUpdater),
		WithInventoryManager(invMgr),
		WithDepotManager(depotMgr),
		WithItemCatalog(catalog),
		WithNowFunc(func() time.Time { return time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC) }),
	)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	t.Run("list home items", func(t *testing.T) {
		items, err := svc.ListHomeItems(ctx, "char-1")
		if err != nil {
			t.Fatalf("ListHomeItems failed: %v", err)
		}
		if len(items) != 7 {
			t.Errorf("expected 7 items total, got %d", len(items))
		}
	})

	t.Run("inspect weapon and armor", func(t *testing.T) {
		resWea, err := svc.UseHomeItem(ctx, "char-1", "inst-wea", "inventory")
		if err != nil {
			t.Fatalf("UseHomeItem weapon failed: %v", err)
		}
		if resWea.Action != "inspect" || resWea.Consumed {
			t.Errorf("expected inspect and not consumed, got %+v", resWea)
		}

		resArm, err := svc.UseHomeItem(ctx, "char-1", "inst-arm", "depot")
		if err != nil {
			t.Fatalf("UseHomeItem armor failed: %v", err)
		}
		if resArm.Action != "inspect" || resArm.Consumed {
			t.Errorf("expected inspect and not consumed, got %+v", resArm)
		}
	})

	t.Run("consume seeds and restore HP", func(t *testing.T) {
		oldMaxHP := chars["char-1"].Stats.MaxHP
		resSeed, err := svc.UseHomeItem(ctx, "char-1", "inst-seed1", "inventory")
		if err != nil {
			t.Fatalf("UseHomeItem seed failed: %v", err)
		}
		if resSeed.Action != "consumed" || !resSeed.Consumed {
			t.Errorf("expected consumed, got %+v", resSeed)
		}
		if chars["char-1"].Stats.MaxHP <= oldMaxHP {
			t.Errorf("expected MaxHP to increase from %d, got %d", oldMaxHP, chars["char-1"].Stats.MaxHP)
		}

		// Consume herb for HP restoration
		oldHP := chars["char-1"].Stats.HP
		resHerb, err := svc.UseHomeItem(ctx, "char-1", "inst-herb", "inventory")
		if err != nil {
			t.Fatalf("UseHomeItem herb failed: %v", err)
		}
		if resHerb.Action != "consumed" {
			t.Errorf("expected consumed, got %+v", resHerb)
		}
		if chars["char-1"].Stats.HP <= oldHP {
			t.Errorf("expected HP to increase from %d, got %d", oldHP, chars["char-1"].Stats.HP)
		}

		// Consume small medal from depot
		resMedal, err := svc.UseHomeItem(ctx, "char-1", "inst-medal", "depot")
		if err != nil {
			t.Fatalf("UseHomeItem medal failed: %v", err)
		}
		if resMedal.Action != "consumed" {
			t.Errorf("expected consumed, got %+v", resMedal)
		}
		if chars["char-1"].SmallMedals != 1 {
			t.Errorf("expected 1 small medal, got %d", chars["char-1"].SmallMedals)
		}
	})

	t.Run("cannot use battle-only item", func(t *testing.T) {
		res, err := svc.UseHomeItem(ctx, "char-1", "inst-other", "inventory")
		if err != nil {
			t.Fatalf("UseHomeItem failed: %v", err)
		}
		if res.Action != "cannot_use" || res.Consumed {
			t.Errorf("expected cannot_use, got %+v", res)
		}
	})
}
