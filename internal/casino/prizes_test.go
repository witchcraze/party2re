package casino_test

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"
	"testing"

	"github.com/witchcraze/party2re/internal/casino"
	"github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/depot"
)

type mockDepotRepo struct {
	depots  map[string]depot.Depot
	saveErr error
}

func newMockDepotRepo() *mockDepotRepo {
	return &mockDepotRepo{depots: make(map[string]depot.Depot)}
}

func (m *mockDepotRepo) FindByCharacterIDForUpdate(ctx context.Context, characterID string) (depot.Depot, error) {
	d, ok := m.depots[characterID]
	if !ok {
		return depot.Depot{}, depot.ErrNotFound
	}
	return d, nil
}

func (m *mockDepotRepo) Save(ctx context.Context, dep depot.Depot) error {
	if m.saveErr != nil {
		return m.saveErr
	}
	seen := make(map[string]bool)
	for _, inst := range dep.Items {
		if seen[inst.ID] {
			return fmt.Errorf("duplicate depot_items id: %s", inst.ID)
		}
		seen[inst.ID] = true
	}
	m.depots[dep.CharacterID] = dep
	return nil
}

type mockPrizeCasinoRepo struct {
	mu       sync.RWMutex
	accounts map[string]casino.Account
}

func newMockPrizeCasinoRepo() *mockPrizeCasinoRepo {
	return &mockPrizeCasinoRepo{accounts: make(map[string]casino.Account)}
}

func (m *mockPrizeCasinoRepo) GetAccount(ctx context.Context, characterID string) (casino.Account, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	acc, ok := m.accounts[characterID]
	if !ok {
		return casino.Account{CharacterID: characterID, Coins: 0}, nil
	}
	return acc, nil
}

func (m *mockPrizeCasinoRepo) GetAccountForUpdate(ctx context.Context, characterID string) (casino.Account, error) {
	return m.GetAccount(ctx, characterID)
}

func (m *mockPrizeCasinoRepo) AdjustCoins(ctx context.Context, characterID string, coinDelta int64) (casino.Account, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	acc, ok := m.accounts[characterID]
	if !ok {
		acc = casino.Account{CharacterID: characterID, Coins: 0}
	}
	acc.Coins += coinDelta
	m.accounts[characterID] = acc
	return acc, nil
}

func (m *mockPrizeCasinoRepo) DeductBetAndCreditPayout(ctx context.Context, characterID string, bet int64, payout int64) (casino.Account, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	acc, ok := m.accounts[characterID]
	if !ok {
		acc = casino.Account{CharacterID: characterID, Coins: 0}
	}
	if acc.Coins < bet {
		return acc, casino.ErrInsufficientCoins
	}
	acc.Coins = acc.Coins - bet + payout
	m.accounts[characterID] = acc
	return acc, nil
}

func TestPrizeCatalog(t *testing.T) {
	prizes := casino.GetPrizes()
	if len(prizes) != 18 {
		t.Fatalf("expected 18 casino prizes matching legacy casino.cgi, got %d", len(prizes))
	}

	p1, err := casino.GetPrizeByCost(100)
	if err != nil || p1.ItemID != "item-004" {
		t.Errorf("expected 100 coin prize item-004, got %+v (err: %v)", p1, err)
	}

	pSwim, err := casino.GetPrizeByCost(8000)
	if err != nil || pSwim.ItemID != "armor-34" || pSwim.ItemName != "危ない水着" {
		t.Errorf("expected 8000 coin prize armor-34 危ない水着, got %+v (err: %v)", pSwim, err)
	}

	pPiercing, err := casino.GetPrizeByCost(30000)
	if err != nil || pPiercing.ItemID != "weapon-31" || pPiercing.ItemName != "必殺のピアス" {
		t.Errorf("expected 30000 coin prize weapon-31 必殺のピアス, got %+v (err: %v)", pPiercing, err)
	}

	_, err = casino.GetPrizeByCost(9999999)
	if !errors.Is(err, casino.ErrPrizeNotFound) {
		t.Errorf("expected ErrPrizeNotFound for non-existent cost, got %v", err)
	}
}

