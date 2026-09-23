package plantation_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/depot"
	"github.com/witchcraze/party2re/internal/plantation"
)

type mockRNG struct {
	values []int
	idx    int
}

func (m *mockRNG) Intn(n int) int {
	if len(m.values) == 0 {
		return 0
	}
	v := m.values[m.idx%len(m.values)]
	m.idx++
	if n <= 0 {
		return 0
	}
	return v % n
}

type mockCharRepo struct {
	chars map[string]corecharacter.Character
}

func newMockCharRepo() *mockCharRepo {
	return &mockCharRepo{chars: make(map[string]corecharacter.Character)}
}

func (m *mockCharRepo) FindByID(_ context.Context, id string) (corecharacter.Character, error) {
	c, ok := m.chars[id]
	if !ok {
		return corecharacter.Character{}, errors.New("character not found")
	}
	return c, nil
}

func (m *mockCharRepo) FindByIDForUpdate(ctx context.Context, id string) (corecharacter.Character, error) {
	return m.FindByID(ctx, id)
}

func (m *mockCharRepo) Update(_ context.Context, c corecharacter.Character) error {
	m.chars[c.ID] = c
	return nil
}

type mockInvRepo struct {
	invs map[string]coreinventory.Inventory
}

func newMockInvRepo() *mockInvRepo {
	return &mockInvRepo{invs: make(map[string]coreinventory.Inventory)}
}

func (m *mockInvRepo) FindByCharacterID(_ context.Context, characterID string) (coreinventory.Inventory, error) {
	inv, ok := m.invs[characterID]
	if !ok {
		return coreinventory.New(characterID)
	}
	return inv, nil
}

func (m *mockInvRepo) FindByCharacterIDForUpdate(ctx context.Context, characterID string) (coreinventory.Inventory, error) {
	return m.FindByCharacterID(ctx, characterID)
}

func (m *mockInvRepo) Save(_ context.Context, inv coreinventory.Inventory) error {
	m.invs[inv.CharacterID] = inv
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
		dep, _ := depot.NewDepot(characterID)
		dep.Capacity = 50
		return dep, nil
	}
	return d, nil
}

func (m *mockDepotRepo) FindByCharacterIDForUpdate(ctx context.Context, characterID string) (depot.Depot, error) {
	return m.FindByCharacterID(ctx, characterID)
}

func (m *mockDepotRepo) Save(_ context.Context, d depot.Depot) error {
	m.depots[d.CharacterID] = d
	return nil
}

type mockPlotRepo struct {
	plots map[string]plantation.Plot
}

func newMockPlotRepo() *mockPlotRepo {
	return &mockPlotRepo{plots: make(map[string]plantation.Plot)}
}

func (m *mockPlotRepo) GetPlot(_ context.Context, characterID string) (plantation.Plot, error) {
	p, ok := m.plots[characterID]
	if !ok {
		return plantation.Plot{}, plantation.ErrPlotNotFound
	}
	return p, nil
}

func (m *mockPlotRepo) GetPlotForUpdate(ctx context.Context, characterID string) (plantation.Plot, error) {
	return m.GetPlot(ctx, characterID)
}

func (m *mockPlotRepo) SavePlot(_ context.Context, p plantation.Plot) error {
	m.plots[p.CharacterID] = p
	return nil
}

func (m *mockPlotRepo) DeletePlot(_ context.Context, characterID string) error {
	delete(m.plots, characterID)
	return nil
}

func setupTestHarness(t *testing.T) (*plantation.Service, *mockCharRepo, *mockInvRepo, *mockDepotRepo, *mockPlotRepo) {
	charRepo := newMockCharRepo()
	invRepo := newMockInvRepo()
	depotRepo := newMockDepotRepo()
	plotRepo := newMockPlotRepo()

	itemDefs, err := coreitem.InitialCatalog()
	if err != nil {
		t.Fatalf("failed to load initial item catalog: %v", err)
	}

	fixedNow := time.Date(2026, 9, 12, 10, 0, 0, 0, time.UTC)
	svc, err := plantation.NewService(
		charRepo,
		invRepo,
		depotRepo,
		plotRepo,
		itemDefs,
		plantation.WithNow(func() time.Time { return fixedNow }),
	)
	if err != nil {
		t.Fatalf("failed to construct service: %v", err)
	}

	return svc, charRepo, invRepo, depotRepo, plotRepo
}

