package eventplaza

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/depot"
	"github.com/witchcraze/party2re/internal/id"
)

type mockClock struct {
	now time.Time
}

func (m *mockClock) Now() time.Time {
	return m.now
}

type memoryRepo struct {
	mu        sync.Mutex
	presences map[string]time.Time
	banquets  map[string]CelebrationBanquet
	toasts    map[string]map[string]time.Time
}

func newMemoryRepo() *memoryRepo {
	return &memoryRepo{
		presences: make(map[string]time.Time),
		banquets:  make(map[string]CelebrationBanquet),
		toasts:    make(map[string]map[string]time.Time),
	}
}

func (m *memoryRepo) RecordPresence(ctx context.Context, characterID string, at time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.presences[characterID] = at
	return nil
}

func (m *memoryRepo) CountActiveParticipants(ctx context.Context, cutoff time.Time) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	count := 0
	for _, lastSeen := range m.presences {
		if !lastSeen.Before(cutoff) {
			count++
		}
	}
	return count, nil
}

func (m *memoryRepo) SaveBanquet(ctx context.Context, banquet CelebrationBanquet) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.banquets[banquet.ID] = banquet
	return nil
}

func (m *memoryRepo) FindBanquetByID(ctx context.Context, id string) (CelebrationBanquet, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	b, ok := m.banquets[id]
	if !ok {
		return CelebrationBanquet{}, ErrBanquetNotFound
	}
	return b, nil
}

func (m *memoryRepo) ListActiveBanquets(ctx context.Context, now time.Time) ([]CelebrationBanquet, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var list []CelebrationBanquet
	for _, b := range m.banquets {
		if now.Before(b.ExpiresAt) {
			list = append(list, b)
		}
	}
	return list, nil
}

func (m *memoryRepo) RecordToast(ctx context.Context, banquetID string, characterID string, toastedAt time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	b, ok := m.banquets[banquetID]
	if !ok {
		return ErrBanquetNotFound
	}
	if m.toasts[banquetID] == nil {
		m.toasts[banquetID] = make(map[string]time.Time)
	}
	if _, exists := m.toasts[banquetID][characterID]; exists {
		return ErrAlreadyToasted
	}
	m.toasts[banquetID][characterID] = toastedAt
	b.ToastCount++
	m.banquets[banquetID] = b
	return nil
}

func (m *memoryRepo) HasToasted(ctx context.Context, banquetID string, characterID string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.toasts[banquetID] == nil {
		return false, nil
	}
	_, exists := m.toasts[banquetID][characterID]
	return exists, nil
}

type memoryCharacterRepo struct {
	mu         sync.Mutex
	characters map[string]corecharacter.Character
}

func newMemoryCharacterRepo() *memoryCharacterRepo {
	return &memoryCharacterRepo{
		characters: make(map[string]corecharacter.Character),
	}
}

func (m *memoryCharacterRepo) FindByID(ctx context.Context, id string) (corecharacter.Character, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.characters[id]
	if !ok {
		return corecharacter.Character{}, errors.New("character not found")
	}
	return c, nil
}

func (m *memoryCharacterRepo) FindByIDForUpdate(ctx context.Context, id string) (corecharacter.Character, error) {
	return m.FindByID(ctx, id)
}

func (m *memoryCharacterRepo) Update(ctx context.Context, value corecharacter.Character) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.characters[value.ID] = value
	return nil
}

type memoryInventoryRepo struct {
	mu          sync.Mutex
	inventories map[string]coreinventory.Inventory
}

func newMemoryInventoryRepo() *memoryInventoryRepo {
	return &memoryInventoryRepo{
		inventories: make(map[string]coreinventory.Inventory),
	}
}

func (m *memoryInventoryRepo) FindByCharacterID(ctx context.Context, characterID string) (coreinventory.Inventory, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	inv, ok := m.inventories[characterID]
	if !ok {
		inv, _ = coreinventory.New(characterID)
	}
	return inv, nil
}

func (m *memoryInventoryRepo) FindByCharacterIDForUpdate(ctx context.Context, characterID string) (coreinventory.Inventory, error) {
	return m.FindByCharacterID(ctx, characterID)
}

func (m *memoryInventoryRepo) Save(ctx context.Context, value coreinventory.Inventory) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.inventories[value.CharacterID] = value
	return nil
}

type memoryDepotRepo struct {
	mu     sync.Mutex
	depots map[string]depot.Depot
}

func newMemoryDepotRepo() *memoryDepotRepo {
	return &memoryDepotRepo{
		depots: make(map[string]depot.Depot),
	}
}

