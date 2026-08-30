package blackmarket_test

import (
	"context"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/blackmarket"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
)

type mockCharacterRepo struct {
	characters map[string]corecharacter.Character
}

func newMockCharacterRepo() *mockCharacterRepo {
	return &mockCharacterRepo{characters: make(map[string]corecharacter.Character)}
}

func (m *mockCharacterRepo) FindByID(_ context.Context, id string) (corecharacter.Character, error) {
	c, ok := m.characters[id]
	if !ok {
		return corecharacter.Character{}, blackmarket.ErrCharacterNotFound
	}
	return c, nil
}

func (m *mockCharacterRepo) FindByIDForUpdate(ctx context.Context, id string) (corecharacter.Character, error) {
	return m.FindByID(ctx, id)
}

func (m *mockCharacterRepo) Update(_ context.Context, value corecharacter.Character) error {
	m.characters[value.ID] = value
	return nil
}

type mockInventoryRepo struct {
	inventories map[string]coreinventory.Inventory
}

func newMockInventoryRepo() *mockInventoryRepo {
	return &mockInventoryRepo{inventories: make(map[string]coreinventory.Inventory)}
}

func (m *mockInventoryRepo) FindByCharacterID(_ context.Context, characterID string) (coreinventory.Inventory, error) {
	inv, ok := m.inventories[characterID]
	if !ok {
		inv, _ = coreinventory.New(characterID)
		m.inventories[characterID] = inv
	}
	return inv, nil
}

func (m *mockInventoryRepo) FindByCharacterIDForUpdate(ctx context.Context, characterID string) (coreinventory.Inventory, error) {
	return m.FindByCharacterID(ctx, characterID)
}

func (m *mockInventoryRepo) Save(_ context.Context, value coreinventory.Inventory) error {
	m.inventories[value.CharacterID] = value
	return nil
}

type mockBlackMarketRepo struct {
	purchases map[string]map[string]int // charID+dateKey -> itemID -> quantity
	state     *blackmarket.MarketState
}

func newMockBlackMarketRepo() *mockBlackMarketRepo {
	return &mockBlackMarketRepo{
		purchases: make(map[string]map[string]int),
	}
}

func (m *mockBlackMarketRepo) dateKey(characterID string, date time.Time) string {
	return characterID + ":" + date.Format("2006-01-02")
}

func (m *mockBlackMarketRepo) GetDailyPurchases(_ context.Context, characterID string, date time.Time) (map[string]int, error) {
	key := m.dateKey(characterID, date)
	res, ok := m.purchases[key]
	if !ok {
		return make(map[string]int), nil
	}
	copied := make(map[string]int, len(res))
	for k, v := range res {
		copied[k] = v
	}
	return copied, nil
}

func (m *mockBlackMarketRepo) RecordPurchase(_ context.Context, characterID string, itemID string, date time.Time, quantity int) error {
	key := m.dateKey(characterID, date)
	if m.purchases[key] == nil {
		m.purchases[key] = make(map[string]int)
	}
	m.purchases[key][itemID] += quantity
	return nil
}

func (m *mockBlackMarketRepo) GetMarketState(_ context.Context) (blackmarket.MarketState, error) {
	if m.state != nil {
		return *m.state, nil
	}
	return blackmarket.MarketState{}, nil
}

func (m *mockBlackMarketRepo) SaveMarketState(_ context.Context, state blackmarket.MarketState) error {
	m.state = &state
	return nil
}

type mockItemDefProvider struct {
	defs map[string]coreitem.Definition
}

func newMockItemDefProvider() *mockItemDefProvider {
	return &mockItemDefProvider{defs: make(map[string]coreitem.Definition)}
}

func (m *mockItemDefProvider) FindByID(id string) (coreitem.Definition, error) {
	d, ok := m.defs[id]
	if !ok {
		return coreitem.Definition{}, coreitem.ErrDefinitionNotFound
	}
	return d, nil
}

type mockTxProvider struct{}

func (m *mockTxProvider) RunInTx(ctx context.Context, fn func(ctx context.Context) error) error {
	return fn(ctx)
}

func TestCatalogLoading(t *testing.T) {
	catalog, err := blackmarket.LoadDefaultCatalog()
	if err != nil {
		t.Fatalf("failed to load default catalog: %v", err)
	}
	items := catalog.Items()
	if len(items) != 10 {
		t.Errorf("expected 10 items, got %d", len(items))
	}

	needle, ok := catalog.FindByID("bm_poison_needle")
	if !ok {
		t.Fatalf("item bm_poison_needle not found")
	}
	if needle.ItemDefinitionID != "weapon-12" {
		t.Errorf("expected weapon-12, got %s", needle.ItemDefinitionID)
	}

	_, ok = catalog.FindByDefinitionID("item-036")
	if !ok {
		t.Fatalf("item definition item-036 not found")
	}
}

