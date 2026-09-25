package shop_test

import (
	"testing"

	"github.com/witchcraze/party2re/internal/shop"
)

func TestGetSalesItemIDs_Weapon(t *testing.T) {
	// Level 0: 1..5, 43, 6+0 = 6
	ids0, err := shop.GetSalesItemIDs(shop.ShopTypeWeapon, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected0 := []string{"weapon-01", "weapon-02", "weapon-03", "weapon-04", "weapon-05", "weapon-43", "weapon-06"}
	if len(ids0) != len(expected0) {
		t.Fatalf("job_lv 0 weapon count = %d, want %d", len(ids0), len(expected0))
	}
	for i, id := range ids0 {
		if id != expected0[i] {
			t.Errorf("job_lv 0 weapon[%d] = %s, want %s", i, id, expected0[i])
		}
	}

	// Level 1: 1..5, 43, 6+1 = 7
	ids1, err := shop.GetSalesItemIDs(shop.ShopTypeWeapon, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected1 := []string{"weapon-01", "weapon-02", "weapon-03", "weapon-04", "weapon-05", "weapon-43", "weapon-07"}
	for i, id := range ids1 {
		if id != expected1[i] {
			t.Errorf("job_lv 1 weapon[%d] = %s, want %s", i, id, expected1[i])
		}
	}

	// Level 12 (job_lv > 11): (1..5, 43, 6..16) -> 17 items total
	ids12, err := shop.GetSalesItemIDs(shop.ShopTypeWeapon, 12)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ids12) != 17 {
		t.Fatalf("job_lv 12 weapon count = %d, want 17", len(ids12))
	}
	expected12 := []string{
		"weapon-01", "weapon-02", "weapon-03", "weapon-04", "weapon-05", "weapon-43",
		"weapon-06", "weapon-07", "weapon-08", "weapon-09", "weapon-10",
		"weapon-11", "weapon-12", "weapon-13", "weapon-14", "weapon-15", "weapon-16",
	}
	for i, id := range ids12 {
		if id != expected12[i] {
			t.Errorf("job_lv 12 weapon[%d] = %s, want %s", i, id, expected12[i])
		}
	}
}

func TestGetSalesItemIDs_Armor(t *testing.T) {
	// Level 0: 1..(5+0) = 1..5
	ids0, err := shop.GetSalesItemIDs(shop.ShopTypeArmor, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ids0) != 5 {
		t.Fatalf("job_lv 0 armor count = %d, want 5", len(ids0))
	}
	if ids0[0] != "armor-01" || ids0[4] != "armor-05" {
		t.Errorf("job_lv 0 armor range mismatch: %v", ids0)
	}

	// Level 5: 1..(5+5) = 1..10
	ids5, err := shop.GetSalesItemIDs(shop.ShopTypeArmor, 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ids5) != 10 {
		t.Fatalf("job_lv 5 armor count = %d, want 10", len(ids5))
	}

	// Level 12 (job_lv > 11): 1..16
	ids12, err := shop.GetSalesItemIDs(shop.ShopTypeArmor, 12)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ids12) != 16 {
		t.Fatalf("job_lv 12 armor count = %d, want 16", len(ids12))
	}
	if ids12[15] != "armor-16" {
		t.Errorf("last armor = %s, want armor-16", ids12[15])
	}
}

func TestGetSalesItemIDs_Item(t *testing.T) {
	// Level 0: (1, 7, 8, 9, 127) -> 5 items
	ids0, err := shop.GetSalesItemIDs(shop.ShopTypeItem, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected0 := []string{"item-001", "item-007", "item-008", "item-009", "item-127"}
	if len(ids0) != len(expected0) {
		t.Fatalf("job_lv 0 item count = %d, want %d", len(ids0), len(expected0))
	}
	for i, id := range ids0 {
		if id != expected0[i] {
			t.Errorf("job_lv 0 item[%d] = %s, want %s", i, id, expected0[i])
		}
	}

	// Level 1: (1, 2, 7, 8, 9, 11, 14, 79, 127) -> 9 items
	ids1, err := shop.GetSalesItemIDs(shop.ShopTypeItem, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ids1) != 9 {
		t.Fatalf("job_lv 1 item count = %d, want 9", len(ids1))
	}

	// Level 3: (1, 2, 7, 8, 9, 11, 14, 79, 41, 42, 127) -> 11 items
	ids3, err := shop.GetSalesItemIDs(shop.ShopTypeItem, 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ids3) != 11 {
		t.Fatalf("job_lv 3 item count = %d, want 11", len(ids3))
	}

	// Level 5: adds 3, 76, 101 -> 14 items
	ids5, err := shop.GetSalesItemIDs(shop.ShopTypeItem, 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ids5) != 14 {
		t.Fatalf("job_lv 5 item count = %d, want 14", len(ids5))
	}

	// Level 7: adds 102 -> 15 items
	ids7, err := shop.GetSalesItemIDs(shop.ShopTypeItem, 7)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ids7) != 15 {
		t.Fatalf("job_lv 7 item count = %d, want 15", len(ids7))
	}
}

