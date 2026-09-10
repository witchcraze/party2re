package shop_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/depot"
	"github.com/witchcraze/party2re/internal/shop"
)

type mockHelperProvider struct {
	activeIDs []string
}

func (m *mockHelperProvider) GetActiveHelperItemIDs(_ context.Context, _ time.Time) ([]string, error) {
	return m.activeIDs, nil
}

type mockCollectionRecorder struct {
	recorded []string
}

func (m *mockCollectionRecorder) RecordItemDiscovered(_ context.Context, charID, itemID, name, cat string) error {
	m.recorded = append(m.recorded, itemID)
	return nil
}

func setupParityTest(t *testing.T) (*shop.Service, *characterRepoStub, *inventoryRepoStub, *depotRepoStub, *mockHelperProvider, *mockCollectionRecorder) {
	t.Helper()
	w1, _ := item.NewEquipmentDefinition("weapon-01", "Club", 50, item.SlotMainHand)
	w2, _ := item.NewEquipmentDefinition("weapon-02", "Copper Sword", 120, item.SlotMainHand)
	a1, _ := item.NewEquipmentDefinition("armor-01", "Plain Clothes", 30, item.SlotBody)
	i1, _ := item.NewDefinition("item-001", "Herb", 10)
	i2, _ := item.NewDefinition("item-007", "Antidote", 15)

	catalog, err := item.NewCatalog([]item.Definition{w1, w2, a1, i1, i2})
	if err != nil {
		t.Fatal(err)
	}

	charRepo := newCharacterRepoStub()
	invRepo := newInventoryRepoStub()
	depotRepo := newDepotRepoStub()
	helperMock := &mockHelperProvider{}
	recorderMock := &mockCollectionRecorder{}

	svc, err := shop.NewService(
		charRepo,
		invRepo,
		catalog,
		shop.WithDepotRepository(depotRepo),
		shop.WithHelperProvider(helperMock),
		shop.WithCollectionRecorder(recorderMock),
		shop.WithTransactionProvider(&mockTxProvider{}),
	)
	if err != nil {
		t.Fatal(err)
	}

	return svc, charRepo, invRepo, depotRepo, helperMock, recorderMock
}

