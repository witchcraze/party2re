package casino_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/casino"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	"github.com/witchcraze/party2re/internal/economy"
)

type mockCasinoRepo struct {
	getAccountFn      func(ctx context.Context, charID string) (casino.Account, error)
	adjustFn          func(ctx context.Context, charID string, delta int64) (casino.Account, error)
	deductAndCreditFn func(ctx context.Context, charID string, bet int64, payout int64) (casino.Account, error)
}

func (m *mockCasinoRepo) GetAccount(ctx context.Context, charID string) (casino.Account, error) {
	if m.getAccountFn != nil {
		return m.getAccountFn(ctx, charID)
	}
	return casino.Account{CharacterID: charID, Coins: 1000, UpdatedAt: time.Now().UTC()}, nil
}

func (m *mockCasinoRepo) GetAccountForUpdate(ctx context.Context, charID string) (casino.Account, error) {
	return m.GetAccount(ctx, charID)
}

func (m *mockCasinoRepo) AdjustCoins(ctx context.Context, charID string, delta int64) (casino.Account, error) {
	if m.adjustFn != nil {
		return m.adjustFn(ctx, charID, delta)
	}
	return casino.Account{CharacterID: charID, Coins: 1000 + delta}, nil
}

func (m *mockCasinoRepo) DeductBetAndCreditPayout(ctx context.Context, charID string, bet int64, payout int64) (casino.Account, error) {
	if m.deductAndCreditFn != nil {
		return m.deductAndCreditFn(ctx, charID, bet, payout)
	}
	return casino.Account{CharacterID: charID, Coins: 1000 - bet + payout}, nil
}

func TestCasinoService_Exchanges(t *testing.T) {
	ctx := context.Background()
	var currentCoins int64 = 0
	repo := &mockCasinoRepo{
		adjustFn: func(_ context.Context, charID string, delta int64) (casino.Account, error) {
			currentCoins += delta
			return casino.Account{CharacterID: charID, Coins: currentCoins}, nil
		},
		deductAndCreditFn: func(_ context.Context, charID string, bet int64, payout int64) (casino.Account, error) {
			currentCoins = currentCoins - bet + payout
			return casino.Account{CharacterID: charID, Coins: currentCoins}, nil
		},
	}
	runner := &stubTransactionRunner{money: 10000}
	svc, err := casino.NewService(repo, casino.WithTransactionRunner(runner))
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}

	t.Run("Buy Coins: 100 coins -> 2000 gold cost", func(t *testing.T) {
		acc, char, err := svc.ExchangeGoldToCoins(ctx, "char1", 100)
		if err != nil {
			t.Fatalf("ExchangeGoldToCoins error: %v", err)
		}
		if acc.Coins != 100 || char.Money != 8000 {
			t.Errorf("acc.Coins = %d, char.Money = %d", acc.Coins, char.Money)
		}
	})

	t.Run("Sell Coins: 50 coins -> 1000 gold reward", func(t *testing.T) {
		runner.money = 8000
		acc, char, err := svc.ExchangeCoinsToGold(ctx, "char1", 50)
		if err != nil {
			t.Fatalf("ExchangeCoinsToGold error: %v", err)
		}
		if acc.Coins != 50 || char.Money != 9000 {
			t.Errorf("acc.Coins = %d, char.Money = %d", acc.Coins, char.Money)
		}
	})

	t.Run("Invalid amount returns error", func(t *testing.T) {
		_, _, err := svc.ExchangeGoldToCoins(ctx, "char1", 0)
		if err != casino.ErrInvalidAmount {
			t.Errorf("got %v, want ErrInvalidAmount", err)
		}
	})

	t.Run("Missing runner returns error", func(t *testing.T) {
		svcNoRunner, _ := casino.NewService(repo)
		_, _, err := svcNoRunner.ExchangeGoldToCoins(ctx, "char1", 10)
		if err == nil {
			t.Errorf("expected error when transaction runner is missing")
		}
	})
}

func TestCasinoService_SpinSlot(t *testing.T) {
	ctx := context.Background()
	var currentCoins int64 = 100

	repo := &mockCasinoRepo{
		getAccountFn: func(_ context.Context, charID string) (casino.Account, error) {
			return casino.Account{CharacterID: charID, Coins: currentCoins}, nil
		},
		adjustFn: func(_ context.Context, charID string, delta int64) (casino.Account, error) {
			currentCoins += delta
			return casino.Account{CharacterID: charID, Coins: currentCoins}, nil
		},
		deductAndCreditFn: func(_ context.Context, charID string, bet int64, payout int64) (casino.Account, error) {
			if currentCoins < bet {
				return casino.Account{CharacterID: charID, Coins: currentCoins}, casino.ErrInsufficientCoins
			}
			currentCoins = currentCoins - bet + payout
			return casino.Account{CharacterID: charID, Coins: currentCoins}, nil
		},
	}
	svc, _ := casino.NewService(repo)

	// Valid spin with 10 coins
	res, acc, err := svc.SpinSlot(ctx, "char1", 10)
	if err != nil {
		t.Fatalf("SpinSlot failed: %v", err)
	}
	if res.BetCoins != 10 {
		t.Errorf("bet coins = %d, want 10", res.BetCoins)
	}
	if acc.Coins != 100+res.NetCoins {
		t.Errorf("account coins = %d, want %d", acc.Coins, 100+res.NetCoins)
	}

	// Invalid rate
	if _, _, err := svc.SpinSlot(ctx, "char1", 25); err != casino.ErrInvalidBetRate {
		t.Errorf("err = %v, want ErrInvalidBetRate", err)
	}

	// Insufficient coins
	currentCoins = 5
	if _, _, err := svc.SpinSlot(ctx, "char1", 50); err != casino.ErrInsufficientCoins {
		t.Errorf("err = %v, want ErrInsufficientCoins", err)
	}
}

