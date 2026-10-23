package blackmarket_test

import (
	"context"
	"errors"
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

	// Verify item-059 sacrifice yield
	yItem059, ok := catalog.GetSacrificeYield("item-059")
	if !ok || yItem059.RarePoints != 1 || yItem059.URarePoints != 0 {
		t.Errorf("unexpected sacrifice yield for item-059: %+v (ok=%v)", yItem059, ok)
	}

	// Verify all 46 authentic legacy sacrifice items
	expectedSacrifices := map[string]blackmarket.SacrificeYield{
		// Weapons: 29, 30, 33..40 (10 items)
		"weapon-29": {ItemDefinitionID: "weapon-29", RarePoints: 1, URarePoints: 0},
		"weapon-30": {ItemDefinitionID: "weapon-30", RarePoints: 1, URarePoints: 0},
		"weapon-33": {ItemDefinitionID: "weapon-33", RarePoints: 1, URarePoints: 0},
		"weapon-34": {ItemDefinitionID: "weapon-34", RarePoints: 1, URarePoints: 0},
		"weapon-35": {ItemDefinitionID: "weapon-35", RarePoints: 1, URarePoints: 0},
		"weapon-36": {ItemDefinitionID: "weapon-36", RarePoints: 1, URarePoints: 0},
		"weapon-37": {ItemDefinitionID: "weapon-37", RarePoints: 1, URarePoints: 0},
		"weapon-38": {ItemDefinitionID: "weapon-38", RarePoints: 1, URarePoints: 0},
		"weapon-39": {ItemDefinitionID: "weapon-39", RarePoints: 1, URarePoints: 0},
		"weapon-40": {ItemDefinitionID: "weapon-40", RarePoints: 1, URarePoints: 0},
		// Armors: 35..40 (6 items)
		"armor-35": {ItemDefinitionID: "armor-35", RarePoints: 1, URarePoints: 0},
		"armor-36": {ItemDefinitionID: "armor-36", RarePoints: 1, URarePoints: 0},
		"armor-37": {ItemDefinitionID: "armor-37", RarePoints: 1, URarePoints: 0},
		"armor-38": {ItemDefinitionID: "armor-38", RarePoints: 1, URarePoints: 0},
		"armor-39": {ItemDefinitionID: "armor-39", RarePoints: 1, URarePoints: 0},
		"armor-40": {ItemDefinitionID: "armor-40", RarePoints: 1, URarePoints: 0},
		// Regular Items: 28, 29, 40, 57..71, 87, 104..107, 109 (24 items)
		"item-028": {ItemDefinitionID: "item-028", RarePoints: 1, URarePoints: 0},
		"item-029": {ItemDefinitionID: "item-029", RarePoints: 1, URarePoints: 0},
		"item-040": {ItemDefinitionID: "item-040", RarePoints: 1, URarePoints: 0},
		"item-057": {ItemDefinitionID: "item-057", RarePoints: 1, URarePoints: 0},
		"item-058": {ItemDefinitionID: "item-058", RarePoints: 1, URarePoints: 0},
		"item-059": {ItemDefinitionID: "item-059", RarePoints: 1, URarePoints: 0},
		"item-060": {ItemDefinitionID: "item-060", RarePoints: 1, URarePoints: 0},
		"item-061": {ItemDefinitionID: "item-061", RarePoints: 1, URarePoints: 0},
		"item-062": {ItemDefinitionID: "item-062", RarePoints: 1, URarePoints: 0},
		"item-063": {ItemDefinitionID: "item-063", RarePoints: 1, URarePoints: 0},
		"item-064": {ItemDefinitionID: "item-064", RarePoints: 1, URarePoints: 0},
		"item-065": {ItemDefinitionID: "item-065", RarePoints: 1, URarePoints: 0},
		"item-066": {ItemDefinitionID: "item-066", RarePoints: 1, URarePoints: 0},
		"item-067": {ItemDefinitionID: "item-067", RarePoints: 1, URarePoints: 0},
		"item-068": {ItemDefinitionID: "item-068", RarePoints: 1, URarePoints: 0},
		"item-069": {ItemDefinitionID: "item-069", RarePoints: 1, URarePoints: 0},
		"item-070": {ItemDefinitionID: "item-070", RarePoints: 1, URarePoints: 0},
		"item-071": {ItemDefinitionID: "item-071", RarePoints: 1, URarePoints: 0},
		"item-087": {ItemDefinitionID: "item-087", RarePoints: 1, URarePoints: 0},
		"item-104": {ItemDefinitionID: "item-104", RarePoints: 1, URarePoints: 0},
		"item-105": {ItemDefinitionID: "item-105", RarePoints: 1, URarePoints: 0},
		"item-106": {ItemDefinitionID: "item-106", RarePoints: 1, URarePoints: 0},
		"item-107": {ItemDefinitionID: "item-107", RarePoints: 1, URarePoints: 0},
		"item-109": {ItemDefinitionID: "item-109", RarePoints: 1, URarePoints: 0},
		// Ultra-Rare Items: 263..268 (6 items)
		"item-263": {ItemDefinitionID: "item-263", RarePoints: 0, URarePoints: 1},
		"item-264": {ItemDefinitionID: "item-264", RarePoints: 0, URarePoints: 3},
		"item-265": {ItemDefinitionID: "item-265", RarePoints: 0, URarePoints: 5},
		"item-266": {ItemDefinitionID: "item-266", RarePoints: 0, URarePoints: 10},
		"item-267": {ItemDefinitionID: "item-267", RarePoints: 0, URarePoints: 20},
		"item-268": {ItemDefinitionID: "item-268", RarePoints: 0, URarePoints: 50},
	}

	if len(expectedSacrifices) != 46 {
		t.Fatalf("expected 46 items in test verification set, got %d", len(expectedSacrifices))
	}

	sacrifices := catalog.Sacrifices()
	if len(sacrifices) != 46 {
		t.Errorf("expected 46 sacrifices in catalog, got %d", len(sacrifices))
	}

	for _, s := range sacrifices {
		if _, ok := expectedSacrifices[s.ItemDefinitionID]; !ok {
			t.Errorf("unexpected sacrifice item in catalog: %+v", s)
		}
	}

	for id, expected := range expectedSacrifices {
		actual, ok := catalog.GetSacrificeYield(id)
		if !ok {
			t.Errorf("missing expected sacrifice item: %s", id)
			continue
		}
		if actual.RarePoints != expected.RarePoints || actual.URarePoints != expected.URarePoints {
			t.Errorf("item %s yield mismatch: got (Rare=%d, URare=%d), expected (Rare=%d, URare=%d)",
				id, actual.RarePoints, actual.URarePoints, expected.RarePoints, expected.URarePoints)
		}
	}

	// Verify non-rare / non-eligible items return false
	ineligibleSample := []string{"weapon-01", "armor-01", "item-001", "item-056", "item-072", "item-086", "item-088", "item-262", "item-269"}
	for _, id := range ineligibleSample {
		if y, ok := catalog.GetSacrificeYield(id); ok {
			t.Errorf("item %s should not be sacrifice eligible, but got %+v", id, y)
		}
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

	inspect, err := svc.Inspect(ctx, "char-talker")
	if err != nil {
		t.Fatalf("unexpected Inspect error: %v", err)
	}
	if inspect.NPCName != "@闇商人" {
		t.Errorf("expected @闇商人, got %s", inspect.NPCName)
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
	itemDefs.defs["item-059"] = coreitem.Definition{ID: "item-059", Name: "天馬のたづな"}
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

	// Sacrifice restored item-059 (天馬のたづな)
	inst059, _ := coreitem.NewInstance("item-059", 1)
	_ = inv.Add(inst059)
	_ = invRepo.Save(ctx, inv)

	res059, err := svc.SacrificeItem(ctx, "char-hero", inst059.ID)
	if err != nil {
		t.Fatalf("unexpected error sacrificing item-059: %v", err)
	}
	if res059.RarePointsGained != 1 || res059.URarePointsGained != 0 {
		t.Errorf("expected 1 rare point and 0 u-rare points for item-059, got rare=%d u-rare=%d",
			res059.RarePointsGained, res059.URarePointsGained)
	}
	if res059.TotalRarePoints != 2 || res059.TotalURarePoints != 1 {
		t.Errorf("expected total rare=2 u-rare=1, got rare=%d u-rare=%d", res059.TotalRarePoints, res059.TotalURarePoints)
	}
	if res059.Message != "…天馬のたづな…か…。レアだな…。いいだろう…。お前のレアポイントを加算しておこう…" {
		t.Errorf("unexpected dialogue for item-059: %s", res059.Message)
	}

	// Verify item-059 was consumed from inventory
	inv, _ = invRepo.FindByCharacterID(ctx, "char-hero")
	if _, found := inv.Find(inst059.ID); found {
		t.Errorf("expected item-059 to be consumed from inventory")
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
	itemDefs.defs["item-059"] = coreitem.Definition{ID: "item-059", Name: "天馬のたづな"}

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

	// Sacrifice restored item-059 from depot
	inst059, _ := coreitem.NewInstance("item-059", 1)
	_ = dep.AddItem(inst059)
	_ = depotRepo.Save(ctx, dep)

	res059, err := svc.SacrificeItem(ctx, "char-depot", inst059.ID)
	if err != nil {
		t.Fatalf("unexpected sacrifice item-059 from depot error: %v", err)
	}
	if res059.RarePointsGained != 1 || res059.TotalRarePoints != 2 || res059.URarePointsGained != 0 {
		t.Errorf("expected gained=1 total=2 rare points for item-059 from depot, got gained=%d total=%d",
			res059.RarePointsGained, res059.TotalRarePoints)
	}
	dep, _ = depotRepo.FindByCharacterID(ctx, "char-depot")
	if len(dep.Items) != 0 {
		t.Errorf("expected depot to be empty after item-059 sacrifice, got %d items", len(dep.Items))
	}
}

func TestSacrificeItem_PreservationOnFailure(t *testing.T) {
	ctx := context.Background()
	charRepo := newMockCharacterRepo()
	invRepo := newMockInventoryRepo()
	depotRepo := newMockDepotRepo()
	bmRepo := newMockBlackMarketRepo()
	catalog, _ := blackmarket.LoadDefaultCatalog()
	itemDefs := newMockItemDefProvider()
	itemDefs.defs["item-001"] = coreitem.Definition{ID: "item-001", Name: "薬草"}
	itemDefs.defs["item-059"] = coreitem.Definition{ID: "item-059", Name: "天馬のたづな"}

	// Set initial points
	bmRepo.points["char-fail"] = blackmarket.CharacterPoints{
		CharacterID: "char-fail",
		RarePoints:  10,
		URarePoints: 5,
	}

	char := corecharacter.Character{ID: "char-fail", Name: "FailTestChar"}
	_ = charRepo.Update(ctx, char)

	inv, _ := invRepo.FindByCharacterID(ctx, "char-fail")
	instCommon, _ := coreitem.NewInstance("item-001", 2)
	_ = inv.Add(instCommon)
	_ = invRepo.Save(ctx, inv)

	dep, _ := depotRepo.FindByCharacterID(ctx, "char-fail")
	instDepot059, _ := coreitem.NewInstance("item-059", 1)
	_ = dep.AddItem(instDepot059)
	_ = depotRepo.Save(ctx, dep)

	svc, _ := blackmarket.NewService(
		charRepo,
		invRepo,
		bmRepo,
		catalog,
		blackmarket.WithDepotRepository(depotRepo),
		blackmarket.WithItemDefinitionProvider(itemDefs),
		blackmarket.WithTransactionProvider(&mockTxProvider{}),
	)

	// 1. Failure: Ineligible item rejected
	_, err := svc.SacrificeItem(ctx, "char-fail", instCommon.ID)
	if !errors.Is(err, blackmarket.ErrNotSacrificeEligible) {
		t.Fatalf("expected ErrNotSacrificeEligible, got %v", err)
	}

	// Verify inventory unchanged
	inv, _ = invRepo.FindByCharacterID(ctx, "char-fail")
	foundInst, found := inv.Find(instCommon.ID)
	if !found || foundInst.Quantity != 2 {
		t.Errorf("expected inventory item to be preserved with quantity 2, got found=%v quantity=%d", found, foundInst.Quantity)
	}

	// Verify depot unchanged
	dep, _ = depotRepo.FindByCharacterID(ctx, "char-fail")
	if len(dep.Items) != 1 || dep.Items[0].DefinitionID != "item-059" {
		t.Errorf("expected depot item-059 preserved, got %+v", dep.Items)
	}

	// Verify points unchanged
	pts := bmRepo.points["char-fail"]
	if pts.RarePoints != 10 || pts.URarePoints != 5 {
		t.Errorf("expected points preserved at (10, 5), got (%d, %d)", pts.RarePoints, pts.URarePoints)
	}

	// 2. Failure: Non-existent item ID rejected
	_, err = svc.SacrificeItem(ctx, "char-fail", "non-existent-instance-id")
	if !errors.Is(err, blackmarket.ErrUnownedItem) {
		t.Fatalf("expected ErrUnownedItem, got %v", err)
	}

	// Verify points still unchanged
	pts = bmRepo.points["char-fail"]
	if pts.RarePoints != 10 || pts.URarePoints != 5 {
		t.Errorf("expected points preserved at (10, 5), got (%d, %d)", pts.RarePoints, pts.URarePoints)
	}
}

func TestSacrificeItem_Depot_StackedItem(t *testing.T) {
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

	char := corecharacter.Character{ID: "char-depot-stack", Name: "StackDepotUser"}
	_ = charRepo.Update(ctx, char)

	dep, _ := depotRepo.FindByCharacterID(ctx, "char-depot-stack")
	armorInst := coreitem.Instance{
		ID:           "inst-rare-stack",
		DefinitionID: "armor-35",
		Quantity:     3,
	}
	_ = dep.AddItem(armorInst)
	_ = depotRepo.Save(ctx, dep)

	// First sacrifice: Quantity 3 -> 2
	res1, err := svc.SacrificeItem(ctx, "char-depot-stack", "inst-rare-stack")
	if err != nil {
		t.Fatalf("first sacrifice failed: %v", err)
	}
	if res1.RarePointsGained != 1 || res1.TotalRarePoints != 1 {
		t.Errorf("expected 1 rare point, got gained=%d total=%d", res1.RarePointsGained, res1.TotalRarePoints)
	}
	dep, _ = depotRepo.FindByCharacterID(ctx, "char-depot-stack")
	if len(dep.Items) != 1 {
		t.Fatalf("expected 1 item in depot, got %d", len(dep.Items))
	}
	if dep.Items[0].Quantity != 2 {
		t.Errorf("expected quantity 2 in depot, got %d", dep.Items[0].Quantity)
	}

	// Second sacrifice: Quantity 2 -> 1
	res2, err := svc.SacrificeItem(ctx, "char-depot-stack", "inst-rare-stack")
	if err != nil {
		t.Fatalf("second sacrifice failed: %v", err)
	}
	if res2.RarePointsGained != 1 || res2.TotalRarePoints != 2 {
		t.Errorf("expected total 2 rare points, got gained=%d total=%d", res2.RarePointsGained, res2.TotalRarePoints)
	}
	dep, _ = depotRepo.FindByCharacterID(ctx, "char-depot-stack")
	if len(dep.Items) != 1 || dep.Items[0].Quantity != 1 {
		t.Fatalf("expected 1 item with quantity 1 in depot, got %+v", dep.Items)
	}

	// Third sacrifice: Quantity 1 -> 0 (slot removed)
	res3, err := svc.SacrificeItem(ctx, "char-depot-stack", "inst-rare-stack")
	if err != nil {
		t.Fatalf("third sacrifice failed: %v", err)
	}
	if res3.RarePointsGained != 1 || res3.TotalRarePoints != 3 {
		t.Errorf("expected total 3 rare points, got gained=%d total=%d", res3.RarePointsGained, res3.TotalRarePoints)
	}
	dep, _ = depotRepo.FindByCharacterID(ctx, "char-depot-stack")
	if len(dep.Items) != 0 {
		t.Errorf("expected depot to be empty, got %d items", len(dep.Items))
	}

	// Fourth sacrifice: fails because item is no longer in depot
	_, err = svc.SacrificeItem(ctx, "char-depot-stack", "inst-rare-stack")
	if !errors.Is(err, blackmarket.ErrUnownedItem) {
		t.Errorf("expected ErrUnownedItem, got %v", err)
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
	if res.ItemName != "時の砂" {
		t.Errorf("expected item name 時の砂, got %s", res.ItemName)
	}
	if res.Message != "取引成立だ…。時の砂 はお前の預かり所に送っておいた…" {
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

func TestPrizeItemDefinitionsParity(t *testing.T) {
	ctx := context.Background()
	itemCatalog, err := coreitem.InitialCatalog()
	if err != nil {
		t.Fatalf("failed to load item catalog: %v", err)
	}

	bmCatalog, err := blackmarket.LoadDefaultCatalog()
	if err != nil {
		t.Fatalf("failed to load black market catalog: %v", err)
	}

	prizes := bmCatalog.Prizes()
	if len(prizes) != 24 {
		t.Fatalf("expected 24 prizes, got %d", len(prizes))
	}

	// 1. Verify every prize has name matching Core item definition and empty description
	for _, p := range prizes {
		def, err := itemCatalog.FindByID(p.ItemDefinitionID)
		if err != nil {
			t.Errorf("prize %s refers to unknown core item %s: %v", p.ID, p.ItemDefinitionID, err)
			continue
		}
		if p.Name != def.Name {
			t.Errorf("prize %s name mismatch: catalog has %q, core definition has %q", p.ID, p.Name, def.Name)
		}
		if p.Description != "" {
			t.Errorf("prize %s description should be empty, got %q", p.ID, p.Description)
		}
	}

	// 2. Verify GetStatus exposes resolved core item names
	charRepo := newMockCharacterRepo()
	invRepo := newMockInventoryRepo()
	depotRepo := newMockDepotRepo()
	bmRepo := newMockBlackMarketRepo()

	svc, err := blackmarket.NewService(
		charRepo,
		invRepo,
		bmRepo,
		bmCatalog,
		blackmarket.WithDepotRepository(depotRepo),
		blackmarket.WithItemDefinitionProvider(itemCatalog),
		blackmarket.WithTransactionProvider(&mockTxProvider{}),
	)
	if err != nil {
		t.Fatalf("failed to create black market service: %v", err)
	}

	char := corecharacter.Character{ID: "char-parity-test", Name: "Parity Hero", JobLevel: 10}
	_ = charRepo.Update(ctx, char)

	status, err := svc.GetStatus(ctx, "char-parity-test")
	if err != nil {
		t.Fatalf("GetStatus error: %v", err)
	}

	for _, p := range append(status.Prizes, status.UPrizes...) {
		def, err := itemCatalog.FindByID(p.ItemDefinitionID)
		if err != nil {
			t.Errorf("status prize %s refers to unknown core item %s: %v", p.ID, p.ItemDefinitionID, err)
			continue
		}
		if p.Name != def.Name {
			t.Errorf("status prize %s name %q does not match core item definition %q", p.ID, p.Name, def.Name)
		}
	}

	// 3. Verify representative trades and depot delivery
	bmRepo.points["char-parity-test"] = blackmarket.CharacterPoints{
		CharacterID: "char-parity-test",
		RarePoints:  100,
		URarePoints: 100,
	}

	testCases := []struct {
		prizeID       string
		expectedDefID string
		expectedName  string
	}{
		{"bm_prize_087", "item-087", "時の砂"},
		{"bm_prize_057", "item-057", "気合の霊薬"},
		{"bm_prize_207", "item-207", "未来のカケラ"},
		{"bm_uprize_059", "item-059", "天馬のたづな"},
		{"bm_uprize_262", "item-262", "黒い天馬のたづな"},
	}

	for _, tc := range testCases {
		res, err := svc.TradePrize(ctx, "char-parity-test", tc.prizeID)
		if err != nil {
			t.Fatalf("TradePrize(%s) failed: %v", tc.prizeID, err)
		}
		if res.ItemDefinitionID != tc.expectedDefID {
			t.Errorf("expected def ID %s, got %s", tc.expectedDefID, res.ItemDefinitionID)
		}
		if res.ItemName != tc.expectedName {
			t.Errorf("expected trade result ItemName %q, got %q", tc.expectedName, res.ItemName)
		}
		expectedMsg := "取引成立だ…。" + tc.expectedName + " はお前の預かり所に送っておいた…"
		if res.Message != expectedMsg {
			t.Errorf("expected trade message %q, got %q", expectedMsg, res.Message)
		}
	}

	dep, err := depotRepo.FindByCharacterID(ctx, "char-parity-test")
	if err != nil {
		t.Fatalf("FindByCharacterID error: %v", err)
	}
	if len(dep.Items) != len(testCases) {
		t.Fatalf("expected %d items in depot, got %d", len(testCases), len(dep.Items))
	}
	for i, tc := range testCases {
		if dep.Items[i].DefinitionID != tc.expectedDefID {
			t.Errorf("depot item %d has definition ID %s, expected %s", i, dep.Items[i].DefinitionID, tc.expectedDefID)
		}
	}
}
