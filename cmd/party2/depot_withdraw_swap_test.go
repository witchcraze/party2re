package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/depot"
	"github.com/witchcraze/party2re/internal/testutil"
)

// withSwapStorage stores rows ahead of the fixture's equipped main-hand weapon.
func withSwapStorage(t *testing.T, f depotEquipmentFixture, defs ...string) []item.Instance {
	t.Helper()
	rows := make([]item.Instance, len(defs))
	for i, def := range defs {
		var err error
		if rows[i], err = item.NewInstance(def, 1); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := database.CreateTestDepot(context.Background(), f.db, f.sender.ID, 0, rows); err != nil {
		t.Fatal(err)
	}
	return rows
}

func (f depotEquipmentFixture) assertWeaponSwapped(t *testing.T, before depotEquipmentState, mail, sword item.Instance) {
	t.Helper()
	after := f.state(t)
	if !reflect.DeepEqual(after.inventory.Items, []item.Instance{sword}) {
		t.Fatalf("hand = %+v", after.inventory.Items)
	}
	if !reflect.DeepEqual(after.equipment.Slots, map[item.Slot]string{item.SlotMainHand: sword.ID}) {
		t.Fatalf("equipment = %+v", after.equipment.Slots)
	}
	// The replaced weapon returns to the withdrawn row, keeping identity and enhancement.
	if !reflect.DeepEqual(after.senderDepot.Items, []item.Instance{mail, f.instance}) {
		t.Fatalf("depot = %+v", after.senderDepot.Items)
	}
	want := before.sender
	want.ClearWeaponCustomization()
	if after.sender.WeaponCustomName != "" || after.sender.WeaponSeal != 0 || after.sender.ArmorCustomName != want.ArmorCustomName || after.sender.Money != want.Money {
		t.Fatalf("sender = %+v", after.sender)
	}
	if !reflect.DeepEqual(before.recipient, after.recipient) || !reflect.DeepEqual(before.recipientDepot, after.recipientDepot) {
		t.Fatal("withdrawal touched another character")
	}
}

func TestWireDepotWithdrawSwapsEquippedWeapon_Integration(t *testing.T) {
	f := newDepotEquipmentFixture(t, item.SlotMainHand, true)
	rows := withSwapStorage(t, f, "armor-01", "weapon-02")
	before := f.state(t)
	if _, err := f.econ.depot.WithdrawItem(context.Background(), f.sender.ID, rows[1].ID); err != nil {
		t.Fatal(err)
	}
	f.assertWeaponSwapped(t, before, rows[0], rows[1])
}

func TestWireDepotWithdrawSwapRollback_Integration(t *testing.T) {
	for _, stage := range []string{"equipment read", "equipment save", "character save", "inventory save", "depot save"} {
		t.Run(stage, func(t *testing.T) {
			f := newDepotEquipmentFixture(t, item.SlotMainHand, true)
			rows := withSwapStorage(t, f, "armor-01", "weapon-02")
			var inv depot.InventoryRepository = f.core.invRepo
			var chars depot.CharacterRepository = f.core.charRepo
			var storage depot.Repository = f.econ.depotRepo
			switch stage {
			case "inventory save":
				inv = depotMovementInventoryFailure{f.core.invRepo}
			case "character save":
				chars = depotMovementCharacterFailure{f.core.charRepo}
			case "depot save":
				storage = depotMovementStorageFailure{f.econ.depotRepo}
			}
			svc, err := depot.NewServiceWithTransaction(storage, chars, inv, f.core.txProvider,
				depot.WithItemDefinitionProvider(f.core.itemCatalog),
				depot.WithEquipmentRepository(depotEquipmentFailure{f.econ.equipRepo, stage}))
			if err != nil {
				t.Fatal(err)
			}
			before := f.state(t)
			if _, err := svc.WithdrawItem(context.Background(), f.sender.ID, rows[1].ID); !errors.Is(err, errDepotEquipmentPersistence) {
				t.Fatalf("error = %v", err)
			}
			if after := f.state(t); !reflect.DeepEqual(before, after) {
				t.Fatalf("failed %s changed state:\nbefore=%+v\nafter=%+v", stage, before, after)
			}
			if _, err := f.econ.depot.WithdrawItem(context.Background(), f.sender.ID, rows[1].ID); err != nil {
				t.Fatalf("retry: %v", err)
			}
			f.assertWeaponSwapped(t, before, rows[0], rows[1])
		})
	}
}

func TestWireDepotWithdrawSwapConcurrency_Integration(t *testing.T) {
	f := newDepotEquipmentFixture(t, item.SlotMainHand, true)
	rows := withSwapStorage(t, f, "weapon-02", "weapon-03")
	weapons := []string{f.instance.ID, rows[0].ID, rows[1].ID}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	res := testutil.RunConcurrentStressTest(t, testutil.GetStressConfig(), func(worker, op int) error {
		// Whole-row swaps keep identities; only the weapon currently held is absent.
		_, err := f.econ.depot.WithdrawItem(ctx, f.sender.ID, weapons[(worker+op)%3])
		if errors.Is(err, depot.ErrItemNotFound) {
			return nil
		}
		return err
	})
	if res.Failures != 0 || res.Deadlocks != 0 {
		t.Fatalf("result=%+v", res)
	}
	after := f.state(t)
	if len(after.inventory.Items) != 1 || len(after.senderDepot.Items) != 2 {
		t.Fatalf("hand=%+v depot=%+v", after.inventory.Items, after.senderDepot.Items)
	}
	held := after.inventory.Items[0].ID
	if got, _ := after.equipment.Equipped(item.SlotMainHand); got != held || len(after.equipment.Slots) != 1 {
		t.Fatalf("equipment=%+v hand=%s", after.equipment.Slots, held)
	}
	owned := []string{held, after.senderDepot.Items[0].ID, after.senderDepot.Items[1].ID}
	slices.Sort(owned)
	slices.Sort(weapons)
	if !slices.Equal(owned, weapons) {
		t.Fatalf("weapons not conserved: %v want %v", owned, weapons)
	}
}

func TestWireDepotGatewayWithdrawSwap_Integration(t *testing.T) {
	for _, failSave := range []bool{false, true} {
		t.Run(fmt.Sprintf("failSave=%t", failSave), func(t *testing.T) {
			f := newDepotEquipmentFixture(t, item.SlotMainHand, true)
			rows := withSwapStorage(t, f, "armor-01", "weapon-02")
			if failSave {
				var err error
				f.econ.depot, err = depot.NewServiceWithTransaction(depotMovementStorageFailure{f.econ.depotRepo}, f.core.charRepo, f.core.invRepo, f.core.txProvider,
					depot.WithItemDefinitionProvider(f.core.itemCatalog), depot.WithEquipmentRepository(f.econ.equipRepo))
				if err != nil {
					t.Fatal(err)
				}
			}
			before := f.state(t)
			r := httptest.NewRequest(http.MethodPost, "/api/v1/characters/"+f.sender.ID+"/actions", strings.NewReader(`{"action":"depot_withdraw","params":{"item_id":"`+rows[1].ID+`"}}`))
			r.Header.Set("Authorization", "Bearer session")
			r.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			f.gateway(t).ServeHTTP(w, r)
			if failSave {
				if w.Code != 500 || !strings.Contains(w.Body.String(), `"EXECUTION_FAILED"`) {
					t.Fatalf("save failure: %d %s", w.Code, w.Body.String())
				}
				if after := f.state(t); !reflect.DeepEqual(before, after) {
					t.Fatal("failed Gateway withdrawal changed assets or equipment")
				}
				return
			}
			if w.Code != 200 || !strings.Contains(w.Body.String(), `"success":true`) {
				t.Fatalf("Gateway withdrawal: %d %s", w.Code, w.Body.String())
			}
			f.assertWeaponSwapped(t, before, rows[0], rows[1])
		})
	}
}