func TestCasinoService_GamePlayedHook(t *testing.T) {
	ctx := context.Background()
	var currentCoins int64 = 500

	repo := &mockCasinoRepo{
		getAccountFn: func(_ context.Context, charID string) (casino.Account, error) {
			return casino.Account{CharacterID: charID, Coins: currentCoins}, nil
		},
		deductAndCreditFn: func(_ context.Context, charID string, bet int64, payout int64) (casino.Account, error) {
			currentCoins = currentCoins - bet + payout
			return casino.Account{CharacterID: charID, Coins: currentCoins}, nil
		},
	}
	svc, err := casino.NewService(repo)
	if err != nil {
		t.Fatal(err)
	}

	var playedGames []string
	svc.SetGamePlayedHook(func(ctx context.Context, characterID string, gameName string) error {
		playedGames = append(playedGames, gameName)
		return nil
	})

	// 1. Slot
	_, _, err = svc.SpinSlot(ctx, "char1", 10)
	if err != nil {
		t.Fatalf("SpinSlot failed: %v", err)
	}

	if len(playedGames) != 1 || playedGames[0] != "slot" {
		t.Fatalf("expected 1 game recorded as 'slot', got %v", playedGames)
	}
}

type stubTransactionRunner struct {
	called bool
	req    economy.TransactionRequest
	money  int
}

func (s *stubTransactionRunner) ExecuteTransaction(ctx context.Context, req economy.TransactionRequest, fn economy.TransactionCallback) (*economy.TransactionResult, error) {
	s.called = true
	s.req = req

	char, _ := corecharacter.New("Casino Gambler")
	char.ID = req.CharacterID
	char.Money = s.money

	if req.Cost.Gold > 0 {
		if char.Money < req.Cost.Gold {
			return nil, economy.ErrInsufficientGold
		}
		_ = char.DeductMoney(req.Cost.Gold)
	}

	tc := &economy.TxContext{
		Context:   ctx,
		Character: char,
	}

	if fn != nil {
		if err := fn(tc); err != nil {
			return nil, err
		}
	}

	if req.Grant.Gold > 0 {
		_ = tc.Character.AddMoney(req.Grant.Gold)
	}

	return &economy.TransactionResult{
		Character: tc.Character,
	}, nil
}

func TestCasinoService_Exchanges_WithTransactionRunner(t *testing.T) {
	ctx := context.Background()
	var currentCoins int64 = 100

	repo := &mockCasinoRepo{
		adjustFn: func(_ context.Context, charID string, delta int64) (casino.Account, error) {
			currentCoins += delta
			return casino.Account{CharacterID: charID, Coins: currentCoins}, nil
		},
		deductAndCreditFn: func(_ context.Context, charID string, bet int64, payout int64) (casino.Account, error) {
			if currentCoins < bet {
				return casino.Account{CharacterID: charID, Coins: currentCoins}, casino.ErrInsufficientCoins
			}
			currentCoins = currentCoins - bet + payout
			return casino.Account{CharacterID: charID, Coins: currentCoins}, nil
		},
	}

	runner := &stubTransactionRunner{money: 5000}
	svc, err := casino.NewService(repo, casino.WithTransactionRunner(runner))
	if err != nil {
		t.Fatal(err)
	}

	t.Run("Buy coins via TransactionRunner: 50 coins -> 1000 gold", func(t *testing.T) {
		acc, char, err := svc.ExchangeGoldToCoins(ctx, "char1", 50)
		if err != nil {
			t.Fatalf("ExchangeGoldToCoins error: %v", err)
		}
		if !runner.called {
			t.Error("expected TransactionRunner to be called")
		}
		if runner.req.Cost.Gold != 1000 {
			t.Errorf("req.Cost.Gold = %d, want 1000", runner.req.Cost.Gold)
		}
		if acc.Coins != 150 {
			t.Errorf("acc.Coins = %d, want 150", acc.Coins)
		}
		if char.Money != 4000 {
			t.Errorf("char.Money = %d, want 4000", char.Money)
		}
	})

	t.Run("Sell coins via TransactionRunner: 50 coins -> 1000 gold grant", func(t *testing.T) {
		runner.called = false
		runner.money = 4000
		acc, char, err := svc.ExchangeCoinsToGold(ctx, "char1", 50)
		if err != nil {
			t.Fatalf("ExchangeCoinsToGold error: %v", err)
		}
		if !runner.called {
			t.Error("expected TransactionRunner to be called")
		}
		if runner.req.Grant.Gold != 1000 {
			t.Errorf("req.Grant.Gold = %d, want 1000", runner.req.Grant.Gold)
		}
		if acc.Coins != 100 {
			t.Errorf("acc.Coins = %d, want 100", acc.Coins)
		}
		if char.Money != 5000 {
			t.Errorf("char.Money = %d, want 5000", char.Money)
		}
	})

	t.Run("Insufficient gold returns ErrInsufficientGold", func(t *testing.T) {
		runner.money = 100 // not enough for 100 coins (2000 gold)
		_, _, err := svc.ExchangeGoldToCoins(ctx, "char1", 100)
		if !errors.Is(err, casino.ErrInsufficientGold) {
			t.Errorf("expected ErrInsufficientGold, got %v", err)
		}
	})

	t.Run("Insufficient coins returns ErrInsufficientCoins", func(t *testing.T) {
		currentCoins = 10
		_, _, err := svc.ExchangeCoinsToGold(ctx, "char1", 50)
		if !errors.Is(err, casino.ErrInsufficientCoins) {
			t.Errorf("expected ErrInsufficientCoins, got %v", err)
		}
	})
}

