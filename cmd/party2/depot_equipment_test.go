package main

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreequipment "github.com/witchcraze/party2re/internal/core/equipment"
	"github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/depot"
	"github.com/witchcraze/party2re/internal/testutil"
)

type depotEquipmentFixture struct {
	db                *sql.DB
	core              *coreServices
	econ              *econServices
	sender, recipient corecharacter.Character
	instance          item.Instance
}

func newDepotEquipmentFixture(t *testing.T, slot item.Slot, equipped bool) depotEquipmentFixture {
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
	ctx := context.Background()
	sender, err := database.CreateTestCharacterWithFunds(ctx, db, "Depot equipped sender", 1000)
	if err != nil {
		t.Fatal(err)
	}
	recipient, err := database.CreateTestCharacterWithFunds(ctx, db, "Depot equipped recipient", 2000)
	if err != nil {
		t.Fatal(err)
	}
	sender.WeaponCustomName, sender.ArmorCustomName, sender.WeaponSeal = "Weapon name", "Armor name", 3
	recipient.WeaponCustomName, recipient.ArmorCustomName, recipient.WeaponSeal = "Recipient weapon", "Recipient armor", 2
	for _, char := range []corecharacter.Character{sender, recipient} {
		if err := core.charRepo.Update(ctx, char); err != nil {
			t.Fatal(err)
		}
	}
	definitionID := map[item.Slot]string{item.SlotMainHand: "weapon-01", item.SlotBody: "armor-01", item.SlotOffHand: "item-071", item.SlotAccessory: "item-012", item.SlotNone: "weapon-01"}[slot]
	def, err := core.itemCatalog.FindByID(definitionID)
	if err != nil {
		t.Fatal(err)
	}
	instance, err := def.NewInstanceWithEnhancement(1, 6)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.CreateTestInventoryWithItems(ctx, db, sender.ID, []item.Instance{instance}); err != nil {
		t.Fatal(err)
	}
	eq, err := coreequipment.New(sender.ID)
	if err != nil {
		t.Fatal(err)
	}
	if equipped {
		inv, err := core.invRepo.FindByCharacterID(ctx, sender.ID)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := eq.Equip(&inv, def, instance.ID); err != nil {
			t.Fatal(err)
		}
	}
	if err := econ.equipRepo.Save(ctx, eq); err != nil {
		t.Fatal(err)
	}
	return depotEquipmentFixture{db, core, econ, sender, recipient, instance}
}

func (f depotEquipmentFixture) move(ctx context.Context, send bool) (depot.Depot, error) {
	if send {
		return f.econ.depot.SendItem(ctx, f.sender.ID, f.recipient.ID, f.instance.ID)
	}
	return f.econ.depot.DepositItem(ctx, f.sender.ID, f.instance.ID)
}

