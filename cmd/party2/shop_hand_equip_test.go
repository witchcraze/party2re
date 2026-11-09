package main

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"reflect"
	"testing"

	"github.com/witchcraze/party2re/internal/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreequipment "github.com/witchcraze/party2re/internal/core/equipment"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	"github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/depot"
	"github.com/witchcraze/party2re/internal/shop"
)

type zeroRandomSource struct{}

func (zeroRandomSource) Intn(_ int) (int, error) { return 0, nil }

type shopFixture struct {
	db   *sql.DB
	core *coreServices
	econ *econServices
	char corecharacter.Character
}

func newShopFixture(t *testing.T, money int) shopFixture {
	t.Helper()
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}
	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = db.Close()
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
	char, err := database.CreateTestCharacterWithFunds(ctx, db, "Shop hero", money)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.CreateTestDepot(ctx, db, char.ID, 0, nil); err != nil {
		t.Fatal(err)
	}
	return shopFixture{db: db, core: core, econ: econ, char: char}
}

type shopState struct {
	char      corecharacter.Character
	inventory coreinventory.Inventory
	equipment coreequipment.Equipment
	depot     depot.Depot
}

func (f shopFixture) state(t *testing.T) shopState {
	t.Helper()
	ctx := context.Background()
	var state shopState
	var err error
	state.char, err = f.core.charRepo.FindByID(ctx, f.char.ID)
	if err != nil {
		t.Fatal(err)
	}
	state.inventory, err = f.core.invRepo.FindByCharacterID(ctx, f.char.ID)
	if err != nil {
		t.Fatal(err)
	}
	state.equipment, err = f.econ.equipRepo.FindByCharacterID(ctx, f.char.ID)
	if err != nil {
		t.Fatal(err)
	}
	state.depot, err = f.econ.depotRepo.FindByCharacterID(ctx, f.char.ID)
	if err != nil {
		t.Fatal(err)
	}
	return state
}