func TestExchangePrize_SuccessAndDepotRouting(t *testing.T) {
	repo := newMockPrizeCasinoRepo()
	depotMock := newMockDepotRepo()

	svc, err := casino.NewService(repo, casino.WithDepotRepository(depotMock))
	if err != nil {
		t.Fatalf("failed to create casino service: %v", err)
	}

	charID := "char-1"
	repo.accounts[charID] = casino.Account{CharacterID: charID, Coins: 10000}

	// 1. Exchange 1 unit of 8000 coin prize (危ない水着)
	res, err := svc.ExchangePrize(context.Background(), charID, 8000, 1)
	if err != nil {
		t.Fatalf("ExchangePrize error: %v", err)
	}

	if !res.TransferredToDepot {
		t.Errorf("expected TransferredToDepot to be true")
	}
	if res.RemainingCoins != 2000 {
		t.Errorf("expected remaining coins 2000, got %d", res.RemainingCoins)
	}
	savedDepot := depotMock.depots[charID]
	if len(savedDepot.Items) != 1 || savedDepot.Items[0].DefinitionID != "armor-34" {
		t.Errorf("expected 1 armor-34 in depot, got %+v", savedDepot.Items)
	}

	// 2. Exchange 2 units of 100 coin prize
	res2, err := svc.ExchangePrize(context.Background(), charID, 100, 2)
	if err != nil {
		t.Fatalf("ExchangePrize count=2 error: %v", err)
	}
	if res2.TotalCostCoins != 200 {
		t.Errorf("expected total cost 200, got %d", res2.TotalCostCoins)
	}
	if res2.RemainingCoins != 1800 {
		t.Errorf("expected remaining coins 1800, got %d", res2.RemainingCoins)
	}
	savedDepot = depotMock.depots[charID]
	if len(savedDepot.Items) != 2 || savedDepot.Quantity("item-004") != 2 {
		t.Errorf("expected 2 slots (armor and stacked item-004 x2) in depot, got %+v", savedDepot.Items)
	}
}

func TestExchangePrize_InsufficientCoins(t *testing.T) {
	repo := newMockPrizeCasinoRepo()
	depotMock := newMockDepotRepo()

	svc, err := casino.NewService(repo, casino.WithDepotRepository(depotMock))
	if err != nil {
		t.Fatalf("failed to create casino service: %v", err)
	}

	charID := "char-poor"
	repo.accounts[charID] = casino.Account{CharacterID: charID, Coins: 50}

	_, err = svc.ExchangePrize(context.Background(), charID, 100, 1)
	if !errors.Is(err, casino.ErrInsufficientCoins) {
		t.Errorf("expected ErrInsufficientCoins, got %v", err)
	}
	savedDepot, ok := depotMock.depots[charID]
	if ok && len(savedDepot.Items) != 0 {
		t.Errorf("expected zero items sent to depot on failure")
	}
}

func TestExchangePrize_DepotFull(t *testing.T) {
	repo := newMockPrizeCasinoRepo()
	depotMock := newMockDepotRepo()

	svc, err := casino.NewService(repo, casino.WithDepotRepository(depotMock))
	if err != nil {
		t.Fatalf("failed to create casino service: %v", err)
	}

	charID := "char-depot-full"
	repo.accounts[charID] = casino.Account{CharacterID: charID, Coins: 5000}

	// Pre-fill depot to capacity (5 items for lv 0)
	d, _ := depot.NewDepot(charID)
	for i := 0; i < 5; i++ {
		_ = d.AddItem(item.Instance{ID: "dummy", DefinitionID: "weapon-31", Quantity: 1})
	}
	depotMock.depots[charID] = d

	_, err = svc.ExchangePrize(context.Background(), charID, 2000, 1)
	if !errors.Is(err, depot.ErrDepotFull) {
		t.Errorf("expected depot.ErrDepotFull, got %v", err)
	}
	// Verify coins not deducted
	acc, _ := repo.GetAccount(context.Background(), charID)
	if acc.Coins != 5000 {
		t.Errorf("expected coins preserved at 5000, got %d", acc.Coins)
	}
}