func (m *memoryDepotRepo) FindByCharacterID(ctx context.Context, characterID string) (depot.Depot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	d, ok := m.depots[characterID]
	if !ok {
		return depot.Depot{}, depot.ErrNotFound
	}
	return d, nil
}

func (m *memoryDepotRepo) FindByCharacterIDForUpdate(ctx context.Context, characterID string) (depot.Depot, error) {
	return m.FindByCharacterID(ctx, characterID)
}

func (m *memoryDepotRepo) Save(ctx context.Context, value depot.Depot) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.depots[value.CharacterID] = value
	return nil
}

type mockHelperProvider struct {
	activeIDs []string
}

func (m *mockHelperProvider) GetActiveHelperItemIDs(ctx context.Context, now time.Time) ([]string, error) {
	return m.activeIDs, nil
}

type mockCollectionRecorder struct {
	mu       sync.Mutex
	recorded []string
}

func (m *mockCollectionRecorder) RecordItemDiscovered(ctx context.Context, characterID, itemID, itemName, category string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.recorded = append(m.recorded, itemID)
	return nil
}

func populatePresences(repo *memoryRepo, count int, at time.Time) {
	for i := 1; i <= count; i++ {
		charID := fmt.Sprintf("presence-char-%03d", i)
		_ = repo.RecordPresence(context.Background(), charID, at)
	}
}

func TestCatalogLoading_ParityWithLegacyCGI(t *testing.T) {
	catalog, err := LoadDefaultBazaarCatalog()
	if err != nil {
		t.Fatalf("unexpected error loading catalog: %v", err)
	}
	if len(catalog) != 26 {
		t.Fatalf("expected exactly 26 items in authentic bazaar catalog, got %d", len(catalog))
	}

	tier1Items := map[string]int{
		"item-072": 2400, // 800 * 3
		"item-081": 3600, // 1200 * 3
		"item-082": 4800, // 1600 * 3
		"item-083": 5100, // 1700 * 3
		"item-084": 2100, // 700 * 3
		"item-086": 3000, // 1000 * 3
	}

	tier2Items := map[string]int{
		"item-073": 3000,  // 1000 * 3
		"item-074": 3600,  // 1200 * 3
		"item-077": 15000, // 5000 * 3
		"item-005": 15000, // 5000 * 3
		"item-075": 4500,  // 1500 * 3
		"item-085": 10800, // 3600 * 3
	}

	tier3Items := map[string]int{
		"item-090": 600,   // 200 * 3
		"item-091": 600,   // 200 * 3
		"item-092": 540,   // 180 * 3
		"item-093": 450,   // 150 * 3
		"item-094": 600,   // 200 * 3
		"item-095": 600,   // 200 * 3
		"item-096": 540,   // 180 * 3
		"item-097": 540,   // 180 * 3
		"item-098": 450,   // 150 * 3
		"item-099": 600,   // 200 * 3
		"item-100": 600,   // 200 * 3
		"item-108": 60000, // 20000 * 3
		"item-142": 450,   // 150 * 3
		"item-217": 1500,  // 500 * 3
	}

	for _, it := range catalog {
		switch it.TierRequired {
		case 1:
			expectedPrice, exists := tier1Items[it.ItemDefinitionID]
			if !exists {
				t.Errorf("unexpected tier 1 item: %s", it.ItemDefinitionID)
			} else if it.Price != expectedPrice {
				t.Errorf("item %s expected price %d, got %d", it.ItemDefinitionID, expectedPrice, it.Price)
			}
		case 2:
			expectedPrice, exists := tier2Items[it.ItemDefinitionID]
			if !exists {
				t.Errorf("unexpected tier 2 item: %s", it.ItemDefinitionID)
			} else if it.Price != expectedPrice {
				t.Errorf("item %s expected price %d, got %d", it.ItemDefinitionID, expectedPrice, it.Price)
			}
		case 3:
			expectedPrice, exists := tier3Items[it.ItemDefinitionID]
			if !exists {
				t.Errorf("unexpected tier 3 item: %s", it.ItemDefinitionID)
			} else if it.Price != expectedPrice {
				t.Errorf("item %s expected price %d, got %d", it.ItemDefinitionID, expectedPrice, it.Price)
			}
		default:
			t.Errorf("unexpected tier %d for item %s", it.TierRequired, it.ID)
		}
	}
}