func TestCatalogs(t *testing.T) {
	seeds := plantation.AllSeeds()
	if len(seeds) != 6 {
		t.Fatalf("expected 6 seeds, got %d", len(seeds))
	}

	ferts := plantation.AllFertilizers()
	if len(ferts) != 14 {
		t.Fatalf("expected 14 selectable fertilizers, got %d", len(ferts))
	}

	s, ok := plantation.FindSeed("red")
	if !ok || s.Name != "赤の種" || s.Price != 50 {
		t.Fatalf("seed 'red' lookup failed: %+v", s)
	}

	f, ok := plantation.FindFertilizer("magic_powder")
	if !ok || f.Name != "魔法の粉" || !f.IsItem || f.ItemID != "item-081" {
		t.Fatalf("fertilizer 'magic_powder' lookup failed: %+v", f)
	}

	defaultF := plantation.GetDefaultFertilizer()
	if defaultF.ID != "none" || defaultF.WitherRate != 10 {
		t.Fatalf("default fertilizer mismatch: %+v", defaultF)
	}
}

func TestCleanRoomFertilizerNames(t *testing.T) {
	testCases := []struct {
		id         string
		cleanName  string
		legacyName string
		itemID     string
	}{
		{id: "yggdrasil_dew", cleanName: "霊樹のしずく", legacyName: "世界樹のしずく", itemID: "item-005"},
		{id: "gysahl_greens", cleanName: "走鳥の野菜", legacyName: "ギザールの野菜", itemID: "item-037"},
		{id: "kupo_nut", cleanName: "幻獣の実", legacyName: "クポの実", itemID: "item-038"},
		{id: "padekia_root", cleanName: "薬草の根っこ", legacyName: "パデキアの根っこ", itemID: "item-010"},
	}

	for _, tc := range testCases {
		fert, ok := plantation.FindFertilizer(tc.id)
		if !ok {
			t.Fatalf("fertilizer %s not found", tc.id)
		}
		if fert.Name != tc.cleanName {
			t.Errorf("fertilizer %s Name: expected %q, got %q", tc.id, tc.cleanName, fert.Name)
		}
		if fert.ItemID != tc.itemID {
			t.Errorf("fertilizer %s ItemID: expected %q, got %q", tc.id, tc.itemID, fert.ItemID)
		}

		// Lookup by clean-room name should succeed
		byClean, okClean := plantation.FindFertilizer(tc.cleanName)
		if !okClean || byClean.ID != tc.id {
			t.Errorf("lookup by clean name %q failed: %+v", tc.cleanName, byClean)
		}

		// Lookup by legacy copyrighted name should fail
		if _, okLegacy := plantation.FindFertilizer(tc.legacyName); okLegacy {
			t.Errorf("lookup by legacy copyrighted name %q should fail, but succeeded", tc.legacyName)
		}
	}
}