func (f depotEquipmentFixture) assertMoved(t *testing.T, slot item.Slot, equipped, send bool) {
	t.Helper()
	ctx := context.Background()
	inv, err := f.core.invRepo.FindByCharacterID(ctx, f.sender.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(inv.Items) != 0 {
		t.Fatalf("sender still owns inventory: %+v", inv)
	}
	eq, err := f.econ.equipRepo.FindByCharacterID(ctx, f.sender.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(eq.Slots) != 0 {
		t.Fatalf("sender equipment remains: %+v", eq)
	}
	expected := f.sender
	if equipped && slot == item.SlotMainHand {
		expected.ClearWeaponCustomization()
	}
	if equipped && slot == item.SlotBody {
		expected.ClearArmorCustomization()
	}
	char, err := f.core.charRepo.FindByID(ctx, f.sender.ID)
	if err != nil {
		t.Fatal(err)
	}
	if char.WeaponCustomName != expected.WeaponCustomName || char.ArmorCustomName != expected.ArmorCustomName || char.WeaponSeal != expected.WeaponSeal || char.Money != expected.Money {
		t.Fatalf("unexpected sender metadata: %+v", char)
	}
	recipient, err := f.core.charRepo.FindByID(ctx, f.recipient.ID)
	if err != nil {
		t.Fatal(err)
	}
	if recipient.WeaponCustomName != f.recipient.WeaponCustomName || recipient.ArmorCustomName != f.recipient.ArmorCustomName || recipient.WeaponSeal != f.recipient.WeaponSeal || recipient.Money != f.recipient.Money {
		t.Fatalf("recipient metadata changed: %+v", recipient)
	}
	owner, other := f.sender.ID, f.recipient.ID
	if send {
		owner, other = other, owner
	}
	storage, err := f.econ.depotRepo.FindByCharacterID(ctx, owner)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(storage.Items, []item.Instance{f.instance}) {
		t.Fatalf("moved item changed: %+v", storage.Items)
	}
	otherStorage, err := f.econ.depotRepo.FindByCharacterID(ctx, other)
	if err != nil {
		t.Fatal(err)
	}
	if len(otherStorage.Items) != 0 {
		t.Fatalf("item duplicated into other depot: %+v", otherStorage)
	}
}

func TestWireDepotEquipmentMovement_Integration(t *testing.T) {
	for _, send := range []bool{false, true} {
		name := "deposit"
		if send {
			name = "send"
		}
		for _, slot := range []item.Slot{item.SlotMainHand, item.SlotBody, item.SlotOffHand, item.SlotAccessory, item.SlotNone} {
			slotName := string(slot)
			if slot == item.SlotNone {
				slotName = "unequipped"
			}
			t.Run(name+"/"+slotName, func(t *testing.T) {
				equipped := slot != item.SlotNone
				f := newDepotEquipmentFixture(t, slot, equipped)
				if _, err := f.move(context.Background(), send); err != nil {
					t.Fatal(err)
				}
				f.assertMoved(t, slot, equipped, send)
			})
		}
	}
}

func TestWireDepotEquipmentFullRollback_Integration(t *testing.T) {
	for _, send := range []bool{false, true} {
		name := "deposit"
		if send {
			name = "send"
		}
		t.Run(name, func(t *testing.T) {
			f := newDepotEquipmentFixture(t, item.SlotMainHand, true)
			ctx := context.Background()
			target := f.sender.ID
			if send {
				target = f.recipient.ID
			}
			items := make([]item.Instance, depot.MinDepotCapacity)
			for i := range items {
				var err error
				items[i], err = item.NewInstanceWithEnhancement("weapon-01", 1, 1)
				if err != nil {
					t.Fatal(err)
				}
			}
			if _, err := database.CreateTestDepot(ctx, f.db, target, 0, items); err != nil {
				t.Fatal(err)
			}
			before := f.state(t)
			_, err := f.move(ctx, send)
			expected := depot.ErrDepotFull
			if send {
				expected = depot.ErrRecipientDepotFull
			}
			if !errors.Is(err, expected) {
				t.Fatalf("error = %v, want %v", err, expected)
			}
			if after := f.state(t); !reflect.DeepEqual(before, after) {
				t.Fatal("full storage rejection changed assets or equipment")
			}
		})
	}
}

func TestWireDepotEquipmentMovementConcurrency_Integration(t *testing.T) {
	f := newDepotEquipmentFixture(t, item.SlotMainHand, true)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var deposits, sends atomic.Int64
	res := testutil.RunConcurrentStressTest(t, testutil.GetStressConfig(), func(worker, op int) error {
		send := (worker+op)%2 == 0
		_, err := f.move(ctx, send)
		if errors.Is(err, depot.ErrItemNotFound) {
			return nil
		}
		if err == nil {
			if send {
				sends.Add(1)
			} else {
				deposits.Add(1)
			}
		}
		return err
	})
	if res.Failures != 0 || deposits.Load()+sends.Load() != 1 {
		t.Fatalf("result=%+v deposits=%d sends=%d", res, deposits.Load(), sends.Load())
	}
	f.assertMoved(t, item.SlotMainHand, true, sends.Load() == 1)
}
