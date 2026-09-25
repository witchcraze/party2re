package shop_test

import (
	"context"
	"errors"
	"math"
	"testing"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	"github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/depot"
	"github.com/witchcraze/party2re/internal/shop"
)

func setupAccessoryTestService(t *testing.T) (*shop.Service, *characterRepoStub, *inventoryRepoStub, *depotRepoStub, *mockCollectionRecorder) {
	t.Helper()
	cat, err := item.InitialCatalog()
	if err != nil {
		t.Fatalf("failed to init catalog: %v", err)
	}

	charRepo := newCharacterRepoStub()
	invRepo := newInventoryRepoStub()
	depotRepo := newDepotRepoStub()
	recorder := &mockCollectionRecorder{}

	svc, err := shop.NewService(
		charRepo,
		invRepo,
		cat,
		shop.WithDepotRepository(depotRepo),
		shop.WithCollectionRecorder(recorder),
	)
	if err != nil {
		t.Fatalf("failed to create shop service: %v", err)
	}
	return svc, charRepo, invRepo, depotRepo, recorder
}

func TestAccessoryPricing(t *testing.T) {
	svc, _, _, _, _ := setupAccessoryTestService(t)

	// Normal accessory item: 10x base price
	price147, err := svc.CalculateRetailPriceForShop(shop.ShopTypeAccessory, "item-147", 50)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if price147 != 500 {
		t.Errorf("item-147 price = %d, want 500 (50 * 10)", price147)
	}

	// Rare items (item-150 and item-151): 1000x base price
	price150, err := svc.CalculateRetailPriceForShop(shop.ShopTypeAccessory, "item-150", 150)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if price150 != 150000 {
		t.Errorf("item-150 price = %d, want 150000 (150 * 1000)", price150)
	}

	price151, err := svc.CalculateRetailPriceForShop(shop.ShopTypeAccessory, "item-151", 50)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if price151 != 50000 {
		t.Errorf("item-151 price = %d, want 50000 (50 * 1000)", price151)
	}

	// Overflow guard test
	_, err = svc.CalculateRetailPriceForShop(shop.ShopTypeAccessory, "item-150", math.MaxInt/500)
	if err == nil {
		t.Errorf("expected overflow error for large price, got nil")
	}
}

func TestAccessoryPurchase_InventoryRoutingAndDiscovery(t *testing.T) {
	ctx := context.Background()
	svc, charRepo, invRepo, _, recorder := setupAccessoryTestService(t)

	char := corecharacter.Character{
		ID:       "char-acc-1",
		Name:     "アリス",
		Money:    10000,
		JobLevel: 100,
	}
	_ = charRepo.Update(ctx, char)
	_ = invRepo.Save(ctx, coreinventory.Inventory{CharacterID: char.ID})

	// Purchase item-147 (base price 50, retail 500 G in accessory shop)
	res, err := svc.PurchaseInShop(ctx, char.ID, shop.ShopTypeAccessory, "item-147", 1)
	if err != nil {
		t.Fatalf("PurchaseInShop error = %v", err)
	}

	if res.TotalPrice != 500 {
		t.Errorf("res.TotalPrice = %d, want 500", res.TotalPrice)
	}
	if res.TransferredToDepot {
		t.Errorf("expected item to be in inventory, but transferred to depot")
	}
	if res.Character.Money != 9500 {
		t.Errorf("character money = %d, want 9500", res.Character.Money)
	}
	// Verify collection recorder was triggered
	if len(recorder.recorded) != 1 || recorder.recorded[0] != "item-147" {
		t.Errorf("recorded discovered items = %v, want [item-147]", recorder.recorded)
	}

	// Purchase second accessory item when accessory slot is occupied -> routes to depot
	res2, err := svc.PurchaseInShop(ctx, char.ID, shop.ShopTypeAccessory, "item-148", 1)
	if err != nil {
		t.Fatalf("PurchaseInShop 2 error = %v", err)
	}
	if !res2.TransferredToDepot {
		t.Errorf("expected second accessory to be transferred to depot")
	}
}