func TestGetStatus(t *testing.T) {
	svc, charRepo, _, _, plotRepo := setupTestHarness(t)
	ctx := context.Background()
	charID := "c1"
	charRepo.chars[charID] = corecharacter.Character{ID: charID, Name: "Hero", Money: 1000}

	// Case 1: No plot
	st, err := svc.GetStatus(ctx, charID)
	if err != nil {
		t.Fatalf("GetStatus error: %v", err)
	}
	if st.Status != plantation.StatusNone || st.Plot != nil {
		t.Fatalf("expected StatusNone, got %v", st.Status)
	}

	// Case 2: Growing
	now := time.Date(2026, 9, 12, 10, 0, 0, 0, time.UTC)
	matures := timer.NextMidnightJST(now)
	plotRepo.plots[charID] = plantation.Plot{
		CharacterID: charID,
		SeedID:      "red",
		SownAt:      now,
		MaturesAt:   matures,
	}

	st, err = svc.GetStatus(ctx, charID)
	if err != nil {
		t.Fatalf("GetStatus error: %v", err)
	}
	if st.Status != plantation.StatusGrowing || st.Plot == nil {
		t.Fatalf("expected StatusGrowing, got %v", st.Status)
	}

	// Case 3: Ready (matured)
	plotRepo.plots[charID] = plantation.Plot{
		CharacterID: charID,
		SeedID:      "red",
		SownAt:      now.Add(-24 * time.Hour),
		MaturesAt:   now.Add(-1 * time.Hour),
	}
	st, err = svc.GetStatus(ctx, charID)
	if err != nil {
		t.Fatalf("GetStatus error: %v", err)
	}
	if st.Status != plantation.StatusReady {
		t.Fatalf("expected StatusReady, got %v", st.Status)
	}
}

func TestSow(t *testing.T) {
	svc, charRepo, _, _, plotRepo := setupTestHarness(t)
	ctx := context.Background()
	charID := "c1"
	charRepo.chars[charID] = corecharacter.Character{ID: charID, Name: "Hero", Money: 100}

	// Invalid seed
	if _, err := svc.Sow(ctx, charID, "invalid"); !errors.Is(err, plantation.ErrInvalidSeedID) {
		t.Fatalf("expected ErrInvalidSeedID, got %v", err)
	}

	// Insufficient gold (gold seed costs 500)
	if _, err := svc.Sow(ctx, charID, "gold"); !errors.Is(err, plantation.ErrInsufficientGold) {
		t.Fatalf("expected ErrInsufficientGold, got %v", err)
	}

	// Successful sow (red seed costs 50)
	res, err := svc.Sow(ctx, charID, "red")
	if err != nil {
		t.Fatalf("Sow failed: %v", err)
	}
	if res.Plot.SeedID != "red" {
		t.Errorf("expected seed red, got %s", res.Plot.SeedID)
	}
	if charRepo.chars[charID].Money != 50 {
		t.Errorf("expected wallet 50, got %d", charRepo.chars[charID].Money)
	}
	if _, exists := plotRepo.plots[charID]; !exists {
		t.Fatal("expected plot in repository")
	}

	// Already sown
	if _, err := svc.Sow(ctx, charID, "red"); !errors.Is(err, plantation.ErrPlotAlreadySown) {
		t.Fatalf("expected ErrPlotAlreadySown, got %v", err)
	}
}

