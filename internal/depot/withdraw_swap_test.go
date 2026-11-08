package depot

import (
	"context"
	"errors"
	"testing"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreequipment "github.com/witchcraze/party2re/internal/core/equipment"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	"github.com/witchcraze/party2re/internal/core/item"
)

type memoryEquipRepo struct {
	value   coreequipment.Equipment
	saves   int
	saveErr error
}

func (r *memoryEquipRepo) FindByCharacterIDForUpdate(_ context.Context, _ string) (coreequipment.Equipment, error) {
	return r.value.Clone(), nil
}

func (r *memoryEquipRepo) Save(_ context.Context, value coreequipment.Equipment) error {
	if r.saveErr != nil {
		return r.saveErr
	}
	r.saves++
	r.value = value.Clone()
	return nil
}

type swapFixture struct {
	char    corecharacter.Character
	chars   *memoryCharRepo
	invs    *memoryInvRepo
	depots  *memoryDepotRepo
	equip   *memoryEquipRepo
	service *Service
}

// newSwapFixture holds hand (equipping entries in slots) and stores stored.
func newSwapFixture(t *testing.T, hand []item.Instance, slots map[item.Slot]string, stored []item.Instance) swapFixture {
	t.Helper()
	catalog, err := item.InitialCatalog()
	if err != nil {
		t.Fatal(err)
	}
	f := swapFixture{chars: newMemoryCharRepo(), invs: newMemoryInvRepo(), depots: newMemoryDepotRepo()}
	f.char = corecharacter.Character{ID: "swap-char", WeaponCustomName: "Blade", WeaponSeal: 3, ArmorCustomName: "Mail"}
	f.chars.characters[f.char.ID] = f.char
	inv, _ := coreinventory.New(f.char.ID)
	inv.Items = append(inv.Items, hand...)
	f.invs.inventories[f.char.ID] = inv
	eq, _ := coreequipment.New(f.char.ID)
	for slot, instanceID := range slots {
		eq.Slots[slot] = instanceID
	}
	f.equip = &memoryEquipRepo{value: eq}
	dep, _ := NewDepot(f.char.ID)
	dep.Items = stored
	f.depots.depots[f.char.ID] = dep
	f.service, err = NewService(f.depots, f.chars, f.invs, WithItemDefinitionProvider(catalog), WithEquipmentRepository(f.equip))
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func ids(items []item.Instance) []string {
	out := make([]string, len(items))
	for i, inst := range items {
		out[i] = inst.ID
	}
	return out
}

func equalIDs(got []item.Instance, want ...string) bool {
	g := ids(got)
	if len(g) != len(want) {
		return false
	}
	for i := range g {
		if g[i] != want[i] {
			return false
		}
	}
	return true
}

func TestWithdrawItem_SwapsSameLegacyHand(t *testing.T) {
	ctx := context.Background()
	oldSword := item.Instance{ID: "old-sword", DefinitionID: "weapon-01", Quantity: 1, EnhancementLevel: 4}
	newSword := item.Instance{ID: "new-sword", DefinitionID: "weapon-02", Quantity: 1}
	herb := item.Instance{ID: "held-herb", DefinitionID: "item-001", Quantity: 1}
	before := item.Instance{ID: "before", DefinitionID: "armor-01", Quantity: 1}
	after := item.Instance{ID: "after", DefinitionID: "armor-02", Quantity: 1}

	f := newSwapFixture(t,
		[]item.Instance{oldSword, herb},
		map[item.Slot]string{item.SlotMainHand: oldSword.ID},
		[]item.Instance{before, newSword, after})

	dep, err := f.service.WithdrawItem(ctx, f.char.ID, newSword.ID)
	if err != nil {
		t.Fatal(err)
	}
	// The old weapon takes the withdrawn row's position, enhancement intact.
	if !equalIDs(dep.Items, "before", "old-sword", "after") || dep.Items[1].EnhancementLevel != 4 {
		t.Fatalf("depot = %+v", dep.Items)
	}
	inv := f.invs.inventories[f.char.ID]
	if !equalIDs(inv.Items, "held-herb", "new-sword") {
		t.Fatalf("hand = %+v (the held item belongs to another field)", inv.Items)
	}
	if got, _ := f.equip.value.Equipped(item.SlotMainHand); got != newSword.ID || len(f.equip.value.Slots) != 1 {
		t.Fatalf("equipment = %+v", f.equip.value.Slots)
	}
	char := f.chars.characters[f.char.ID]
	if char.WeaponCustomName != "" || char.WeaponSeal != 0 || char.ArmorCustomName != "Mail" {
		t.Fatalf("customization = %q/%d/%q", char.WeaponCustomName, char.WeaponSeal, char.ArmorCustomName)
	}
}

func TestWithdrawItem_ArmorIntoEmptyHandClearsOnlyArmorName(t *testing.T) {
	ctx := context.Background()
	mail := item.Instance{ID: "mail", DefinitionID: "armor-01", Quantity: 1}
	f := newSwapFixture(t, nil, nil, []item.Instance{mail})

	dep, err := f.service.WithdrawItem(ctx, f.char.ID, mail.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(dep.Items) != 0 {
		t.Fatalf("depot = %+v", dep.Items)
	}
	if got, _ := f.equip.value.Equipped(item.SlotBody); got != mail.ID {
		t.Fatalf("equipment = %+v", f.equip.value.Slots)
	}
	char := f.chars.characters[f.char.ID]
	if char.ArmorCustomName != "" || char.WeaponCustomName != "Blade" || char.WeaponSeal != 3 {
		t.Fatalf("customization = %q/%d/%q", char.WeaponCustomName, char.WeaponSeal, char.ArmorCustomName)
	}
}

func TestWithdrawItem_ShieldAndConsumableShareHeldHand(t *testing.T) {
	ctx := context.Background()
	shield := item.Instance{ID: "shield", DefinitionID: "item-071", Quantity: 1}
	herbs := item.Instance{ID: "herbs", DefinitionID: "item-001", Quantity: 3}
	f := newSwapFixture(t,
		[]item.Instance{shield},
		map[item.Slot]string{item.SlotOffHand: shield.ID},
		[]item.Instance{herbs})

	dep, err := f.service.WithdrawItem(ctx, f.char.ID, herbs.ID)
	if err != nil {
		t.Fatal(err)
	}
	// The stack keeps its row; the shield is inserted at that position.
	if !equalIDs(dep.Items, "shield", "herbs") || dep.Items[1].Quantity != 2 {
		t.Fatalf("depot = %+v", dep.Items)
	}
	inv := f.invs.inventories[f.char.ID]
	if len(inv.Items) != 1 || inv.Items[0].DefinitionID != "item-001" || inv.Items[0].ID == herbs.ID {
		t.Fatalf("hand = %+v", inv.Items)
	}
	if len(f.equip.value.Slots) != 0 {
		t.Fatalf("shield still equipped: %+v", f.equip.value.Slots)
	}
	char := f.chars.characters[f.char.ID]
	if char.WeaponCustomName != "Blade" || char.ArmorCustomName != "Mail" {
		t.Fatalf("held-item swap changed customization: %+v", char)
	}
}

func TestWithdrawItem_FullDepotNeverRejectsSwap(t *testing.T) {
	ctx := context.Background()
	fill := func(n int) []item.Instance {
		rows := make([]item.Instance, n)
		for i := range rows {
			rows[i] = item.Instance{ID: "gear-" + string(rune('a'+i)), DefinitionID: "weapon-01", Quantity: 1}
		}
		return rows
	}

	t.Run("same definition merges into the stack", func(t *testing.T) {
		held := item.Instance{ID: "held", DefinitionID: "item-001", Quantity: 1}
		stack := item.Instance{ID: "stack", DefinitionID: "item-001", Quantity: 2}
		f := newSwapFixture(t, []item.Instance{held}, nil, append(fill(MinDepotCapacity-1), stack))
		dep, err := f.service.WithdrawItem(ctx, f.char.ID, stack.ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(dep.Items) != MinDepotCapacity || dep.Items[len(dep.Items)-1].Quantity != 2 {
			t.Fatalf("depot = %+v", dep.Items)
		}
	})

	t.Run("different held item may exceed capacity by one", func(t *testing.T) {
		held := item.Instance{ID: "held", DefinitionID: "item-002", Quantity: 1}
		stack := item.Instance{ID: "stack", DefinitionID: "item-001", Quantity: 2}
		f := newSwapFixture(t, []item.Instance{held}, nil, append(fill(MinDepotCapacity-1), stack))
		dep, err := f.service.WithdrawItem(ctx, f.char.ID, stack.ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(dep.Items) != MinDepotCapacity+1 || dep.Items[MinDepotCapacity-1].ID != "held" {
			t.Fatalf("depot = %+v", dep.Items)
		}
	})
}

func TestWithdrawItem_RejectsWithoutChange(t *testing.T) {
	ctx := context.Background()
	herb := item.Instance{ID: "herb", DefinitionID: "item-001", Quantity: 1}

	t.Run("ambiguous held hand", func(t *testing.T) {
		a := item.Instance{ID: "a", DefinitionID: "item-002", Quantity: 1}
		b := item.Instance{ID: "b", DefinitionID: "item-071", Quantity: 1}
		f := newSwapFixture(t, []item.Instance{a, b}, nil, []item.Instance{herb})
		if _, err := f.service.WithdrawItem(ctx, f.char.ID, herb.ID); !errors.Is(err, ErrInventoryFull) {
			t.Fatalf("err = %v", err)
		}
		if !equalIDs(f.depots.depots[f.char.ID].Items, "herb") {
			t.Fatal("rejection changed the depot")
		}
	})

	t.Run("unclassifiable held item", func(t *testing.T) {
		odd := item.Instance{ID: "odd", DefinitionID: "unknown-1", Quantity: 1}
		f := newSwapFixture(t, []item.Instance{odd}, nil, []item.Instance{herb})
		if _, err := f.service.WithdrawItem(ctx, f.char.ID, herb.ID); err == nil {
			t.Fatal("expected catalog error")
		}
	})

	t.Run("equipment save failure", func(t *testing.T) {
		sword := item.Instance{ID: "sword", DefinitionID: "weapon-01", Quantity: 1}
		next := item.Instance{ID: "next", DefinitionID: "weapon-02", Quantity: 1}
		f := newSwapFixture(t, []item.Instance{sword}, map[item.Slot]string{item.SlotMainHand: sword.ID}, []item.Instance{next})
		boom := errors.New("equip save")
		f.equip.saveErr = boom
		if _, err := f.service.WithdrawItem(ctx, f.char.ID, next.ID); !errors.Is(err, boom) {
			t.Fatalf("err = %v", err)
		}
		// Rollback needs a real transaction: cmd/party2/depot_equipment_test.go.
	})
}