type inMemoryCharRepo struct {
	chars map[string]corecharacter.Character
}

func (r *inMemoryCharRepo) FindByID(_ context.Context, id string) (corecharacter.Character, error) {
	c, ok := r.chars[id]
	if !ok {
		return corecharacter.Character{}, corecharacter.ErrNotFound
	}
	return c, nil
}

func (r *inMemoryCharRepo) FindByIDForUpdate(ctx context.Context, id string) (corecharacter.Character, error) {
	return r.FindByID(ctx, id)
}

func (r *inMemoryCharRepo) Update(_ context.Context, character corecharacter.Character) error {
	r.chars[character.ID] = character
	return nil
}

type inMemoryInvRepo struct{}

func (r *inMemoryInvRepo) FindByCharacterID(_ context.Context, charID string) (coreinventory.Inventory, error) {
	return coreinventory.New(charID)
}

func (r *inMemoryInvRepo) FindByCharacterIDForUpdate(ctx context.Context, charID string) (coreinventory.Inventory, error) {
	return r.FindByCharacterID(ctx, charID)
}

func (r *inMemoryInvRepo) Save(_ context.Context, _ coreinventory.Inventory) error {
	return nil
}

func TestCasinoService_Exchanges_WithEconomyService(t *testing.T) {
	ctx := context.Background()
	charRepo := &inMemoryCharRepo{
		chars: make(map[string]corecharacter.Character),
	}
	char, _ := corecharacter.New("Economy Gambler")
	char.ID = "char-eco-1"
	char.Money = 5000
	charRepo.chars[char.ID] = char

	invRepo := &inMemoryInvRepo{}
	eco, err := economy.NewService(charRepo, invRepo)
	if err != nil {
		t.Fatal(err)
	}

	var currentCoins int64 = 50
	repo := &mockCasinoRepo{
		adjustFn: func(_ context.Context, charID string, delta int64) (casino.Account, error) {
			currentCoins += delta
			return casino.Account{CharacterID: charID, Coins: currentCoins}, nil
		},
		deductAndCreditFn: func(_ context.Context, charID string, bet int64, payout int64) (casino.Account, error) {
			if currentCoins < bet {
				return casino.Account{CharacterID: charID, Coins: currentCoins}, casino.ErrInsufficientCoins
			}
			currentCoins = currentCoins - bet + payout
			return casino.Account{CharacterID: charID, Coins: currentCoins}, nil
		},
	}

	svc, err := casino.NewService(repo, casino.WithEconomy(eco))
	if err != nil {
		t.Fatal(err)
	}

	// 1. Buy 50 coins for 1000 gold
	acc, updatedChar, err := svc.ExchangeGoldToCoins(ctx, char.ID, 50)
	if err != nil {
		t.Fatalf("ExchangeGoldToCoins error: %v", err)
	}
	if acc.Coins != 100 {
		t.Errorf("acc.Coins = %d, want 100", acc.Coins)
	}
	if updatedChar.Money != 4000 {
		t.Errorf("updatedChar.Money = %d, want 4000", updatedChar.Money)
	}

	// 2. Sell 30 coins for 600 gold
	acc, updatedChar, err = svc.ExchangeCoinsToGold(ctx, char.ID, 30)
	if err != nil {
		t.Fatalf("ExchangeCoinsToGold error: %v", err)
	}
	if acc.Coins != 70 {
		t.Errorf("acc.Coins = %d, want 70", acc.Coins)
	}
	if updatedChar.Money != 4600 {
		t.Errorf("updatedChar.Money = %d, want 4600", updatedChar.Money)
	}
}