func TestFertilize(t *testing.T) {
	svc, charRepo, invRepo, depotRepo, plotRepo := setupTestHarness(t)
	ctx := context.Background()
	charID := "c1"
	charRepo.chars[charID] = corecharacter.Character{ID: charID, Name: "Hero", Money: 1000}

	now := time.Date(2026, 9, 12, 10, 0, 0, 0, time.UTC)
	plotRepo.plots[charID] = plantation.Plot{
		CharacterID: charID,
		SeedID:      "red",
		SownAt:      now,
		MaturesAt:   timer.NextMidnightJST(now),
	}

	// 1. Gold fertilizer: chemical (costs 500)
	res, err := svc.Fertilize(ctx, charID, "chemical")
	if err != nil {
		t.Fatalf("Fertilize with chemical failed: %v", err)
	}
	if *res.Plot.FertilizerID != "chemical" {
		t.Errorf("expected fertilizer chemical, got %v", res.Plot.FertilizerID)
	}
	if charRepo.chars[charID].Money != 500 {
		t.Errorf("expected wallet 500, got %d", charRepo.chars[charID].Money)
	}

	// 2. Already applied fertilizer
	if _, err := svc.Fertilize(ctx, charID, "bone_meal"); !errors.Is(err, plantation.ErrFertilizerAlreadyApplied) {
		t.Fatalf("expected ErrFertilizerAlreadyApplied, got %v", err)
	}

	// 3. Item fertilizer from Depot
	charID2 := "c2"
	charRepo.chars[charID2] = corecharacter.Character{ID: charID2, Name: "Hero2", Money: 100}
	plotRepo.plots[charID2] = plantation.Plot{
		CharacterID: charID2,
		SeedID:      "gold",
		SownAt:      now,
		MaturesAt:   timer.NextMidnightJST(now),
	}

	dep, _ := depot.NewDepot(charID2)
	dep.Capacity = 50
	magicPowder, _ := coreitem.NewInstance("item-081", 2)
	_ = dep.AddItem(magicPowder)
	_ = depotRepo.Save(ctx, dep)

	res2, err := svc.Fertilize(ctx, charID2, "magic_powder")
	if err != nil {
		t.Fatalf("Fertilize with magic_powder failed: %v", err)
	}
	if *res2.Plot.FertilizerID != "magic_powder" {
		t.Errorf("expected fertilizer magic_powder, got %v", res2.Plot.FertilizerID)
	}
	// Verify depot quantity decremented from 2 to 1
	d2, _ := depotRepo.FindByCharacterID(ctx, charID2)
	if d2.Quantity("item-081") != 1 {
		t.Errorf("expected 1 magic_powder in depot, got %d", d2.Quantity("item-081"))
	}

	// 4. Item fertilizer from Inventory
	charID3 := "c3"
	charRepo.chars[charID3] = corecharacter.Character{ID: charID3, Name: "Hero3", Money: 100}
	plotRepo.plots[charID3] = plantation.Plot{
		CharacterID: charID3,
		SeedID:      "silver",
		SownAt:      now,
		MaturesAt:   timer.NextMidnightJST(now),
	}

	inv, _ := coreinventory.New(charID3)
	horseDung, _ := coreitem.NewInstance("item-130", 1)
	_ = inv.Add(horseDung)
	_ = invRepo.Save(ctx, inv)

	res3, err := svc.Fertilize(ctx, charID3, "horse_dung")
	if err != nil {
		t.Fatalf("Fertilize with horse_dung failed: %v", err)
	}
	if *res3.Plot.FertilizerID != "horse_dung" {
		t.Errorf("expected horse_dung, got %v", res3.Plot.FertilizerID)
	}
	inv3, _ := invRepo.FindByCharacterID(ctx, charID3)
	if inv3.Quantity("item-130") != 0 {
		t.Errorf("expected 0 horse_dung in inventory, got %d", inv3.Quantity("item-130"))
	}

	// 5. Item fertilizer missing
	charID4 := "c4"
	charRepo.chars[charID4] = corecharacter.Character{ID: charID4, Name: "Hero4", Money: 100}
	plotRepo.plots[charID4] = plantation.Plot{
		CharacterID: charID4,
		SeedID:      "blue",
		SownAt:      now,
		MaturesAt:   timer.NextMidnightJST(now),
	}
	if _, err := svc.Fertilize(ctx, charID4, "kupo_nut"); !errors.Is(err, plantation.ErrMissingFertilizerItem) {
		t.Fatalf("expected ErrMissingFertilizerItem, got %v", err)
	}

	// 6. Fertilize with clean-room item from Depot using clean-room name
	charID5 := "c5"
	charRepo.chars[charID5] = corecharacter.Character{ID: charID5, Name: "Hero5", Money: 100}
	plotRepo.plots[charID5] = plantation.Plot{
		CharacterID: charID5,
		SeedID:      "green",
		SownAt:      now,
		MaturesAt:   timer.NextMidnightJST(now),
	}
	dep5, _ := depot.NewDepot(charID5)
	dep5.Capacity = 50
	cleanItem, _ := coreitem.NewInstance("item-037", 1) // 走鳥の野菜
	_ = dep5.AddItem(cleanItem)
	_ = depotRepo.Save(ctx, dep5)

	res5, err := svc.Fertilize(ctx, charID5, "走鳥の野菜")
	if err != nil {
		t.Fatalf("Fertilize with clean-room name 走鳥の野菜 failed: %v", err)
	}
	if *res5.Plot.FertilizerID != "gysahl_greens" {
		t.Errorf("expected fertilizer gysahl_greens, got %v", res5.Plot.FertilizerID)
	}
	if res5.Message != "走鳥の野菜をまくよ！" {
		t.Errorf("expected message '走鳥の野菜をまくよ！', got %q", res5.Message)
	}
	d5, _ := depotRepo.FindByCharacterID(ctx, charID5)
	if d5.Quantity("item-037") != 0 {
		t.Errorf("expected 0 item-037 in depot, got %d", d5.Quantity("item-037"))
	}
}

