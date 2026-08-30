package eventplaza

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	"github.com/witchcraze/party2re/internal/id"
)

type mockClock struct {
	now time.Time
}

func (m *mockClock) Now() time.Time {
	return m.now
}

type memoryRepo struct {
	mu           sync.Mutex
	participants int
	banquets     map[string]CelebrationBanquet
	toasts       map[string]map[string]time.Time // banquetID -> characterID -> time
}

func newMemoryRepo(participants int) *memoryRepo {
	return &memoryRepo{
		participants: participants,
		banquets:     make(map[string]CelebrationBanquet),
		toasts:       make(map[string]map[string]time.Time),
	}
}

func (m *memoryRepo) CountActiveParticipants(ctx context.Context) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.participants, nil
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

func TestCatalogLoading(t *testing.T) {
	catalog, err := LoadDefaultBazaarCatalog()
	if err != nil {
		t.Fatalf("unexpected error loading catalog: %v", err)
	}
	if len(catalog) == 0 {
		t.Fatal("expected non-empty bazaar catalog")
	}

	tier1Count := 0
	tier2Count := 0
	tier3Count := 0
	for _, item := range catalog {
		if item.ID == "" || item.Name == "" || item.Price <= 0 {
			t.Errorf("invalid item: %+v", item)
		}
		switch item.TierRequired {
		case 1:
			tier1Count++
		case 2:
			tier2Count++
		case 3:
			tier3Count++
		}
	}

	if tier1Count == 0 || tier2Count == 0 || tier3Count == 0 {
		t.Errorf("expected items in all tiers, got tier1=%d tier2=%d tier3=%d", tier1Count, tier2Count, tier3Count)
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

func TestPlazaStatusAndBazaarListing(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)
	clock := &mockClock{now: now}

	repo := newMemoryRepo(15) // Tier 1
	charRepo := newMemoryCharacterRepo()
	invRepo := newMemoryInventoryRepo()

	svc, err := NewService(repo, charRepo, invRepo, WithClock(clock))
	if err != nil {
		t.Fatalf("unexpected error initializing service: %v", err)
	}

	status, err := svc.GetPlazaStatus(ctx)
	if err != nil {
		t.Fatalf("unexpected error getting status: %v", err)
	}

	if status.ActiveParticipants != 15 {
		t.Errorf("expected 15 participants, got %d", status.ActiveParticipants)
	}
	if status.MerchantTier != 1 {
		t.Errorf("expected merchant tier 1, got %d", status.MerchantTier)
	}
	if status.NextTierThreshold != 20 {
		t.Errorf("expected next threshold 20, got %d", status.NextTierThreshold)
	}

	items, tier, err := svc.ListAvailableBazaarItems(ctx)
	if err != nil {
		t.Fatalf("unexpected error listing items: %v", err)
	}
	if tier != 1 {
		t.Errorf("expected tier 1, got %d", tier)
	}
	for _, item := range items {
		if item.TierRequired > 1 {
			t.Errorf("tier 1 bazaar returned tier %d item: %s", item.TierRequired, item.ID)
		}
	}
}

func TestPurchaseBazaarItem(t *testing.T) {
	ctx := context.Background()
	repo := newMemoryRepo(25) // Tier 2 unlocked
	charRepo := newMemoryCharacterRepo()
	invRepo := newMemoryInventoryRepo()

	charID := id.New()
	char, err := corecharacter.New("Hero")
	if err != nil {
		t.Fatalf("failed to create character: %v", err)
	}
	char.ID = charID
	char.PlayerID = "player-1"
	char.Money = 20000
	charRepo.characters[charID] = char

	svc, err := NewService(repo, charRepo, invRepo)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	// 1. Successful purchase of Tier 1 item
	res, err := svc.PurchaseBazaarItem(ctx, charID, "bazaar_herb_extract", 2)
	if err != nil {
		t.Fatalf("purchase failed: %v", err)
	}
	if res.TotalPrice != 1000 {
		t.Errorf("expected total price 1000, got %d", res.TotalPrice)
	}
	if res.RemainingGold != 19000 {
		t.Errorf("expected remaining gold 19000, got %d", res.RemainingGold)
	}

	// Verify inventory
	inv, _ := invRepo.FindByCharacterID(ctx, charID)
	if len(inv.Items) != 1 || inv.Items[0].Quantity != 2 {
		t.Errorf("expected 1 item with quantity 2 in inventory, got %+v", inv.Items)
	}

	// 2. Purchase Tier 3 item when only Tier 2 is unlocked -> ErrItemTierLocked
	_, err = svc.PurchaseBazaarItem(ctx, charID, "bazaar_starlight_blade", 1)
	if !errors.Is(err, ErrItemTierLocked) {
		t.Errorf("expected ErrItemTierLocked, got %v", err)
	}

	// 3. Insufficient funds
	_, err = svc.PurchaseBazaarItem(ctx, charID, "bazaar_wind_cloak", 2) // 15,000 * 2 = 30,000 > 19,000
	if !errors.Is(err, ErrInsufficientGold) {
		t.Errorf("expected ErrInsufficientGold, got %v", err)
	}

	// 4. Invalid quantity
	_, err = svc.PurchaseBazaarItem(ctx, charID, "bazaar_herb_extract", 0)
	if !errors.Is(err, ErrInvalidQuantity) {
		t.Errorf("expected ErrInvalidQuantity, got %v", err)
	}

	// 5. Unknown item
	_, err = svc.PurchaseBazaarItem(ctx, charID, "nonexistent_item", 1)
	if !errors.Is(err, ErrItemNotFound) {
		t.Errorf("expected ErrItemNotFound, got %v", err)
	}
}

func TestVictoryBanquetAndToast(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)
	clock := &mockClock{now: now}

	repo := newMemoryRepo(10)
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