func TestCalculateMerchantTier(t *testing.T) {
	tests := []struct {
		population   int
		expectedTier int
		expectedNext int
	}{
		{0, 0, 10},
		{5, 0, 10},
		{9, 0, 10},
		{10, 1, 20},
		{15, 1, 20},
		{19, 1, 20},
		{20, 2, 30},
		{25, 2, 30},
		{29, 2, 30},
		{30, 3, 0},
		{50, 3, 0},
	}

	for _, tt := range tests {
		tier, _, next := CalculateMerchantTier(tt.population)
		if tier != tt.expectedTier {
			t.Errorf("for population %d, expected tier %d, got %d", tt.population, tt.expectedTier, tier)
		}
		if next != tt.expectedNext {
			t.Errorf("for population %d, expected next %d, got %d", tt.population, tt.expectedNext, next)
		}
	}
}

func TestRealTimePresence_AndPlazaStatus(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	clock := &mockClock{now: now}

	repo := newMemoryRepo()
	charRepo := newMemoryCharacterRepo()
	invRepo := newMemoryInventoryRepo()

	svc, err := NewService(repo, charRepo, invRepo, WithClock(clock))
	if err != nil {
		t.Fatalf("failed to init service: %v", err)
	}

	// 1. Initial empty plaza -> Tier 0
	status, err := svc.GetPlazaStatus(ctx)
	if err != nil {
		t.Fatalf("GetPlazaStatus failed: %v", err)
	}
	if status.ActiveParticipants != 0 || status.MerchantTier != 0 {
		t.Errorf("expected 0 participants and tier 0, got %d / %d", status.ActiveParticipants, status.MerchantTier)
	}

	// 2. 12 characters register presence within window -> Tier 1
	populatePresences(repo, 12, now)
	status, err = svc.GetPlazaStatus(ctx)
	if err != nil {
		t.Fatalf("GetPlazaStatus failed: %v", err)
	}
	if status.ActiveParticipants != 12 || status.MerchantTier != 1 {
		t.Errorf("expected 12 participants and tier 1, got %d / %d", status.ActiveParticipants, status.MerchantTier)
	}

	// 3. Advance clock by 6 minutes (exceeding PresenceWindow 5 min) -> Members expire -> Tier 0
	clock.now = now.Add(6 * time.Minute)
	status, err = svc.GetPlazaStatus(ctx)
	if err != nil {
		t.Fatalf("GetPlazaStatus failed: %v", err)
	}
	if status.ActiveParticipants != 0 || status.MerchantTier != 0 {
		t.Errorf("expected 0 active participants after 6m, got %d / %d", status.ActiveParticipants, status.MerchantTier)
	}

	// 4. Record presence for 25 characters at new time -> Tier 2
	populatePresences(repo, 25, clock.now)
	status, err = svc.GetPlazaStatus(ctx)
	if err != nil {
		t.Fatalf("GetPlazaStatus failed: %v", err)
	}
	if status.ActiveParticipants != 25 || status.MerchantTier != 2 {
		t.Errorf("expected 25 participants and tier 2, got %d / %d", status.ActiveParticipants, status.MerchantTier)
	}
}

func TestListAvailableBazaarItems_WithHelperExclusion(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	clock := &mockClock{now: now}

	repo := newMemoryRepo()
	charRepo := newMemoryCharacterRepo()
	invRepo := newMemoryInventoryRepo()
	helperMock := &mockHelperProvider{}

	svc, err := NewService(
		repo, charRepo, invRepo,
		WithClock(clock),
		WithHelperProvider(helperMock),
	)
	if err != nil {
		t.Fatalf("failed to init service: %v", err)
	}

	// 1. Tier 0 (< 10 members): No merchant -> empty list
	items, tier, err := svc.ListAvailableBazaarItems(ctx)
	if err != nil {
		t.Fatalf("ListAvailableBazaarItems failed: %v", err)
	}
	if tier != 0 || len(items) != 0 {
		t.Errorf("expected tier 0 and 0 items, got tier %d and %d items", tier, len(items))
	}

	// 2. 15 members -> Tier 1 (6 items)
	populatePresences(repo, 15, now)
	items, tier, err = svc.ListAvailableBazaarItems(ctx)
	if err != nil {
		t.Fatalf("ListAvailableBazaarItems failed: %v", err)
	}
	if tier != 1 || len(items) != 6 {
		t.Fatalf("expected tier 1 and 6 items, got tier %d and %d items", tier, len(items))
	}

	// 3. Mark item-081 (魔法の粉) as active in Helper Quest -> item-081 is excluded (5 items left)
	helperMock.activeIDs = []string{"item-081"}
	items, tier, err = svc.ListAvailableBazaarItems(ctx)
	if err != nil {
		t.Fatalf("ListAvailableBazaarItems failed: %v", err)
	}
	if len(items) != 5 {
		t.Errorf("expected 5 items after helper exclusion, got %d", len(items))
	}
	for _, it := range items {
		if it.ItemDefinitionID == "item-081" {
			t.Errorf("item-081 should have been excluded by active helper quest")
		}
	}

	// 4. 25 members -> Tier 2 (6 items from Tier 2 list)
	populatePresences(repo, 25, now)
	helperMock.activeIDs = nil
	items, tier, err = svc.ListAvailableBazaarItems(ctx)
	if err != nil {
		t.Fatalf("ListAvailableBazaarItems failed: %v", err)
	}
	if tier != 2 || len(items) != 6 {
		t.Errorf("expected tier 2 and 6 items, got tier %d and %d items", tier, len(items))
	}
	for _, it := range items {
		if it.TierRequired != 2 {
			t.Errorf("tier 2 merchant should only sell tier 2 items, got %+v", it)
		}
	}
}