func TestExchangePrize_BulkEquipmentDistinctIDs(t *testing.T) {
	repo := newMockPrizeCasinoRepo()
	depotMock := newMockDepotRepo()

	svc, err := casino.NewService(repo, casino.WithDepotRepository(depotMock))
	if err != nil {
		t.Fatalf("failed to create casino service: %v", err)
	}

	charID := "char-bulk"
	repo.accounts[charID] = casino.Account{CharacterID: charID, Coins: 100000}

	// 1. Exchange 2 units of 30,000 coin weapon (weapon-31: 必殺のピアス)
	res, err := svc.ExchangePrize(context.Background(), charID, 30000, 2)
	if err != nil {
		t.Fatalf("ExchangePrize bulk weapon error: %v", err)
	}
	if res.TotalCostCoins != 60000 {
		t.Errorf("expected total cost 60000, got %d", res.TotalCostCoins)
	}
	if res.RemainingCoins != 40000 {
		t.Errorf("expected remaining coins 40000, got %d", res.RemainingCoins)
	}

	savedDepot := depotMock.depots[charID]
	if len(savedDepot.Items) != 2 {
		t.Fatalf("expected 2 weapon instances in depot, got %d", len(savedDepot.Items))
	}
	if savedDepot.Items[0].ID == savedDepot.Items[1].ID {
		t.Errorf("expected distinct instance IDs for bulk weapons, got duplicate ID %s", savedDepot.Items[0].ID)
	}
	if savedDepot.Items[0].DefinitionID != "weapon-31" || savedDepot.Items[1].DefinitionID != "weapon-31" {
		t.Errorf("expected both items to be weapon-31, got %+v", savedDepot.Items)
	}
	if savedDepot.Items[0].Quantity != 1 || savedDepot.Items[1].Quantity != 1 {
		t.Errorf("expected quantity 1 for each equipment instance, got %+v", savedDepot.Items)
	}

	// 2. Exchange 2 units of 8,000 coin armor (armor-34: 危ない水着)
	resArmor, err := svc.ExchangePrize(context.Background(), charID, 8000, 2)
	if err != nil {
		t.Fatalf("ExchangePrize bulk armor error: %v", err)
	}
	if resArmor.TotalCostCoins != 16000 {
		t.Errorf("expected total cost 16000, got %d", resArmor.TotalCostCoins)
	}
	if resArmor.RemainingCoins != 24000 {
		t.Errorf("expected remaining coins 24000, got %d", resArmor.RemainingCoins)
	}

	savedDepot = depotMock.depots[charID]
	if len(savedDepot.Items) != 4 {
		t.Fatalf("expected 4 equipment instances in depot, got %d", len(savedDepot.Items))
	}
	ids := make(map[string]bool)
	for _, it := range savedDepot.Items {
		if ids[it.ID] {
			t.Errorf("duplicate ID %s found in depot items", it.ID)
		}
		ids[it.ID] = true
	}
}

func TestExchangePrize_DepotCapacityRollbackForBulkEquipment(t *testing.T) {
	repo := newMockPrizeCasinoRepo()
	depotMock := newMockDepotRepo()

	svc, err := casino.NewService(repo, casino.WithDepotRepository(depotMock))
	if err != nil {
		t.Fatalf("failed to create casino service: %v", err)
	}

	charID := "char-cap-rollback"
	repo.accounts[charID] = casino.Account{CharacterID: charID, Coins: 100000}

	// Depot capacity is 5. Pre-fill with 4 items so only 1 slot is left.
	d, _ := depot.NewDepot(charID)
	for i := 0; i < 4; i++ {
		_ = d.AddItem(item.Instance{ID: fmt.Sprintf("pre-%d", i), DefinitionID: "weapon-31", Quantity: 1})
	}
	depotMock.depots[charID] = d

	// Attempt to exchange 2 weapons (needs 2 slots, only 1 available)
	_, err = svc.ExchangePrize(context.Background(), charID, 30000, 2)
	if !errors.Is(err, depot.ErrDepotFull) {
		t.Errorf("expected depot.ErrDepotFull, got %v", err)
	}

	// Verify coins preserved at 100,000
	acc, _ := repo.GetAccount(context.Background(), charID)
	if acc.Coins != 100000 {
		t.Errorf("expected coins preserved at 100000, got %d", acc.Coins)
	}

	// Verify depot items preserved at 4
	savedDepot := depotMock.depots[charID]
	if len(savedDepot.Items) != 4 {
		t.Errorf("expected depot items preserved at 4, got %d", len(savedDepot.Items))
	}
}

func TestExchangePrize_OverflowRejected(t *testing.T) {
	repo := newMockPrizeCasinoRepo()
	depotMock := newMockDepotRepo()

	svc, err := casino.NewService(repo, casino.WithDepotRepository(depotMock))
	if err != nil {
		t.Fatalf("failed to create casino service: %v", err)
	}

	charID := "char-overflow"
	repo.accounts[charID] = casino.Account{CharacterID: charID, Coins: 100000}

	// Attempt to exchange with count = math.MaxInt (overflows SafeMultiply)
	_, err = svc.ExchangePrize(context.Background(), charID, 100, math.MaxInt)
	if !errors.Is(err, casino.ErrCoinOverflow) {
		t.Fatalf("expected ErrCoinOverflow, got %v", err)
	}

	// Verify coins preserved
	acc, _ := repo.GetAccount(context.Background(), charID)
	if acc.Coins != 100000 {
		t.Errorf("expected coins preserved at 100000, got %d", acc.Coins)
	}
}
