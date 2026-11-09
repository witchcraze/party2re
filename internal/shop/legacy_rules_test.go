package shop_test

import (
	"context"
	"errors"
	"testing"

	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	"github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/shop"
)

// Legacy kau walks only the job-level @sales list in every shop.
func TestPurchaseRejectsItemsOutsideTheSalesTier(t *testing.T) {
	svc, chars, invs, depots, _, _ := setupParityTest(t)
	char := createTestCharacter(t, chars, "Hero", 1000)
	for _, tc := range []struct {
		shopType shop.ShopType
		id       string
	}{
		{shop.ShopTypeWeapon, "weapon-07"}, // job level 1 and up
		{shop.ShopTypeItem, "weapon-01"},   // another shop's goods
		{shop.ShopTypeArmor, "item-001"},
	} {
		if _, err := svc.PurchaseInShop(context.Background(), char.ID, tc.shopType, tc.id, 1); !errors.Is(err, shop.ErrItemNotFound) {
			t.Fatalf("%s %s: err = %v", tc.shopType, tc.id, err)
		}
	}
	got, _ := chars.FindByID(context.Background(), char.ID)
	inv, _ := invs.FindByCharacterID(context.Background(), char.ID)
	if got.Money != 1000 || len(inv.Items) != 0 || len(depots.depots) != 0 {
		t.Fatalf("rejected purchase changed state: money=%d inv=%+v", got.Money, inv.Items)
	}
}

// Legacy get_helper_item aborts the action when the helper file cannot be read.
func TestPurchaseAbortsOnHelperReadFailure(t *testing.T) {
	svc, chars, invs, depots, helper, _ := setupParityTest(t)
	char := createTestCharacter(t, chars, "Hero", 1000)
	helper.err = errors.New("helper read failed")
	if _, err := svc.PurchaseInShop(context.Background(), char.ID, shop.ShopTypeWeapon, "weapon-01", 1); !errors.Is(err, helper.err) {
		t.Fatalf("single purchase err = %v", err)
	}
	items := []shop.BatchPurchaseItemRequest{{ItemDefinitionID: "weapon-01", Quantity: 1}}
	if _, err := svc.BatchPurchase(context.Background(), char.ID, shop.ShopTypeWeapon, items); !errors.Is(err, helper.err) {
		t.Fatalf("batch purchase err = %v", err)
	}
	got, _ := chars.FindByID(context.Background(), char.ID)
	inv, _ := invs.FindByCharacterID(context.Background(), char.ID)
	if got.Money != 1000 || len(inv.Items) != 0 || len(depots.depots) != 0 {
		t.Fatalf("aborted purchase changed state: money=%d inv=%+v", got.Money, inv.Items)
	}
}

// Legacy uru sells only $m{wea}, $m{arm} or $m{ite}, depending on the shop.
func TestSellOnlyFromTheShopHand(t *testing.T) {
	svc, chars, invs, _, _, _ := setupParityTest(t)
	char := createTestCharacter(t, chars, "Hero", 0)
	weapon, _ := item.NewInstance("weapon-01", 1)
	herb, _ := item.NewInstance("item-001", 1)
	if err := invs.Save(context.Background(), coreinventory.Inventory{CharacterID: char.ID, Items: []item.Instance{weapon, herb}}); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		shopType shop.ShopType
		id       string
	}{
		{shop.ShopTypeItem, weapon.ID},
		{shop.ShopTypeAccessory, weapon.ID},
		{shop.ShopTypeArmor, weapon.ID},
		{shop.ShopTypeWeapon, herb.ID},
	} {
		if _, err := svc.Sell(context.Background(), char.ID, tc.shopType, tc.id, 1); !errors.Is(err, shop.ErrNotInShopHand) {
			t.Fatalf("%s %s: err = %v", tc.shopType, tc.id, err)
		}
	}
	if got, _ := chars.FindByID(context.Background(), char.ID); got.Money != 0 {
		t.Fatalf("rejected sale paid %d", got.Money)
	}
	for shopType, id := range map[shop.ShopType]string{shop.ShopTypeWeapon: weapon.ID, shop.ShopTypeAccessory: herb.ID} {
		if _, err := svc.Sell(context.Background(), char.ID, shopType, id, 1); err != nil {
			t.Fatalf("%s sale: %v", shopType, err)
		}
	}
	if got, _ := chars.FindByID(context.Background(), char.ID); got.Money != 25+5 {
		t.Fatalf("hand sales paid %d, want 30", got.Money)
	}
}