func TestGetSalesItemIDs_InvalidShopType(t *testing.T) {
	_, err := shop.GetSalesItemIDs(shop.ShopType("unknown"), 0)
	if err != shop.ErrInvalidShopType {
		t.Errorf("expected ErrInvalidShopType, got %v", err)
	}
}

func TestGetSalesItemIDs_Accessory(t *testing.T) {
	// Level 0 (< 50): 147, 148, 158, 169, 175, 222 (6 items)
	ids0, err := shop.GetSalesItemIDs(shop.ShopTypeAccessory, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected0 := []string{"item-147", "item-148", "item-158", "item-169", "item-175", "item-222"}
	if len(ids0) != len(expected0) {
		t.Fatalf("job_lv 0 accessory count = %d, want %d", len(ids0), len(expected0))
	}
	for i, id := range ids0 {
		if id != expected0[i] {
			t.Errorf("job_lv 0 accessory[%d] = %s, want %s", i, id, expected0[i])
		}
	}

	// Level 50 (>= 50 && < 100): 144..146, 147, 148, 158, 169, 175, 218..220, 222, 223, 225, 226, 228..230, 248 (18 items)
	ids50, err := shop.GetSalesItemIDs(shop.ShopTypeAccessory, 50)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected50 := []string{
		"item-144", "item-145", "item-146", "item-147", "item-148", "item-158", "item-169", "item-175",
		"item-218", "item-219", "item-220", "item-222", "item-223", "item-225", "item-226", "item-228",
		"item-229", "item-230", "item-248",
	}
	if len(ids50) != len(expected50) {
		t.Fatalf("job_lv 50 accessory count = %d, want %d", len(ids50), len(expected50))
	}
	for i, id := range ids50 {
		if id != expected50[i] {
			t.Errorf("job_lv 50 accessory[%d] = %s, want %s", i, id, expected50[i])
		}
	}

	// Level 100 (>= 100): 143..151, 158, 169, 175, 218..220, 222, 223, 225, 226, 228..230, 181, 182, 189, 190, 248 (28 items)
	ids100, err := shop.GetSalesItemIDs(shop.ShopTypeAccessory, 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected100 := []string{
		"item-143", "item-144", "item-145", "item-146", "item-147", "item-148", "item-149", "item-150", "item-151",
		"item-158", "item-169", "item-175",
		"item-218", "item-219", "item-220", "item-222", "item-223", "item-225", "item-226", "item-228", "item-229", "item-230",
		"item-181", "item-182", "item-189", "item-190", "item-248",
	}
	if len(ids100) != len(expected100) {
		t.Fatalf("job_lv 100 accessory count = %d, want %d", len(ids100), len(expected100))
	}
	for i, id := range ids100 {
		if id != expected100[i] {
			t.Errorf("job_lv 100 accessory[%d] = %s, want %s", i, id, expected100[i])
		}
	}
}

func TestAccessoryShopMetaAndDialogue(t *testing.T) {
	if !shop.ValidateShopType(shop.ShopTypeAccessory) {
		t.Errorf("ValidateShopType(ShopTypeAccessory) = false, want true")
	}

	title, npc := shop.GetShopMeta(shop.ShopTypeAccessory)
	if title != "アクセサリー屋" || npc != "@ミラ" {
		t.Errorf("GetShopMeta(ShopTypeAccessory) = (%q, %q), want (アクセサリー屋, @ミラ)", title, npc)
	}

	dialogue, hint := shop.GetInspectDialogue(shop.ShopTypeAccessory)
	if dialogue != "なにか私についてる？" || hint != "" {
		t.Errorf("GetInspectDialogue(ShopTypeAccessory) = (%q, %q), want (なにか私についてる？, \"\")", dialogue, hint)
	}

	words := shop.GetShopWords(shop.ShopTypeAccessory)
	if len(words) == 0 {
		t.Errorf("GetShopWords(ShopTypeAccessory) is empty")
	}
}
