package shop_test

import (
	"context"
	"testing"

	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	"github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/shop"
)

func TestGetSaleOfferReturnsTheShopHand(t *testing.T) {
	svc, chars, invs, _, _, _ := setupParityTest(t)
	char := createTestCharacter(t, chars, "Hero", 0)
	weapon, _ := item.NewInstance("weapon-01", 1)
	herb, _ := item.NewInstance("item-001", 1)
	if err := invs.Save(context.Background(), coreinventory.Inventory{CharacterID: char.ID, Items: []item.Instance{weapon, herb}}); err != nil {
		t.Fatal(err)
	}
	for shopType, want := range map[shop.ShopType]*shop.SaleOffer{
		shop.ShopTypeWeapon:    {ItemInstanceID: weapon.ID, ItemDefinitionID: "weapon-01", Name: "Club", Payout: 25},
		shop.ShopTypeArmor:     nil,
		shop.ShopTypeItem:      {ItemInstanceID: herb.ID, ItemDefinitionID: "item-001", Name: "Herb", Payout: 5},
		shop.ShopTypeAccessory: {ItemInstanceID: herb.ID, ItemDefinitionID: "item-001", Name: "Herb", Payout: 5},
	} {
		got, err := svc.GetSaleOffer(context.Background(), shopType, char.ID)
		if err != nil {
			t.Fatal(err)
		}
		if (got == nil) != (want == nil) || (got != nil && *got != *want) {
			t.Errorf("%s offer = %+v, want %+v", shopType, got, want)
		}
	}
	if _, err := svc.GetSaleOffer(context.Background(), "bogus", char.ID); err != shop.ErrInvalidShopType {
		t.Fatalf("invalid shop type error = %v", err)
	}
}

func TestGetCatalogShowsNominalWeaponAndArmorStats(t *testing.T) {
	catalog, err := item.DefaultCatalog()
	if err != nil {
		t.Fatal(err)
	}
	chars := newCharacterRepoStub()
	char := createTestCharacter(t, chars, "Hero", 0)
	svc, err := shop.NewService(chars, newInventoryRepoStub(), catalog)
	if err != nil {
		t.Fatal(err)
	}
	for shopType, nominal := range map[shop.ShopType]func(string) (int, int){
		shop.ShopTypeWeapon: item.NominalWeaponStats,
		shop.ShopTypeArmor:  item.NominalArmorStats,
	} {
		got, err := svc.GetCatalog(context.Background(), shopType, char.ID)
		if err != nil {
			t.Fatal(err)
		}
		for _, it := range got.Items {
			power, weight := nominal(it.ID)
			if it.Power != power || it.Weight != weight || power == 0 {
				t.Errorf("%s %s stats = %d/%d, want nonzero %d/%d", shopType, it.ID, it.Power, it.Weight, power, weight)
			}
		}
	}
}