func TestCalculateRetailPrice(t *testing.T) {
	svc, _, _, _, _, _ := setupParityTest(t)

	price, err := svc.CalculateRetailPrice(100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if price != 200 {
		t.Errorf("retail price = %d, want 200", price)
	}

	// Non-positive price
	zeroPrice, err := svc.CalculateRetailPrice(0)
	if err != nil || zeroPrice != 0 {
		t.Errorf("zero price = %d, err = %v", zeroPrice, err)
	}
}

func TestPurchase_SlotOccupied_AutoTransferToDepot(t *testing.T) {
	svc, charRepo, invRepo, depotRepo, _, recorder := setupParityTest(t)
	char := createTestCharacter(t, charRepo, "Fighter", 1000)

	// 1. First weapon purchase -> empty inventory -> goes to inventory
	res1, err := svc.Purchase(context.Background(), char.ID, "weapon-01", 1)
	if err != nil {
		t.Fatalf("first purchase error: %v", err)
	}
	if res1.TransferredToDepot {
		t.Errorf("expected first weapon to be placed in inventory")
	}
	if res1.TotalPrice != 100 { // 50 * 2
		t.Errorf("res1 TotalPrice = %d, want 100", res1.TotalPrice)
	}
	if len(recorder.recorded) != 1 || recorder.recorded[0] != "weapon-01" {
		t.Errorf("expected weapon-01 to be recorded to collection, got %v", recorder.recorded)
	}

	inv, _ := invRepo.FindByCharacterID(context.Background(), char.ID)
	if len(inv.Items) != 1 || inv.Items[0].DefinitionID != "weapon-01" {
		t.Fatalf("expected 1 weapon in inventory, got %#v", inv.Items)
	}

	// 2. Second weapon purchase -> slot occupied -> auto-transfers to depot
	res2, err := svc.Purchase(context.Background(), char.ID, "weapon-02", 1)
	if err != nil {
		t.Fatalf("second purchase error: %v", err)
	}
	if !res2.TransferredToDepot {
		t.Errorf("expected second weapon to be auto-transferred to depot")
	}
	if res2.TotalPrice != 240 { // 120 * 2
		t.Errorf("res2 TotalPrice = %d, want 240", res2.TotalPrice)
	}

	// Inventory must still have only 1 item
	invAfter, _ := invRepo.FindByCharacterID(context.Background(), char.ID)
	if len(invAfter.Items) != 1 {
		t.Errorf("inventory item count = %d, want 1", len(invAfter.Items))
	}

	// Depot must have the second weapon
	dep, err := depotRepo.FindByCharacterID(context.Background(), char.ID)
	if err != nil {
		t.Fatalf("depot not found: %v", err)
	}
	if len(dep.Items) != 1 || dep.Items[0].DefinitionID != "weapon-02" {
		t.Errorf("depot items mismatch: %#v", dep.Items)
	}

	// Character money: 1000 - 100 - 240 = 660
	finalChar, _ := charRepo.FindByID(context.Background(), char.ID)
	if finalChar.Money != 660 {
		t.Errorf("finalChar.Money = %d, want 660", finalChar.Money)
	}
}

func TestPurchase_DepotFull(t *testing.T) {
	svc, charRepo, invRepo, depotRepo, _, _ := setupParityTest(t)
	char := createTestCharacter(t, charRepo, "Hero", 1000)

	// Pre-occupy weapon slot in inventory
	inv, _ := invRepo.FindByCharacterID(context.Background(), char.ID)
	w1Inst, _ := item.NewInstance("weapon-01", 1)
	_ = inv.Add(w1Inst)
	_ = invRepo.Save(context.Background(), inv)

	// Create a depot and fill it to capacity (5)
	dep, _ := depot.NewDepot(char.ID)
	for i := 1; i <= 5; i++ {
		inst, _ := item.NewInstance("dummy_item", 1)
		inst.ID = "dummy-inst-" + string(rune('a'+i))
		inst.DefinitionID = "dummy-def-" + string(rune('a'+i))
		dep.Items = append(dep.Items, inst)
	}
	_ = depotRepo.Save(context.Background(), dep)

	// Attempt to purchase weapon-02 (will route to depot, but depot is full)
	_, err := svc.Purchase(context.Background(), char.ID, "weapon-02", 1)
	if !errors.Is(err, shop.ErrDepotFull) {
		t.Fatalf("expected ErrDepotFull, got %v", err)
	}

	// Ensure atomic rollback: money must remain untouched
	charAfter, _ := charRepo.FindByID(context.Background(), char.ID)
	if charAfter.Money != 1000 {
		t.Errorf("charAfter.Money = %d, want 1000 (atomic rollback)", charAfter.Money)
	}
}

func TestPurchase_ActiveHelperExclusion(t *testing.T) {
	svc, charRepo, _, _, helperMock, _ := setupParityTest(t)
	char := createTestCharacter(t, charRepo, "Hero", 1000)

	// Mark weapon-01 as active in a helper quest
	helperMock.activeIDs = []string{"weapon-01"}

	// Attempt to purchase weapon-01 -> rejected with ErrItemUnavailable
	_, err := svc.Purchase(context.Background(), char.ID, "weapon-01", 1)
	if !errors.Is(err, shop.ErrItemUnavailable) {
		t.Fatalf("expected ErrItemUnavailable, got %v", err)
	}

	// Purchase weapon-02 (not in helper) -> succeeds
	res, err := svc.Purchase(context.Background(), char.ID, "weapon-02", 1)
	if err != nil {
		t.Fatalf("weapon-02 purchase error: %v", err)
	}
	if res.TotalPrice != 240 {
		t.Errorf("TotalPrice = %d, want 240", res.TotalPrice)
	}
}

func TestGetCatalog_JobLevelProgression_And_HelperExclusion(t *testing.T) {
	svc, charRepo, _, _, helperMock, _ := setupParityTest(t)
	char := createTestCharacter(t, charRepo, "Hero", 1000)
	char.JobLevel = 0
	_ = charRepo.Update(context.Background(), char)

	// Helper has item-001
	helperMock.activeIDs = []string{"item-001"}

	catalog, err := svc.GetCatalog(context.Background(), shop.ShopTypeItem, char.ID)
	if err != nil {
		t.Fatalf("GetCatalog error: %v", err)
	}

	if catalog.ShopType != shop.ShopTypeItem {
		t.Errorf("ShopType = %s, want item", catalog.ShopType)
	}
	if catalog.NPCName != "@アイテムコ" {
		t.Errorf("NPCName = %s, want @アイテムコ", catalog.NPCName)
	}

	// For job_lv 0, item shop sells: item-001, item-007, item-008, item-009, item-127
	// But item-001 is excluded by helper!
	// And only item-007 exists in our test definition provider.
	for _, it := range catalog.Items {
		if it.ID == "item-001" {
			t.Errorf("item-001 should have been excluded by active helper quest")
		}
		if it.ID == "item-007" {
			if it.RetailPrice != 30 { // 15 base * 2 = 30
				t.Errorf("item-007 retail price = %d, want 30", it.RetailPrice)
			}
		}
	}
}

func TestBatchPurchase_DirectToDepot(t *testing.T) {
	svc, charRepo, invRepo, depotRepo, _, _ := setupParityTest(t)
	char := createTestCharacter(t, charRepo, "Hero", 2000)

	req := []shop.BatchPurchaseItemRequest{
		{ItemDefinitionID: "weapon-01", Quantity: 2}, // 50 * 2 * 2 = 200
		{ItemDefinitionID: "item-007", Quantity: 5},  // 15 * 2 * 5 = 150
	}

	result, err := svc.BatchPurchase(context.Background(), char.ID, shop.ShopTypeWeapon, req)
	if err != nil {
		t.Fatalf("BatchPurchase error: %v", err)
	}

	if result.TotalPrice != 350 {
		t.Errorf("result.TotalPrice = %d, want 350", result.TotalPrice)
	}

	// Inventory must be untouched (all to depot)
	inv, _ := invRepo.FindByCharacterID(context.Background(), char.ID)
	if len(inv.Items) != 0 {
		t.Errorf("inventory should have remained empty, got %d items", len(inv.Items))
	}

	// Depot must have both items
	dep, err := depotRepo.FindByCharacterID(context.Background(), char.ID)
	if err != nil {
		t.Fatalf("depot not found: %v", err)
	}
	if len(dep.Items) != 2 {
		t.Fatalf("depot items count = %d, want 2", len(dep.Items))
	}

	// Money: 2000 - 350 = 1650
	savedChar, _ := charRepo.FindByID(context.Background(), char.ID)
	if savedChar.Money != 1650 {
		t.Errorf("savedChar.Money = %d, want 1650", savedChar.Money)
	}
}

func TestBatchPurchase_DepotFull(t *testing.T) {
	svc, charRepo, _, depotRepo, _, _ := setupParityTest(t)
	char := createTestCharacter(t, charRepo, "Hero", 2000)

	// Depot filled with 4 items (capacity is 5)
	dep, _ := depot.NewDepot(char.ID)
	for i := 1; i <= 4; i++ {
		inst, _ := item.NewInstance("dummy_item", 1)
		inst.ID = "dummy-batch-" + string(rune('a'+i))
		inst.DefinitionID = "dummy-batch-def-" + string(rune('a'+i))
		dep.Items = append(dep.Items, inst)
	}
	_ = depotRepo.Save(context.Background(), dep)

	req := []shop.BatchPurchaseItemRequest{
		{ItemDefinitionID: "weapon-01", Quantity: 1},
		{ItemDefinitionID: "weapon-02", Quantity: 1},
	}

	// 2 items cannot fit into depot with 4 items (limit 5)
	_, err := svc.BatchPurchase(context.Background(), char.ID, shop.ShopTypeWeapon, req)
	if !errors.Is(err, shop.ErrDepotFull) {
		t.Fatalf("expected ErrDepotFull, got %v", err)
	}

	// Rollback check: money unchanged
	savedChar, _ := charRepo.FindByID(context.Background(), char.ID)
	if savedChar.Money != 2000 {
		t.Errorf("savedChar.Money = %d, want 2000 (atomic rollback)", savedChar.Money)
	}
}

func TestNPC_Inspect_And_Talk(t *testing.T) {
	svc, _, _, _, _, _ := setupParityTest(t)

	// Weapon shop inspect
	resW, err := svc.InspectNPC(context.Background(), shop.ShopTypeWeapon, "c1")
	if err != nil {
		t.Fatalf("InspectNPC weapon error: %v", err)
	}
	if resW.Dialogue != "おいおい、俺は武器じゃねぇぜ" {
		t.Errorf("weapon inspect dialogue mismatch: %s", resW.Dialogue)
	}

	// Item shop inspect (has secret shop hint)
	resI, err := svc.InspectNPC(context.Background(), shop.ShopTypeItem, "c1")
	if err != nil {
		t.Fatalf("InspectNPC item error: %v", err)
	}
	if resI.Dialogue != "ほえ？なんでしょうかぁ？" {
		t.Errorf("item inspect dialogue mismatch: %s", resI.Dialogue)
	}
	if resI.SecretShopHint != "＠ひみつのみせ に行きたい" {
		t.Errorf("item inspect secret shop hint mismatch: %s", resI.SecretShopHint)
	}

	// Talk
	talkW, err := svc.TalkNPC(context.Background(), shop.ShopTypeWeapon)
	if err != nil || talkW == "" {
		t.Errorf("TalkNPC weapon returned empty or error: %v", err)
	}
}

func TestDiscoverSecretShop(t *testing.T) {
	svc, charRepo, _, _, _, _ := setupParityTest(t)
	char := createTestCharacter(t, charRepo, "Hero", 100)

	// Level 6 (< 7): fails
	char.JobLevel = 6
	_ = charRepo.Update(context.Background(), char)
	ok, msg, err := svc.DiscoverSecretShop(context.Background(), char.ID)
	if err != nil {
		t.Fatalf("DiscoverSecretShop error: %v", err)
	}
	if ok {
		t.Errorf("job_lv 6 should not unlock secret shop")
	}
	if msg == "" {
		t.Errorf("expected failure message")
	}

	// Level 7: succeeds
	char.JobLevel = 7
	_ = charRepo.Update(context.Background(), char)
	ok7, msg7, err := svc.DiscoverSecretShop(context.Background(), char.ID)
	if err != nil {
		t.Fatalf("DiscoverSecretShop error: %v", err)
	}
	if !ok7 {
		t.Errorf("job_lv 7 should unlock secret shop")
	}
	if msg7 != "秘密の店を見つけました！" {
		t.Errorf("unexpected success message: %s", msg7)
	}
}