func TestAccessoryPurchase_JobLevelFilter(t *testing.T) {
	ctx := context.Background()
	svc, charRepo, invRepo, _, _ := setupAccessoryTestService(t)

	// Character with job_lv = 10 (less than 50)
	char := corecharacter.Character{
		ID:       "char-low-lv",
		Name:     "ボブ",
		Money:    1000000,
		JobLevel: 10,
	}
	_ = charRepo.Update(ctx, char)
	_ = invRepo.Save(ctx, coreinventory.Inventory{CharacterID: char.ID})

	// item-143 is only available at job_lv >= 100
	_, err := svc.PurchaseInShop(ctx, char.ID, shop.ShopTypeAccessory, "item-143", 1)
	if !errors.Is(err, shop.ErrItemNotFound) {
		t.Errorf("expected ErrItemNotFound for item-143 at job_lv 10, got %v", err)
	}

	// item-147 is available at job_lv < 50
	_, err = svc.PurchaseInShop(ctx, char.ID, shop.ShopTypeAccessory, "item-147", 1)
	if err != nil {
		t.Errorf("unexpected error purchasing item-147 at job_lv 10: %v", err)
	}
}

func TestAccessorySynthesis_WithHiyakuGuarantee(t *testing.T) {
	ctx := context.Background()
	svc, charRepo, invRepo, depotRepo, _ := setupAccessoryTestService(t)

	char := corecharacter.Character{
		ID:       "char-synth-1",
		Name:     "クララ",
		Money:    5000,
		JobLevel: 50,
	}
	_ = charRepo.Update(ctx, char)

	// Put item-180 (合成の秘薬) in character inventory
	hiyaku, _ := item.NewInstance("item-180", 1)
	_ = invRepo.Save(ctx, coreinventory.Inventory{
		CharacterID: char.ID,
		Items:       []item.Instance{hiyaku},
	})

	// Put materials for recipe-01 ("命の宝珠": item-016 命の木の実, item-064 イエローオーブ) in depot
	dep, _ := depot.NewDepotWithCapacity(char.ID, 50, 0, 0)
	mat1, _ := item.NewInstance("item-016", 1)
	mat2, _ := item.NewInstance("item-064", 1)
	_ = dep.AddItem(mat1)
	_ = dep.AddItem(mat2)
	_ = depotRepo.Save(ctx, dep)

	// Execute synthesis with recipe "命の宝珠"
	res, err := svc.Synthesize(ctx, char.ID, "命の宝珠")
	if err != nil {
		t.Fatalf("Synthesize error = %v", err)
	}

	if !res.Success {
		t.Fatalf("expected success with hiyaku guarantee, got failure")
	}
	if !res.UsedHiyaku {
		t.Errorf("expected UsedHiyaku = true")
	}
	if res.CreatedItem == nil || res.CreatedItem.DefinitionID != "item-152" {
		t.Errorf("expected CreatedItem = item-152, got %+v", res.CreatedItem)
	}
	if len(res.ConsumedMaterials) != 2 {
		t.Errorf("expected 2 consumed materials, got %d", len(res.ConsumedMaterials))
	}

	// Verify inventory no longer has item-180
	updatedInv, _ := invRepo.FindByCharacterID(ctx, char.ID)
	for _, inst := range updatedInv.Items {
		if inst.DefinitionID == "item-180" {
			t.Errorf("expected item-180 to be consumed from inventory")
		}
	}

	// Verify depot has the created item and materials removed
	updatedDepot, _ := depotRepo.FindByCharacterID(ctx, char.ID)
	foundProduct := false
	for _, inst := range updatedDepot.Items {
		if inst.DefinitionID == "item-152" {
			foundProduct = true
		}
		if inst.DefinitionID == "item-016" || inst.DefinitionID == "item-064" {
			t.Errorf("material %s should have been consumed from depot", inst.DefinitionID)
		}
	}
	if !foundProduct {
		t.Errorf("created product item-152 not found in depot")
	}
}