func TestHarvest_NotMatured(t *testing.T) {
	svc, charRepo, _, _, plotRepo := setupTestHarness(t)
	ctx := context.Background()
	charID := "c1"
	charRepo.chars[charID] = corecharacter.Character{ID: charID, Name: "Hero"}

	now := time.Date(2026, 9, 12, 10, 0, 0, 0, time.UTC)
	plotRepo.plots[charID] = plantation.Plot{
		CharacterID: charID,
		SeedID:      "red",
		SownAt:      now,
		MaturesAt:   now.Add(5 * time.Hour), // Still growing
	}

	_, err := svc.Harvest(ctx, charID)
	if !errors.Is(err, plantation.ErrCropNotMatured) {
		t.Fatalf("expected ErrCropNotMatured, got %v", err)
	}
}

func TestHarvest_Wither(t *testing.T) {
	charRepo := newMockCharRepo()
	invRepo := newMockInvRepo()
	depotRepo := newMockDepotRepo()
	plotRepo := newMockPlotRepo()
	itemDefs, _ := coreitem.InitialCatalog()

	charID := "c1"
	charRepo.chars[charID] = corecharacter.Character{ID: charID, Name: "Hero"}

	now := time.Date(2026, 9, 12, 10, 0, 0, 0, time.UTC)
	// Chemical fertilizer has 35% wither rate
	fertID := "chemical"
	plotRepo.plots[charID] = plantation.Plot{
		CharacterID:  charID,
		SeedID:       "red",
		FertilizerID: &fertID,
		SownAt:       now.Add(-24 * time.Hour),
		MaturesAt:    now.Add(-1 * time.Hour),
	}

	// RNG roll 5 < 35 -> triggers wither
	rng := &mockRNG{values: []int{5}}
	svc, _ := plantation.NewService(
		charRepo, invRepo, depotRepo, plotRepo, itemDefs,
		plantation.WithNow(func() time.Time { return now }),
		plantation.WithRNG(rng),
	)

	res, err := svc.Harvest(context.Background(), charID)
	if err != nil {
		t.Fatalf("Harvest failed: %v", err)
	}
	if !res.Withered {
		t.Fatal("expected crop to wither")
	}
	if len(res.Yields) != 0 {
		t.Errorf("expected 0 yields, got %d", len(res.Yields))
	}
	// Verify plot deleted
	if _, exists := plotRepo.plots[charID]; exists {
		t.Fatal("expected plot to be deleted upon wither")
	}
}

