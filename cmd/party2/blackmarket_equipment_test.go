package main

import (
	"context"
	"errors"
	"os"
	"sync/atomic"
	"testing"

	"github.com/witchcraze/party2re/internal/blackmarket"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreequipment "github.com/witchcraze/party2re/internal/core/equipment"
	"github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/depot"
	"github.com/witchcraze/party2re/internal/logging"
	"github.com/witchcraze/party2re/internal/testutil"
)

type blackMarketEquipmentFixture struct {
	core     *coreServices
	econ     *econServices
	misc     *miscServices
	char     corecharacter.Character
	instance item.Instance
}

func newBlackMarketEquipmentFixture(t *testing.T, slot item.Slot, definitionID string) blackMarketEquipmentFixture {
	t.Helper()
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}
	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	core, err := newCoreServices(db, nil)
	if err != nil {
		t.Fatal(err)
	}
	econ, err := newEconServices(db, core)
	if err != nil {
		t.Fatal(err)
	}
	soc, err := newSocServices(db, core, econ, nil, logging.Nop())
	if err != nil {
		t.Fatal(err)
	}
	misc, err := newMiscServices(db, core, soc, econ, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	char, err := database.CreateTestCharacterWithFunds(ctx, db, "Black market equipped", 0)
	if err != nil {
		t.Fatal(err)
	}
	char.JobLevel = blackmarket.MinAccessJobLevel
	char.WeaponCustomName, char.WeaponSeal, char.ArmorCustomName = "Named blade", 3, "Named mail"
	if err := core.charRepo.Update(ctx, char); err != nil {
		t.Fatal(err)
	}
	instance, err := item.NewInstance(definitionID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.CreateTestInventoryWithItems(ctx, db, char.ID, []item.Instance{instance}); err != nil {
		t.Fatal(err)
	}
	if err := econ.equipRepo.Save(ctx, coreequipment.Equipment{CharacterID: char.ID, Slots: map[item.Slot]string{slot: instance.ID}}); err != nil {
		t.Fatal(err)
	}
	return blackMarketEquipmentFixture{core, econ, misc, char, instance}
}

// assertUnequipped checks the slot is empty and only that slot's customization was cleared.
func (f blackMarketEquipmentFixture) assertUnequipped(t *testing.T, slot item.Slot) {
	t.Helper()
	ctx := context.Background()
	eq, err := f.econ.equipRepo.FindByCharacterID(ctx, f.char.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(eq.Slots) != 0 {
		t.Fatalf("equipment remains: %+v", eq.Slots)
	}
	want := f.char
	switch slot {
	case item.SlotMainHand:
		want.ClearWeaponCustomization()
	case item.SlotBody:
		want.ClearArmorCustomization()
	}
	got, err := f.core.charRepo.FindByID(ctx, f.char.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.WeaponCustomName != want.WeaponCustomName || got.WeaponSeal != want.WeaponSeal || got.ArmorCustomName != want.ArmorCustomName {
		t.Fatalf("customization: got %+v", got)
	}
}

func TestWireBlackMarketSacrificesEquippedItems_Integration(t *testing.T) {
	for _, tc := range []struct {
		slot         item.Slot
		definitionID string
	}{{item.SlotMainHand, "weapon-29"}, {item.SlotBody, "armor-35"}, {item.SlotOffHand, "item-071"}} {
		t.Run(string(tc.slot), func(t *testing.T) {
			f := newBlackMarketEquipmentFixture(t, tc.slot, tc.definitionID)
			ctx := context.Background()
			res, err := f.misc.blackmarket.SacrificeItem(ctx, f.char.ID, f.instance.ID)
			if err != nil {
				t.Fatal(err)
			}
			if res.TotalRarePoints != 1 {
				t.Fatalf("points = %d, want 1", res.TotalRarePoints)
			}
			inv, err := f.core.invRepo.FindByCharacterID(ctx, f.char.ID)
			if err != nil {
				t.Fatal(err)
			}
			if len(inv.Items) != 0 {
				t.Fatalf("inventory remains: %+v", inv.Items)
			}
			f.assertUnequipped(t, tc.slot)
		})
	}
}

func TestWireBlackMarketSacrificeRacesDepotDeposit_Integration(t *testing.T) {
	f := newBlackMarketEquipmentFixture(t, item.SlotMainHand, "weapon-29")
	ctx := context.Background()
	var sacrificed, deposited atomic.Int64
	sacrifice := func() error {
		_, err := f.misc.blackmarket.SacrificeItem(ctx, f.char.ID, f.instance.ID)
		if err == nil {
			sacrificed.Add(1)
		}
		return err
	}
	deposit := func() error {
		_, err := f.econ.depot.DepositItem(ctx, f.char.ID, f.instance.ID)
		if errors.Is(err, depot.ErrItemNotFound) {
			return nil
		}
		if err == nil {
			deposited.Add(1)
		}
		return err
	}
	for _, err := range testutil.RunRace(sacrifice, deposit) {
		// The sacrifice that loses to a deposit takes the Depot copy instead.
		if err != nil {
			t.Fatal(err)
		}
	}
	points, err := f.misc.blackmarket.GetStatus(ctx, f.char.ID)
	if err != nil {
		t.Fatal(err)
	}
	if sacrificed.Load() != 1 || points.RarePoints != 1 || len(points.SacrificeCandidates) != 0 {
		t.Fatalf("sacrificed=%d deposited=%d status=%+v", sacrificed.Load(), deposited.Load(), points)
	}
	f.assertUnequipped(t, item.SlotMainHand)
}

func TestWireBlackMarketPartialStackKeepsOtherEquipment_Integration(t *testing.T) {
	f := newBlackMarketEquipmentFixture(t, item.SlotMainHand, "weapon-29")
	ctx := context.Background()
	stack, err := item.NewInstance("item-028", 2)
	if err != nil {
		t.Fatal(err)
	}
	inv, err := f.core.invRepo.FindByCharacterID(ctx, f.char.ID)
	if err != nil {
		t.Fatal(err)
	}
	inv.Items = append(inv.Items, stack)
	if err := f.core.invRepo.Save(ctx, inv); err != nil {
		t.Fatal(err)
	}
	if _, err := f.misc.blackmarket.SacrificeItem(ctx, f.char.ID, stack.ID); err != nil {
		t.Fatal(err)
	}
	inv, err = f.core.invRepo.FindByCharacterID(ctx, f.char.ID)
	if err != nil {
		t.Fatal(err)
	}
	if remaining, ok := inv.Find(stack.ID); !ok || remaining.Quantity != 1 || len(inv.Items) != 2 {
		t.Fatalf("inventory after partial sacrifice: %+v", inv.Items)
	}
	eq, err := f.econ.equipRepo.FindByCharacterID(ctx, f.char.ID)
	if err != nil {
		t.Fatal(err)
	}
	got, err := f.core.charRepo.FindByID(ctx, f.char.ID)
	if err != nil {
		t.Fatal(err)
	}
	if eq.Slots[item.SlotMainHand] != f.instance.ID || got.WeaponCustomName != f.char.WeaponCustomName || got.WeaponSeal != f.char.WeaponSeal {
		t.Fatalf("unrelated equipment changed: slots=%+v char=%+v", eq.Slots, got)
	}
}