func TestAccessorySynthesis_MissingMaterials(t *testing.T) {
	ctx := context.Background()
	svc, charRepo, invRepo, depotRepo, _ := setupAccessoryTestService(t)

	char := corecharacter.Character{
		ID:       "char-synth-missing",
		Name:     "デイビッド",
		Money:    5000,
		JobLevel: 50,
	}
	_ = charRepo.Update(ctx, char)
	_ = invRepo.Save(ctx, coreinventory.Inventory{CharacterID: char.ID})

	// Depot only has 1 material, missing the second
	dep, _ := depot.NewDepotWithCapacity(char.ID, 50, 0, 0)
	mat1, _ := item.NewInstance("item-016", 1)
	_ = dep.AddItem(mat1)
	_ = depotRepo.Save(ctx, dep)

	_, err := svc.Synthesize(ctx, char.ID, "命の宝珠")
	if !errors.Is(err, shop.ErrMaterialsNotFound) {
		t.Errorf("expected ErrMaterialsNotFound, got %v", err)
	}

	// Verify material was NOT consumed when transaction failed
	updatedDep, _ := depotRepo.FindByCharacterID(ctx, char.ID)
	if len(updatedDep.Items) != 1 {
		t.Errorf("depot item count = %d, want 1", len(updatedDep.Items))
	}
}

func TestAccessorySynthesis_UnknownRecipe(t *testing.T) {
	ctx := context.Background()
	svc, charRepo, invRepo, _, _ := setupAccessoryTestService(t)

	char := corecharacter.Character{ID: "char-synth-unk"}
	_ = charRepo.Update(ctx, char)
	_ = invRepo.Save(ctx, coreinventory.Inventory{CharacterID: char.ID})

	_, err := svc.Synthesize(ctx, char.ID, "架空のアクセサリー")
	if !errors.Is(err, shop.ErrRecipeNotFound) {
		t.Errorf("expected ErrRecipeNotFound, got %v", err)
	}
}

func TestAccessorySynthesis_RateBased(t *testing.T) {
	ctx := context.Background()
	svc, charRepo, invRepo, depotRepo, _ := setupAccessoryTestService(t)

	char := corecharacter.Character{
		ID:       "char-synth-rate",
		Name:     "エレン",
		Money:    5000,
		JobLevel: 50,
	}
	_ = charRepo.Update(ctx, char)
	// No hiyaku in inventory
	_ = invRepo.Save(ctx, coreinventory.Inventory{CharacterID: char.ID})

	// Add materials for recipe-07 ("幸せのブローチ", 99% success rate)
	dep, _ := depot.NewDepotWithCapacity(char.ID, 50, 0, 0)
	mat1, _ := item.NewInstance("item-022", 1) // 幸せの種
	mat2, _ := item.NewInstance("item-136", 1) // オリハルコン
	_ = dep.AddItem(mat1)
	_ = dep.AddItem(mat2)
	_ = depotRepo.Save(ctx, dep)

	res, err := svc.Synthesize(ctx, char.ID, "幸せのブローチ")
	if err != nil {
		t.Fatalf("Synthesize error = %v", err)
	}

	if res.UsedHiyaku {
		t.Errorf("expected UsedHiyaku = false")
	}
	if len(res.ConsumedMaterials) != 2 {
		t.Errorf("expected 2 consumed materials, got %d", len(res.ConsumedMaterials))
	}

	// Recipe has 99% success rate, should almost certainly succeed in a normal roll
	if res.Success {
		if res.CreatedItem == nil || res.CreatedItem.DefinitionID != "item-167" {
			t.Errorf("expected created item = item-167, got %+v", res.CreatedItem)
		}
	} else {
		if res.CreatedItem != nil {
			t.Errorf("expected created item = nil on failure")
		}
	}
}