func TestCheckEligibility(t *testing.T) {
	cLow := corecharacter.Character{Level: 5, RebirthCount: 0}
	if blackmarket.CheckEligibility(cLow) {
		t.Errorf("expected level 5 character to not be eligible")
	}

	cReq := corecharacter.Character{Level: 10, RebirthCount: 0}
	if !blackmarket.CheckEligibility(cReq) {
		t.Errorf("expected level 10 character to be eligible")
	}

	cRebirth := corecharacter.Character{Level: 1, RebirthCount: 1}
	if !blackmarket.CheckEligibility(cRebirth) {
		t.Errorf("expected rebirth character to be eligible")
	}
}

func TestGetStatus(t *testing.T) {
	ctx := context.Background()
	charRepo := newMockCharacterRepo()
	invRepo := newMockInventoryRepo()
	bmRepo := newMockBlackMarketRepo()
	catalog, err := blackmarket.LoadDefaultCatalog()
	if err != nil {
		t.Fatal(err)
	}

	svc, err := blackmarket.NewService(
		charRepo,
		invRepo,
		bmRepo,
		catalog,
		blackmarket.WithTransactionProvider(&mockTxProvider{}),
	)
	if err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)
	char := corecharacter.Character{
		ID:    "char-1",
		Name:  "Shadow Rogue",
		Level: 15,
		Money: 50000,
	}
	_ = charRepo.Update(ctx, char)

	status, err := svc.GetStatus(ctx, "char-1", now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if status.CharacterID != "char-1" {
		t.Errorf("expected char-1, got %s", status.CharacterID)
	}
	if !status.IsEligible {
		t.Errorf("expected eligible status")
	}
	if len(status.Items) != 10 {
		t.Errorf("expected 10 items, got %d", len(status.Items))
	}
	if status.NPCName != "@ヤミジ" {
		t.Errorf("expected @ヤミジ, got %s", status.NPCName)
	}
}

func TestTalkAndRumors(t *testing.T) {
	ctx := context.Background()
	charRepo := newMockCharacterRepo()
	invRepo := newMockInventoryRepo()
	bmRepo := newMockBlackMarketRepo()
	catalog, _ := blackmarket.LoadDefaultCatalog()

	svc, _ := blackmarket.NewService(charRepo, invRepo, bmRepo, catalog)
	now := time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)

	// Low level character
	charLow := corecharacter.Character{ID: "char-low", Level: 5}
	_ = charRepo.Update(ctx, charLow)

	_, err := svc.Talk(ctx, "char-low")
	if err != blackmarket.ErrAccessDenied {
		t.Errorf("expected ErrAccessDenied, got %v", err)
	}

	_, err = svc.Rumors(ctx, "char-low", now)
	if err != blackmarket.ErrAccessDenied {
		t.Errorf("expected ErrAccessDenied, got %v", err)
	}

	// Eligible character
	charEligible := corecharacter.Character{ID: "char-ok", Level: 12}
	_ = charRepo.Update(ctx, charEligible)

	talkRes, err := svc.Talk(ctx, "char-ok")
	if err != nil {
		t.Fatalf("talk error: %v", err)
	}
	if talkRes.Dialogue == "" || talkRes.NPCName != "@ヤミジ" {
		t.Errorf("invalid talk result: %+v", talkRes)
	}

	rumorsRes, err := svc.Rumors(ctx, "char-ok", now)
	if err != nil {
		t.Fatalf("rumors error: %v", err)
	}
	if rumorsRes.Rumor == "" || rumorsRes.MarketCondition == "" {
		t.Errorf("invalid rumors result: %+v", rumorsRes)
	}
}

