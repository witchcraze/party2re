package blackmarket_test

import (
	"context"
	"testing"

	"github.com/witchcraze/party2re/internal/blackmarket"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/depot"
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

type mockDepotRepo struct {
	depots map[string]depot.Depot
}

func newMockDepotRepo() *mockDepotRepo {
	return &mockDepotRepo{depots: make(map[string]depot.Depot)}
}

func (m *mockDepotRepo) FindByCharacterID(_ context.Context, characterID string) (depot.Depot, error) {
	d, ok := m.depots[characterID]
	if !ok {
		d, _ = depot.NewDepot(characterID)
		m.depots[characterID] = d
	}
	return d, nil
}

func (m *mockDepotRepo) FindByCharacterIDForUpdate(ctx context.Context, characterID string) (depot.Depot, error) {
	return m.FindByCharacterID(ctx, characterID)
}

func (m *mockDepotRepo) Save(_ context.Context, value depot.Depot) error {
	m.depots[value.CharacterID] = value
	return nil
}

type mockBlackMarketRepo struct {
	points map[string]blackmarket.CharacterPoints
}

func newMockBlackMarketRepo() *mockBlackMarketRepo {
	return &mockBlackMarketRepo{
		points: make(map[string]blackmarket.CharacterPoints),
	}
}

func (m *mockBlackMarketRepo) GetCharacterPoints(_ context.Context, characterID string) (blackmarket.CharacterPoints, error) {
	if pts, ok := m.points[characterID]; ok {
		return pts, nil
	}
	return blackmarket.CharacterPoints{CharacterID: characterID, RarePoints: 0, URarePoints: 0}, nil
}

func (m *mockBlackMarketRepo) GetCharacterPointsForUpdate(ctx context.Context, characterID string) (blackmarket.CharacterPoints, error) {
	return m.GetCharacterPoints(ctx, characterID)
}

func (m *mockBlackMarketRepo) SaveCharacterPoints(_ context.Context, points blackmarket.CharacterPoints) error {
	m.points[points.CharacterID] = points
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

	prizes := catalog.Prizes()
	if len(prizes) != 24 {
		t.Errorf("expected 24 total prizes (12 regular + 12 ura), got %d", len(prizes))
	}

	regularPrizes := catalog.RegularPrizes()
	if len(regularPrizes) != 12 {
		t.Errorf("expected 12 regular prizes, got %d", len(regularPrizes))
	}

	uPrizes := catalog.UPrizes()
	if len(uPrizes) != 12 {
		t.Errorf("expected 12 u-prizes, got %d", len(uPrizes))
	}

	// Verify legacy prize IDs
	p087, ok := catalog.FindPrizeByID("bm_prize_087")
	if !ok {
		t.Fatalf("bm_prize_087 not found")
	}
	if p087.ItemDefinitionID != "item-087" || p087.Cost != 1 || p087.IsURare {
		t.Errorf("unexpected bm_prize_087: %+v", p087)
	}

	up262, ok := catalog.FindPrizeByID("bm_uprize_262")
	if !ok {
		t.Fatalf("bm_uprize_262 not found")
	}
	if up262.ItemDefinitionID != "item-262" || up262.Cost != 20 || !up262.IsURare {
		t.Errorf("unexpected bm_uprize_262: %+v", up262)
	}

	// Verify sacrifice yields
	yWeapon29, ok := catalog.GetSacrificeYield("weapon-29")
	if !ok || yWeapon29.RarePoints != 1 || yWeapon29.URarePoints != 0 {
		t.Errorf("unexpected sacrifice yield for weapon-29: %+v", yWeapon29)
	}

	yURare268, ok := catalog.GetSacrificeYield("item-268")
	if !ok || yURare268.URarePoints != 50 || yURare268.RarePoints != 0 {
		t.Errorf("unexpected sacrifice yield for item-268: %+v", yURare268)
	}
}

func TestCheckEligibility(t *testing.T) {
	c := corecharacter.Character{Level: 1}
	if !blackmarket.CheckEligibility(c) {
		t.Errorf("expected level 1 character to be eligible under authentic Party2 spec")
	}
}

func TestGetStatus(t *testing.T) {
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

	char := corecharacter.Character{
		ID:    "char-1",
		Name:  "Hero",
		Level: 1,
	}
	_ = charRepo.Update(ctx, char)

	bmRepo.points["char-1"] = blackmarket.CharacterPoints{
		CharacterID: "char-1",
		RarePoints:  7,
		URarePoints: 3,
	}

	status, err := svc.GetStatus(ctx, "char-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if status.CharacterID != "char-1" {
		t.Errorf("expected char-1, got %s", status.CharacterID)
	}
	if status.NPCName != "@闇商人" {
		t.Errorf("expected @闇商人, got %s", status.NPCName)
	}
	if status.LocationName != "闇市場" {
		t.Errorf("expected 闇市場, got %s", status.LocationName)
	}
	if status.RarePoints != 7 || status.URarePoints != 3 {
		t.Errorf("expected Rare=7 URare=3, got Rare=%d URare=%d", status.RarePoints, status.URarePoints)
	}
	if len(status.Prizes) != 12 || len(status.UPrizes) != 12 {
		t.Errorf("expected 12 regular and 12 u-prizes")
	}
}

func TestTalkAndInspect(t *testing.T) {
	ctx := context.Background()
	charRepo := newMockCharacterRepo()
	invRepo := newMockInventoryRepo()
	bmRepo := newMockBlackMarketRepo()
	catalog, _ := blackmarket.LoadDefaultCatalog()

	svc, _ := blackmarket.NewService(charRepo, invRepo, bmRepo, catalog)

	char := corecharacter.Character{ID: "char-talker", Name: "Talker"}
	_ = charRepo.Update(ctx, char)

	talk, err := svc.Talk(ctx, "char-talker")
	if err != nil {
		t.Fatalf("unexpected Talk error: %v", err)
	}
	if talk.NPCName != "@闇商人" {
		t.Errorf("expected @闇商人, got %s", talk.NPCName)
	}
	if talk.Dialogue == "" {
		t.Errorf("expected non-empty dialogue")
	}

	inspect, err := svc.Inspect(ctx, "char-talker")
	if err != nil {
		t.Fatalf("unexpected Inspect error: %v", err)
	}
	if inspect.NPCName != "@闇商人" {
		t.Errorf("expected @闇商人, got %s", inspect.NPCName)
	}
	if inspect.Dialogue != "…お前の魂で取引したいのか？" {
		t.Errorf("expected authentic inspect dialogue, got %s", inspect.Dialogue)
	}
}

func TestSacrificeItem_Inventory(t *testing.T) {
	ctx := context.Background()
	charRepo := newMockCharacterRepo()
	invRepo := newMockInventoryRepo()
	bmRepo := newMockBlackMarketRepo()
	catalog, _ := blackmarket.LoadDefaultCatalog()
	itemDefs := newMockItemDefProvider()
	itemDefs.defs["weapon-29"] = coreitem.Definition{ID: "weapon-29", Name: "はやぶさの剣"}
	itemDefs.defs["item-263"] = coreitem.Definition{ID: "item-263", Name: "オリハルコン"}
	itemDefs.defs["item-001"] = coreitem.Definition{ID: "item-001", Name: "薬草"}

	svc, _ := blackmarket.NewService(
		charRepo,
		invRepo,
		bmRepo,
		catalog,
		blackmarket.WithItemDefinitionProvider(itemDefs),
		blackmarket.WithTransactionProvider(&mockTxProvider{}),
	)

	char := corecharacter.Character{ID: "char-hero", Name: "Hero"}
	_ = charRepo.Update(ctx, char)

	inv, _ := invRepo.FindByCharacterID(ctx, "char-hero")
	instWeapon, _ := coreitem.NewInstance("weapon-29", 1)
	_ = inv.Add(instWeapon)
	_ = invRepo.Save(ctx, inv)

	// Sacrifice regular rare item
	res, err := svc.SacrificeItem(ctx, "char-hero", instWeapon.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.RarePointsGained != 1 || res.TotalRarePoints != 1 {
		t.Errorf("expected 1 rare point, got gained=%d total=%d", res.RarePointsGained, res.TotalRarePoints)
	}
	if res.Message != "…はやぶさの剣…か…。レアだな…。いいだろう…。お前のレアポイントを加算しておこう…" {
		t.Errorf("unexpected dialogue: %s", res.Message)
	}

	// Verify item was consumed
	inv, _ = invRepo.FindByCharacterID(ctx, "char-hero")
	if _, found := inv.Find(instWeapon.ID); found {
		t.Errorf("expected weapon-29 to be consumed from inventory")
	}

	// Sacrifice ultra-rare item
	instURare, _ := coreitem.NewInstance("item-263", 1)
	_ = inv.Add(instURare)
	_ = invRepo.Save(ctx, inv)

	resURare, err := svc.SacrificeItem(ctx, "char-hero", instURare.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resURare.URarePointsGained != 1 || resURare.TotalURarePoints != 1 {
		t.Errorf("expected 1 u-rare point, got gained=%d total=%d", resURare.URarePointsGained, resURare.TotalURarePoints)
	}
	if resURare.Message != "これは……! ……いいだろう…。お前の特別なレアポイントを1加算しておこう…" {
		t.Errorf("unexpected dialogue: %s", resURare.Message)
	}

	// Sacrifice non-rare item -> ErrNotSacrificeEligible
	instCommon, _ := coreitem.NewInstance("item-001", 1)
	_ = inv.Add(instCommon)
	_ = invRepo.Save(ctx, inv)

	_, err = svc.SacrificeItem(ctx, "char-hero", instCommon.ID)
	if err != blackmarket.ErrNotSacrificeEligible {
		t.Errorf("expected ErrNotSacrificeEligible, got %v", err)
	}
}

func TestSacrificeItem_Depot(t *testing.T) {
	ctx := context.Background()
	charRepo := newMockCharacterRepo()
	invRepo := newMockInventoryRepo()
	depotRepo := newMockDepotRepo()
	bmRepo := newMockBlackMarketRepo()
	catalog, _ := blackmarket.LoadDefaultCatalog()
	itemDefs := newMockItemDefProvider()
	itemDefs.defs["armor-35"] = coreitem.Definition{ID: "armor-35", Name: "神秘の鎧"}

	svc, _ := blackmarket.NewService(
		charRepo,
		invRepo,
		bmRepo,
		catalog,
		blackmarket.WithDepotRepository(depotRepo),
		blackmarket.WithItemDefinitionProvider(itemDefs),
		blackmarket.WithTransactionProvider(&mockTxProvider{}),
	)

	char := corecharacter.Character{ID: "char-depot", Name: "DepotUser"}
	_ = charRepo.Update(ctx, char)

	dep, _ := depotRepo.FindByCharacterID(ctx, "char-depot")
	armorInst, _ := coreitem.NewInstance("armor-35", 1)
	_ = dep.AddItem(armorInst)
	_ = depotRepo.Save(ctx, dep)

	res, err := svc.SacrificeItem(ctx, "char-depot", armorInst.ID)
	if err != nil {
		t.Fatalf("unexpected sacrifice from depot error: %v", err)
	}
	if res.RarePointsGained != 1 || res.TotalRarePoints != 1 {
		t.Errorf("expected 1 rare point, got gained=%d total=%d", res.RarePointsGained, res.TotalRarePoints)
	}

	// Verify depot item removed
	dep, _ = depotRepo.FindByCharacterID(ctx, "char-depot")
	if len(dep.Items) != 0 {
		t.Errorf("expected depot to be empty after sacrifice, got %d items", len(dep.Items))
	}
}

func TestTradePrize_ToDepot(t *testing.T) {
	ctx := context.Background()
	charRepo := newMockCharacterRepo()
	invRepo := newMockInventoryRepo()
	depotRepo := newMockDepotRepo()
	bmRepo := newMockBlackMarketRepo()
	catalog, _ := blackmarket.LoadDefaultCatalog()

	svc, _ := blackmarket.NewService(
		charRepo,
		invRepo,
		bmRepo,
		catalog,
		blackmarket.WithDepotRepository(depotRepo),
		blackmarket.WithTransactionProvider(&mockTxProvider{}),
	)

	char := corecharacter.Character{
		ID:       "char-trader",
		Name:     "Trader",
		JobLevel: 10,
	}
	_ = charRepo.Update(ctx, char)

	bmRepo.points["char-trader"] = blackmarket.CharacterPoints{
		CharacterID: "char-trader",
		RarePoints:  10,
		URarePoints: 20,
	}

	// Trade regular prize: bm_prize_087 costs 1 Rare Point
	res, err := svc.TradePrize(ctx, "char-trader", "bm_prize_087")
	if err != nil {
		t.Fatalf("unexpected TradePrize error: %v", err)
	}
	if res.RemainingRare != 9 {
		t.Errorf("expected 9 remaining rare points, got %d", res.RemainingRare)
	}
	if res.Message != "取引成立だ…。まほうのそろばん はお前の預かり所に送っておいた…" {
		t.Errorf("unexpected message: %s", res.Message)
	}

	// Verify item in depot
	dep, _ := depotRepo.FindByCharacterID(ctx, "char-trader")
	if len(dep.Items) != 1 || dep.Items[0].DefinitionID != "item-087" {
		t.Errorf("expected item-087 in depot, got %+v", dep.Items)
	}

	// Trade u-prize: bm_uprize_262 costs 20 U-Rare Points
	resU, err := svc.TradePrize(ctx, "char-trader", "bm_uprize_262")
	if err != nil {
		t.Fatalf("unexpected TradePrize u-prize error: %v", err)
	}
	if resU.RemainingURare != 0 {
		t.Errorf("expected 0 remaining u-rare points, got %d", resU.RemainingURare)
	}

	dep, _ = depotRepo.FindByCharacterID(ctx, "char-trader")
	if len(dep.Items) != 2 {
		t.Errorf("expected 2 items in depot, got %d", len(dep.Items))
	}

	// Insufficient rare points
	bmRepo.points["char-trader"] = blackmarket.CharacterPoints{
		CharacterID: "char-trader",
		RarePoints:  0,
		URarePoints: 0,
	}
	_, err = svc.TradePrize(ctx, "char-trader", "bm_prize_087")
	if err != blackmarket.ErrInsufficientRarePoints {
		t.Errorf("expected ErrInsufficientRarePoints, got %v", err)
	}

	// Insufficient u-rare points
	_, err = svc.TradePrize(ctx, "char-trader", "bm_uprize_262")
	if err != blackmarket.ErrInsufficientURarePoints {
		t.Errorf("expected ErrInsufficientURarePoints, got %v", err)
	}

	// Non-existent prize
	_, err = svc.TradePrize(ctx, "char-trader", "invalid_prize_id")
	if err != blackmarket.ErrPrizeNotFound {
		t.Errorf("expected ErrPrizeNotFound, got %v", err)
	}
}

func TestTradePrize_DepotFull(t *testing.T) {
	ctx := context.Background()
	charRepo := newMockCharacterRepo()
	invRepo := newMockInventoryRepo()
	depotRepo := newMockDepotRepo()
	bmRepo := newMockBlackMarketRepo()
	catalog, _ := blackmarket.LoadDefaultCatalog()

	svc, _ := blackmarket.NewService(
		charRepo,
		invRepo,
		bmRepo,
		catalog,
		blackmarket.WithDepotRepository(depotRepo),
		blackmarket.WithTransactionProvider(&mockTxProvider{}),
	)

	// Level 0 character has base depot capacity 5
	char := corecharacter.Character{
		ID:       "char-full-depot",
		Name:     "FullDepotUser",
		JobLevel: 0,
	}
	_ = charRepo.Update(ctx, char)

	bmRepo.points["char-full-depot"] = blackmarket.CharacterPoints{
		CharacterID: "char-full-depot",
		RarePoints:  10,
	}

	dep, _ := depotRepo.FindByCharacterID(ctx, "char-full-depot")
	dep.Capacity = 5
	for i := 1; i <= 5; i++ {
		dummyInst, _ := coreitem.NewInstance("unique-item-"+string(rune('A'+i)), 1)
		_ = dep.AddItem(dummyInst)
	}
	_ = depotRepo.Save(ctx, dep)

	// Trade should fail with ErrDepotFull
	_, err := svc.TradePrize(ctx, "char-full-depot", "bm_prize_087")
	if err != blackmarket.ErrDepotFull {
		t.Errorf("expected ErrDepotFull, got %v", err)
	}
}

func TestNewService_NilDependencies(t *testing.T) {
	charRepo := newMockCharacterRepo()
	invRepo := newMockInventoryRepo()
	bmRepo := newMockBlackMarketRepo()
	catalog, _ := blackmarket.LoadDefaultCatalog()

	if _, err := blackmarket.NewService(nil, invRepo, bmRepo, catalog); err != blackmarket.ErrNilDependency {
		t.Errorf("expected ErrNilDependency, got %v", err)
	}
	if _, err := blackmarket.NewService(charRepo, nil, bmRepo, catalog); err != blackmarket.ErrNilDependency {
		t.Errorf("expected ErrNilDependency, got %v", err)
	}
	if _, err := blackmarket.NewService(charRepo, invRepo, nil, catalog); err != blackmarket.ErrNilDependency {
		t.Errorf("expected ErrNilDependency, got %v", err)
	}
	if _, err := blackmarket.NewService(charRepo, invRepo, bmRepo, nil); err != blackmarket.ErrNilDependency {
		t.Errorf("expected ErrNilDependency, got %v", err)
	}
}

func TestValidationErrors(t *testing.T) {
	ctx := context.Background()
	charRepo := newMockCharacterRepo()
	invRepo := newMockInventoryRepo()
	bmRepo := newMockBlackMarketRepo()
	catalog, _ := blackmarket.LoadDefaultCatalog()

	svc, _ := blackmarket.NewService(charRepo, invRepo, bmRepo, catalog)

	// Blank character ID checks
	if _, err := svc.GetStatus(ctx, ""); err != blackmarket.ErrCharacterNotFound {
		t.Errorf("expected ErrCharacterNotFound, got %v", err)
	}
	if _, err := svc.GetPointsStatus(ctx, ""); err != blackmarket.ErrCharacterNotFound {
		t.Errorf("expected ErrCharacterNotFound, got %v", err)
	}
	if _, err := svc.Talk(ctx, ""); err != blackmarket.ErrCharacterNotFound {
		t.Errorf("expected ErrCharacterNotFound, got %v", err)
	}
	if _, err := svc.Inspect(ctx, ""); err != blackmarket.ErrCharacterNotFound {
		t.Errorf("expected ErrCharacterNotFound, got %v", err)
	}
	if _, err := svc.SacrificeItem(ctx, "", "inst"); err != blackmarket.ErrCharacterNotFound {
		t.Errorf("expected ErrCharacterNotFound, got %v", err)
	}
	if _, err := svc.SacrificeItem(ctx, "char-1", ""); err != blackmarket.ErrUnownedItem {
		t.Errorf("expected ErrUnownedItem, got %v", err)
	}
	if _, err := svc.TradePrize(ctx, "", "prize"); err != blackmarket.ErrCharacterNotFound {
		t.Errorf("expected ErrCharacterNotFound, got %v", err)
	}
	if _, err := svc.TradePrize(ctx, "char-1", ""); err != blackmarket.ErrPrizeNotFound {
		t.Errorf("expected ErrPrizeNotFound, got %v", err)
	}

	// Depot not configured
	char := corecharacter.Character{ID: "char-nodepot"}
	_ = charRepo.Update(ctx, char)
	if _, err := svc.TradePrize(ctx, "char-nodepot", "bm_prize_087"); err != blackmarket.ErrDepotNotConfigured {
		t.Errorf("expected ErrDepotNotConfigured, got %v", err)
	}
}