func TestPurchaseBazaarItem_InventoryAndDepotRouting(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	clock := &mockClock{now: now}

	repo := newMemoryRepo()
	populatePresences(repo, 15, now) // Tier 1 unlocked

	charRepo := newMemoryCharacterRepo()
	invRepo := newMemoryInventoryRepo()
	depotRepo := newMemoryDepotRepo()
	helperMock := &mockHelperProvider{}
	recorderMock := &mockCollectionRecorder{}

	charID := id.New()
	char, _ := corecharacter.New("Hero")
	char.ID = charID
	char.PlayerID = "player-1"
	char.Money = 50000
	char.JobLevel = 5
	charRepo.characters[charID] = char

	svc, err := NewService(
		repo, charRepo, invRepo,
		WithClock(clock),
		WithDepotRepository(depotRepo),
		WithHelperProvider(helperMock),
		WithCollectionRecorder(recorderMock),
	)
	if err != nil {
		t.Fatalf("failed to init service: %v", err)
	}

	// 1. Purchase Tier 1 item with empty consumable slot & quantity 1 -> Delivered to Inventory
	// item-072 (身代わり人形, price 2400)
	res, err := svc.PurchaseBazaarItem(ctx, charID, "item-072", 1)
	if err != nil {
		t.Fatalf("purchase item-072 failed: %v", err)
	}
	if res.TotalPrice != 2400 {
		t.Errorf("expected total price 2400, got %d", res.TotalPrice)
	}
	if res.RemainingGold != 47600 {
		t.Errorf("expected remaining gold 47600, got %d", res.RemainingGold)
	}
	if res.TransferredToDepot {
		t.Errorf("expected transferred to depot = false")
	}
	if res.NPCMessage != "はい、身代わり人形です" {
		t.Errorf("unexpected NPC message: %s", res.NPCMessage)
	}

	inv, _ := invRepo.FindByCharacterID(ctx, charID)
	if len(inv.Items) != 1 || inv.Items[0].DefinitionID != "item-072" {
		t.Fatalf("expected item-072 in inventory, got %+v", inv.Items)
	}

	// Verify collection recorded
	if len(recorderMock.recorded) != 1 || recorderMock.recorded[0] != "item-072" {
		t.Errorf("expected item-072 recorded in collection, got %+v", recorderMock.recorded)
	}

	// 2. Second purchase while consumable slot is now occupied -> Delivered to Depot
	// item-081 (魔法の粉, price 3600)
	res, err = svc.PurchaseBazaarItem(ctx, charID, "item-081", 1)
	if err != nil {
		t.Fatalf("purchase item-081 failed: %v", err)
	}
	if res.TotalPrice != 3600 {
		t.Errorf("expected total price 3600, got %d", res.TotalPrice)
	}
	if res.RemainingGold != 44000 {
		t.Errorf("expected remaining gold 44000, got %d", res.RemainingGold)
	}
	if !res.TransferredToDepot {
		t.Errorf("expected transferred to depot = true when inventory slot occupied")
	}
	if res.NPCMessage != "魔法の粉はHeroさんの預かり所に送っておきましたよ" {
		t.Errorf("unexpected NPC message: %s", res.NPCMessage)
	}

	dep, err := depotRepo.FindByCharacterID(ctx, charID)
	if err != nil {
		t.Fatalf("failed to find depot: %v", err)
	}
	if len(dep.Items) != 1 || dep.Items[0].DefinitionID != "item-081" {
		t.Errorf("expected item-081 in depot, got %+v", dep.Items)
	}

	// 3. Purchase item active in helper quest -> ErrItemUnavailable
	helperMock.activeIDs = []string{"item-084"}
	_, err = svc.PurchaseBazaarItem(ctx, charID, "item-084", 1)
	if !errors.Is(err, ErrItemUnavailable) {
		t.Errorf("expected ErrItemUnavailable, got %v", err)
	}

	// 4. Purchase Tier 2 item while plaza is only at Tier 1 -> ErrItemTierLocked
	_, err = svc.PurchaseBazaarItem(ctx, charID, "item-005", 1)
	if !errors.Is(err, ErrItemTierLocked) {
		t.Errorf("expected ErrItemTierLocked, got %v", err)
	}

	// 5. Insufficient funds
	char.Money = 100
	_ = charRepo.Update(ctx, char)
	_, err = svc.PurchaseBazaarItem(ctx, charID, "item-086", 1)
	if !errors.Is(err, ErrInsufficientGold) {
		t.Errorf("expected ErrInsufficientGold, got %v", err)
	}

	// 6. Depot Full -> ErrDepotFull
	char.Money = 50000
	_ = charRepo.Update(ctx, char)
	// Fill depot to max capacity for char's job level
	fullDepot, _ := depot.NewDepotWithCapacity(charID, char.JobLevel, 0, char.OverDepot)
	fullDepot.RefreshCapacity(char.JobLevel, char.OverDepot)
	for i := 0; i < fullDepot.Capacity; i++ {
		dummyInst, _ := coreitem.NewInstance(fmt.Sprintf("item-dummy-%d", i), 1)
		_ = fullDepot.AddItem(dummyInst)
	}
	_ = depotRepo.Save(ctx, fullDepot)

	_, err = svc.PurchaseBazaarItem(ctx, charID, "item-082", 1)
	if !errors.Is(err, ErrDepotFull) {
		t.Errorf("expected ErrDepotFull, got %v", err)
	}
}