func TestWireShopPurchaseEquipsWeapon_Integration(t *testing.T) {
	f := newShopFixture(t, 50000)
	ctx := context.Background()

	// Initial money is 50000, inventory empty
	res, err := f.econ.shop.PurchaseInShop(ctx, f.char.ID, shop.ShopTypeWeapon, "weapon-01", 1)
	if err != nil {
		t.Fatalf("Purchase(weapon-01) error = %v", err)
	}
	if res.TransferredToDepot {
		t.Fatal("expected weapon-01 to enter inventory, transferred to depot")
	}

	// Verify inventory_items in MariaDB
	inv, err := f.core.invRepo.FindByCharacterID(ctx, f.char.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(inv.Items) != 1 || inv.Items[0].DefinitionID != "weapon-01" {
		t.Fatalf("inventory = %+v, want weapon-01", inv.Items)
	}

	// Verify equipment_slots in MariaDB (FK link)
	eq, err := f.econ.equipRepo.FindByCharacterID(ctx, f.char.ID)
	if err != nil {
		t.Fatal(err)
	}
	eqInstID, ok := eq.Equipped(item.SlotMainHand)
	if !ok || eqInstID != inv.Items[0].ID {
		t.Fatalf("equipment_slots MainHand = %q, want %q", eqInstID, inv.Items[0].ID)
	}

	// Verify combat stats immediately reflect equipped weapon
	char, err := f.core.charRepo.FindByID(ctx, f.char.ID)
	if err != nil {
		t.Fatal(err)
	}
	expectedMoney := 50000 - res.TotalPrice
	if res.Character.Money != expectedMoney {
		t.Fatalf("res.Character.Money = %d, want %d", res.Character.Money, expectedMoney)
	}
	if char.Money != expectedMoney {
		t.Fatalf("MariaDB char.Money = %d, want %d (must not be reset)", char.Money, expectedMoney)
	}
	if char.Money != res.Character.Money {
		t.Fatalf("DB money %d != result money %d", char.Money, res.Character.Money)
	}
	stats := battle.CalculateEquipmentStats(char, inv, eq, zeroRandomSource{})
	if stats.Gear.WeaponID != "weapon-01" || stats.AttackBonus <= 0 {
		t.Fatalf("CalculateEquipmentStats Gear.WeaponID = %q, AttackBonus = %d", stats.Gear.WeaponID, stats.AttackBonus)
	}
}

func TestWireShopPurchaseClearsCustomizationAndPreservesDeductedMoney_Integration(t *testing.T) {
	f := newShopFixture(t, 50000)
	ctx := context.Background()

	// Set weapon and armor customization on character
	char, _ := f.core.charRepo.FindByID(ctx, f.char.ID)
	char.WeaponCustomName = "Customized Sword"
	char.WeaponSeal = 5
	char.ArmorCustomName = "Customized Armor"
	if err := f.core.charRepo.Update(ctx, char); err != nil {
		t.Fatal(err)
	}

	// Purchase weapon-01
	res, err := f.econ.shop.PurchaseInShop(ctx, f.char.ID, shop.ShopTypeWeapon, "weapon-01", 1)
	if err != nil {
		t.Fatalf("Purchase(weapon-01) error = %v", err)
	}

	expectedMoney := 50000 - res.TotalPrice
	if res.Character.Money != expectedMoney {
		t.Errorf("res.Character.Money = %d, want %d", res.Character.Money, expectedMoney)
	}

	// Verify in MariaDB: money is deducted AND weapon customization is cleared
	dbChar, err := f.core.charRepo.FindByID(ctx, f.char.ID)
	if err != nil {
		t.Fatal(err)
	}
	if dbChar.Money != expectedMoney {
		t.Fatalf("MariaDB char.Money was overwritten! got %d, want %d", dbChar.Money, expectedMoney)
	}
	if dbChar.WeaponCustomName != "" || dbChar.WeaponSeal != 0 {
		t.Errorf("weapon customization not cleared: name=%q seal=%d", dbChar.WeaponCustomName, dbChar.WeaponSeal)
	}
	if dbChar.ArmorCustomName != "Customized Armor" {
		t.Errorf("armor customization should be preserved: got %q", dbChar.ArmorCustomName)
	}
	if dbChar.Money != res.Character.Money {
		t.Errorf("DB money %d != res money %d", dbChar.Money, res.Character.Money)
	}
}

func TestWireShopPurchaseIndependentHandFields_Integration(t *testing.T) {
	f := newShopFixture(t, 50000)
	ctx := context.Background()

	// 1. Buy weapon-01 (enters kind 1, equips in MainHand)
	resWeapon, err := f.econ.shop.PurchaseInShop(ctx, f.char.ID, shop.ShopTypeWeapon, "weapon-01", 1)
	if err != nil {
		t.Fatalf("Purchase(weapon-01) error = %v", err)
	}
	if resWeapon.TransferredToDepot {
		t.Fatal("expected weapon-01 to enter inventory")
	}

	// 2. Buy item-001 (Herb, kind 3): weapon is held, but tool hand is empty!
	resHerb, err := f.econ.shop.PurchaseInShop(ctx, f.char.ID, shop.ShopTypeItem, "item-001", 1)
	if err != nil {
		t.Fatalf("Purchase(item-001) error = %v", err)
	}
	if resHerb.TransferredToDepot {
		t.Fatal("expected item-001 to enter empty tool hand, but transferred to depot")
	}

	// 3. Buy armor-01 (enters kind 2, equips in Body)
	resArmor, err := f.econ.shop.PurchaseInShop(ctx, f.char.ID, shop.ShopTypeArmor, "armor-01", 1)
	if err != nil {
		t.Fatalf("Purchase(armor-01) error = %v", err)
	}
	if resArmor.TransferredToDepot {
		t.Fatal("expected armor-01 to enter empty armor hand, but transferred to depot")
	}

	// Verify all 3 hand fields are held in MariaDB
	inv, err := f.core.invRepo.FindByCharacterID(ctx, f.char.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(inv.Items) != 3 {
		t.Fatalf("expected 3 items in inventory (1 per hand), got %d: %+v", len(inv.Items), inv.Items)
	}

	eq, err := f.econ.equipRepo.FindByCharacterID(ctx, f.char.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := eq.Equipped(item.SlotMainHand); !ok || got != resWeapon.ItemInstance.ID {
		t.Errorf("weapon not equipped in SlotMainHand: %q, want %q", got, resWeapon.ItemInstance.ID)
	}
	if got, ok := eq.Equipped(item.SlotBody); !ok || got != resArmor.ItemInstance.ID {
		t.Errorf("armor not equipped in SlotBody: %q, want %q", got, resArmor.ItemInstance.ID)
	}

	// 4. Now tool hand (kind 3) is occupied by item-001.
	// Buying another kind 3 item (item-007) must route to depot!
	resAntidote, err := f.econ.shop.PurchaseInShop(ctx, f.char.ID, shop.ShopTypeItem, "item-007", 1)
	if err != nil {
		t.Fatalf("Purchase(item-007) error = %v", err)
	}
	if !resAntidote.TransferredToDepot {
		t.Fatal("expected second kind-3 item to route to depot")
	}

	// Inventory must still have only 3 items
	invAfter, _ := f.core.invRepo.FindByCharacterID(ctx, f.char.ID)
	if len(invAfter.Items) != 3 {
		t.Fatalf("inventory count = %d, want 3", len(invAfter.Items))
	}

	// Depot must have item-007
	dep, err := f.econ.depotRepo.FindByCharacterID(ctx, f.char.ID)
	if err != nil {
		t.Fatal(err)
	}
	if dep.Quantity("item-007") != 1 {
		t.Fatalf("depot item-007 quantity = %d, want 1", dep.Quantity("item-007"))
	}
}

func TestWireShopPurchaseToolHandSharing_Integration(t *testing.T) {
	f := newShopFixture(t, 50000)
	ctx := context.Background()

	// Shops sell no shields (legacy @sales); an item-shop tool and an accessory share the tool hand.
	resHerb, err := f.econ.shop.PurchaseInShop(ctx, f.char.ID, shop.ShopTypeItem, "item-001", 1)
	if err != nil {
		t.Fatalf("Purchase(item-001) error = %v", err)
	}
	if resHerb.TransferredToDepot {
		t.Fatal("expected item-001 in the empty tool hand")
	}
	resAccessory, err := f.econ.shop.PurchaseInShop(ctx, f.char.ID, shop.ShopTypeAccessory, "item-147", 1)
	if err != nil {
		t.Fatalf("Purchase(item-147) error = %v", err)
	}
	if !resAccessory.TransferredToDepot {
		t.Fatal("expected accessory to route to depot when the tool hand is occupied")
	}
}

func TestWireShopPurchaseRollback_Integration(t *testing.T) {
	for _, stage := range []string{"equipment save"} {
		t.Run(stage, func(t *testing.T) {
			f := newShopFixture(t, 50000)
			ctx := context.Background()

			// Configure shop with failing equipment repository
			failingShop, err := shop.NewService(
				normalWriter(f.core),
				f.core.invRepo,
				f.core.itemCatalog,
				shop.WithTransactionProvider(f.core.txProvider),
				shop.WithDepotRepository(f.econ.depotRepo),
				shop.WithEquipmentRepository(depotEquipmentFailure{f.econ.equipRepo, stage}),
			)
			if err != nil {
				t.Fatal(err)
			}

			before := f.state(t)

			// Attempt purchase of weapon-01
			_, err = failingShop.PurchaseInShop(ctx, f.char.ID, shop.ShopTypeWeapon, "weapon-01", 1)
			if !errors.Is(err, errDepotEquipmentPersistence) {
				t.Fatalf("expected errDepotEquipmentPersistence, got %v", err)
			}

			// Verify MariaDB state is completely rolled back
			after := f.state(t)
			if !reflect.DeepEqual(before, after) {
				t.Fatalf("failed %s changed state:\nbefore=%+v\nafter=%+v", stage, before, after)
			}
		})
	}
}