func TestPurchaseItem(t *testing.T) {
	ctx := context.Background()
	charRepo := newMockCharacterRepo()
	invRepo := newMockInventoryRepo()
	bmRepo := newMockBlackMarketRepo()
	catalog, _ := blackmarket.LoadDefaultCatalog()

	svc, err := blackmarket.NewService(
		charRepo,
		invRepo,
		bmRepo,
		catalog,
		blackmarket.WithTransactionProvider(&mockTxProvider{}),
	)
	if err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 8, 30, 0, 0, 0, 0, time.UTC) // Hour 0 -> Quiet (1.0x price)
	char := corecharacter.Character{
		ID:    "char-buyer",
		Name:  "Test Buyer",
		Level: 15,
		Money: 10000,
	}
	_ = charRepo.Update(ctx, char)

	// 1. Purchase success
	res, err := svc.PurchaseItem(ctx, "char-buyer", "bm_poison_needle", 2, now)
	if err != nil {
		t.Fatalf("unexpected purchase error: %v", err)
	}
	if res.UnitPrice != 1500 {
		t.Errorf("expected unit price 1500, got %d", res.UnitPrice)
	}
	if res.TotalPrice != 3000 {
		t.Errorf("expected total price 3000, got %d", res.TotalPrice)
	}
	if res.RemainingGold != 7000 {
		t.Errorf("expected remaining gold 7000, got %d", res.RemainingGold)
	}
	if res.RemainingQuota != 3 { // Daily limit is 5
		t.Errorf("expected remaining quota 3, got %d", res.RemainingQuota)
	}

	// Verify inventory
	inv, _ := invRepo.FindByCharacterID(ctx, "char-buyer")
	inst, ok := inv.Find(res.InventoryInstanceID)
	if !ok || inst.Quantity != 2 {
		t.Errorf("expected 2 items in inventory, got %+v", inst)
	}

	// 2. Exceed daily limit (Limit is 5, purchased 2, trying to buy 4)
	_, err = svc.PurchaseItem(ctx, "char-buyer", "bm_poison_needle", 4, now)
	if err != blackmarket.ErrDailyLimitExceeded {
		t.Errorf("expected ErrDailyLimitExceeded, got %v", err)
	}

	// 3. Insufficient funds
	poorChar := corecharacter.Character{ID: "char-poor", Level: 20, Money: 100}
	_ = charRepo.Update(ctx, poorChar)
	_, err = svc.PurchaseItem(ctx, "char-poor", "bm_demon_spear", 1, now)
	if err != blackmarket.ErrInsufficientFunds {
		t.Errorf("expected ErrInsufficientFunds, got %v", err)
	}

	// 4. Ineligible character
	lowChar := corecharacter.Character{ID: "char-ineligible", Level: 5, Money: 999999}
	_ = charRepo.Update(ctx, lowChar)
	_, err = svc.PurchaseItem(ctx, "char-ineligible", "bm_poison_needle", 1, now)
	if err != blackmarket.ErrAccessDenied {
		t.Errorf("expected ErrAccessDenied, got %v", err)
	}

	// 5. Invalid item ID
	_, err = svc.PurchaseItem(ctx, "char-buyer", "non_existent_item", 1, now)
	if err != blackmarket.ErrItemNotFound {
		t.Errorf("expected ErrItemNotFound, got %v", err)
	}

	// 6. Invalid quantity
	_, err = svc.PurchaseItem(ctx, "char-buyer", "bm_poison_needle", 0, now)
	if err != blackmarket.ErrInvalidQuantity {
		t.Errorf("expected ErrInvalidQuantity, got %v", err)
	}
}

func TestSellItem(t *testing.T) {
	ctx := context.Background()
	charRepo := newMockCharacterRepo()
	invRepo := newMockInventoryRepo()
	bmRepo := newMockBlackMarketRepo()
	itemDefs := newMockItemDefProvider()
	catalog, _ := blackmarket.LoadDefaultCatalog()

	itemDefs.defs["weapon-12"] = coreitem.Definition{
		ID:    "weapon-12",
		Name:  "どくばり",
		Price: 1500,
	}

	svc, err := blackmarket.NewService(
		charRepo,
		invRepo,
		bmRepo,
		catalog,
		blackmarket.WithItemDefinitionProvider(itemDefs),
		blackmarket.WithTransactionProvider(&mockTxProvider{}),
	)
	if err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 8, 30, 0, 0, 0, 0, time.UTC) // Quiet -> 1.0x sell multiplier
	char := corecharacter.Character{
		ID:    "char-seller",
		Name:  "Test Seller",
		Level: 15,
		Money: 1000,
	}
	_ = charRepo.Update(ctx, char)

	inv, _ := invRepo.FindByCharacterID(ctx, "char-seller")
	inst, _ := coreitem.NewInstance("weapon-12", 5)
	_ = inv.Add(inst)
	_ = invRepo.Save(ctx, inv)

	// Sell 2 units (base price 1500, 60% = 900 G each)
	res, err := svc.SellItem(ctx, "char-seller", inst.ID, 2, now)
	if err != nil {
		t.Fatalf("unexpected sell error: %v", err)
	}
	if res.UnitPrice != 900 {
		t.Errorf("expected unit price 900, got %d", res.UnitPrice)
	}
	if res.TotalPayout != 1800 {
		t.Errorf("expected total payout 1800, got %d", res.TotalPayout)
	}
	if res.RemainingGold != 2800 {
		t.Errorf("expected remaining gold 2800, got %d", res.RemainingGold)
	}

	// Verify inventory reduced to 3
	updatedInv, _ := invRepo.FindByCharacterID(ctx, "char-seller")
	updatedInst, _ := updatedInv.Find(inst.ID)
	if updatedInst.Quantity != 3 {
		t.Errorf("expected 3 remaining, got %d", updatedInst.Quantity)
	}

	// Sell unowned item instance
	_, err = svc.SellItem(ctx, "char-seller", "non-existent-inst", 1, now)
	if err != blackmarket.ErrUnownedItem {
		t.Errorf("expected ErrUnownedItem, got %v", err)
	}

	// Sell excessive quantity
	_, err = svc.SellItem(ctx, "char-seller", inst.ID, 10, now)
	if err != blackmarket.ErrInvalidQuantity {
		t.Errorf("expected ErrInvalidQuantity, got %v", err)
	}
}