func TestHarvest_SuccessAndDepotDeposit(t *testing.T) {
	charRepo := newMockCharRepo()
	invRepo := newMockInvRepo()
	depotRepo := newMockDepotRepo()
	plotRepo := newMockPlotRepo()
	itemDefs, _ := coreitem.InitialCatalog()

	charID := "c1"
	charRepo.chars[charID] = corecharacter.Character{ID: charID, Name: "Hero"}

	now := time.Date(2026, 9, 12, 10, 0, 0, 0, time.UTC)
	fertID := "magic_powder" // 0% wither, yield bonus 2, prob bonus 28
	plotRepo.plots[charID] = plantation.Plot{
		CharacterID:  charID,
		SeedID:       "gold", // High: items 21..22, Low: item 1, HighRate: 18%
		FertilizerID: &fertID,
		SownAt:       now.Add(-24 * time.Hour),
		MaturesAt:    now.Add(-1 * time.Hour),
	}

	// RNG sequence:
	// 1. wither roll: 50 (>= 0 -> success)
	// 2. yield bonus roll: 1 (extra 1 item -> total 2 items)
	// Item 1:
	// 3. high roll: 10 (< 18 + 28 = 46 -> High!)
	// 4. seed.HighRand offset: 1 -> item 21 + 1 = 22 (幸せの種 / item-022)
	// Item 2:
	// 5. high roll: 80 (>= 46 -> Low!)
	// (Gold LowRand is 0, so item 1 / 薬草 / item-001)
	rng := &mockRNG{values: []int{50, 1, 10, 1, 80}}
	svc, _ := plantation.NewService(
		charRepo, invRepo, depotRepo, plotRepo, itemDefs,
		plantation.WithNow(func() time.Time { return now }),
		plantation.WithRNG(rng),
	)

	res, err := svc.Harvest(context.Background(), charID)
	if err != nil {
		t.Fatalf("Harvest failed: %v", err)
	}
	if res.Withered {
		t.Fatal("expected crop NOT to wither")
	}
	if len(res.Yields) != 2 {
		t.Fatalf("expected 2 yields, got %d", len(res.Yields))
	}

	// Verify items delivered to Depot
	dep, _ := depotRepo.FindByCharacterID(context.Background(), charID)
	if dep.Quantity("item-022") != 1 {
		t.Errorf("expected 1 item-022 in depot, got %d", dep.Quantity("item-022"))
	}
	if dep.Quantity("item-001") != 1 {
		t.Errorf("expected 1 item-001 in depot, got %d", dep.Quantity("item-001"))
	}

	// Verify plot deleted
	if _, exists := plotRepo.plots[charID]; exists {
		t.Fatal("expected plot to be deleted upon harvest")
	}
}

func TestHarvest_DepotFull(t *testing.T) {
	charRepo := newMockCharRepo()
	invRepo := newMockInvRepo()
	depotRepo := newMockDepotRepo()
	plotRepo := newMockPlotRepo()
	itemDefs, _ := coreitem.InitialCatalog()

	charID := "c1"
	charRepo.chars[charID] = corecharacter.Character{ID: charID, Name: "Hero"}

	now := time.Date(2026, 9, 12, 10, 0, 0, 0, time.UTC)
	fertID := "magic_powder"
	plotRepo.plots[charID] = plantation.Plot{
		CharacterID:  charID,
		SeedID:       "red",
		FertilizerID: &fertID,
		SownAt:       now.Add(-24 * time.Hour),
		MaturesAt:    now.Add(-1 * time.Hour),
	}

	// Depot filled to capacity (5 slots for JobLevel 0)
	dep, _ := depot.NewDepot(charID)
	for i := 1; i <= 5; i++ {
		inst, _ := coreitem.NewInstance(fmt.Sprintf("armor-%02d", i), 1)
		_ = dep.AddItem(inst)
	}
	_ = depotRepo.Save(context.Background(), dep)

	rng := &mockRNG{values: []int{50, 0, 50, 0}}
	svc, _ := plantation.NewService(
		charRepo, invRepo, depotRepo, plotRepo, itemDefs,
		plantation.WithNow(func() time.Time { return now }),
		plantation.WithRNG(rng),
	)

	_, err := svc.Harvest(context.Background(), charID)
	if !errors.Is(err, depot.ErrDepotFull) {
		t.Fatalf("expected ErrDepotFull, got %v", err)
	}

	// Plot must NOT be deleted when depot is full
	if _, exists := plotRepo.plots[charID]; !exists {
		t.Fatal("plot should be preserved if depot is full")
	}
}
