package casino_test

import (
	"context"
	"errors"
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
	m.depots[dep.CharacterID] = dep
	return nil
}

type mockPrizeCasinoRepo struct {
	accounts map[string]casino.Account
}

func newMockPrizeCasinoRepo() *mockPrizeCasinoRepo {
	return &mockPrizeCasinoRepo{accounts: make(map[string]casino.Account)}
}

func (m *mockPrizeCasinoRepo) GetAccount(ctx context.Context, characterID string) (casino.Account, error) {
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
	acc, _ := m.GetAccount(ctx, characterID)
	acc.Coins += coinDelta
	m.accounts[characterID] = acc
	return acc, nil
}

func (m *mockPrizeCasinoRepo) DeductBetAndCreditPayout(ctx context.Context, characterID string, bet int64, payout int64) (casino.Account, error) {
	acc, _ := m.GetAccount(ctx, characterID)
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