func TestVictoryBanquetAndToast(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	clock := &mockClock{now: now}

	repo := newMemoryRepo()
	charRepo := newMemoryCharacterRepo()
	invRepo := newMemoryInventoryRepo()

	slayerID := id.New()
	slayer, _ := corecharacter.New("SlayerHero")
	slayer.ID = slayerID
	slayer.PlayerID = "player-1"
	charRepo.characters[slayerID] = slayer

	toasterID := id.New()
	toaster, _ := corecharacter.New("TownVillager")
	toaster.ID = toasterID
	toaster.PlayerID = "player-2"
	toaster.Money = 500
	charRepo.characters[toasterID] = toaster

	svc, err := NewService(repo, charRepo, invRepo, WithClock(clock))
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	// 1. Record victory banquet
	banquet, err := svc.RecordVictoryBanquet(ctx, "boss-dragon-king", "Ancient Dragon King", slayerID, "SlayerHero", 3)
	if err != nil {
		t.Fatalf("failed to record victory banquet: %v", err)
	}
	if banquet.BossName != "Ancient Dragon King" || banquet.Tier != 3 {
		t.Errorf("unexpected banquet: %+v", banquet)
	}

	// 2. List active banquets
	banquets, err := svc.ListActiveBanquets(ctx)
	if err != nil || len(banquets) != 1 {
		t.Fatalf("expected 1 active banquet, got %d (err: %v)", len(banquets), err)
	}

	// 3. Toast banquet
	toastRes, err := svc.ToastBanquet(ctx, banquet.ID, toasterID)
	if err != nil {
		t.Fatalf("toast failed: %v", err)
	}
	// DefaultToastGoldReward (300) * Tier (3) = 900
	if toastRes.GoldAwarded != 900 {
		t.Errorf("expected 900 G reward, got %d", toastRes.GoldAwarded)
	}
	if toastRes.CurrentCharacterGold != 1400 {
		t.Errorf("expected 1400 G total, got %d", toastRes.CurrentCharacterGold)
	}

	// Verify toaster presence was recorded
	presenceCount, _ := repo.CountActiveParticipants(ctx, now.Add(-5*time.Minute))
	if presenceCount != 1 {
		t.Errorf("expected toaster presence recorded, got count %d", presenceCount)
	}

	// 4. Duplicate toast fails
	_, err = svc.ToastBanquet(ctx, banquet.ID, toasterID)
	if !errors.Is(err, ErrAlreadyToasted) {
		t.Errorf("expected ErrAlreadyToasted, got %v", err)
	}

	// 5. Expired banquet toast fails
	clock.now = clock.now.Add(25 * time.Hour) // Advance past expiration
	_, err = svc.ToastBanquet(ctx, banquet.ID, slayerID)
	if !errors.Is(err, ErrBanquetExpired) {
		t.Errorf("expected ErrBanquetExpired, got %v", err)
	}
}
