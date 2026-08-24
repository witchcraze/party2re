package casino_test

import (
	"context"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/casino"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

type mockCasinoRepo struct {
	getAccountFn func(ctx context.Context, charID string) (casino.Account, error)
	buyCoinsFn   func(ctx context.Context, charID string, coins int64, goldCost int) (casino.Account, corecharacter.Character, error)
	sellCoinsFn  func(ctx context.Context, charID string, coins int64, goldReward int) (casino.Account, corecharacter.Character, error)
	adjustFn     func(ctx context.Context, charID string, delta int64) (casino.Account, error)
}

func (m *mockCasinoRepo) GetAccount(ctx context.Context, charID string) (casino.Account, error) {
	if m.getAccountFn != nil {
		return m.getAccountFn(ctx, charID)
	}
	return casino.Account{CharacterID: charID, Coins: 1000, UpdatedAt: time.Now().UTC()}, nil
}

func (m *mockCasinoRepo) ExchangeGoldToCoins(ctx context.Context, charID string, coins int64, goldCost int) (casino.Account, corecharacter.Character, error) {
	if m.buyCoinsFn != nil {
		return m.buyCoinsFn(ctx, charID, coins, goldCost)
	}
	return casino.Account{CharacterID: charID, Coins: coins}, corecharacter.Character{ID: charID, Money: 10000 - goldCost}, nil
}

func (m *mockCasinoRepo) ExchangeCoinsToGold(ctx context.Context, charID string, coins int64, goldReward int) (casino.Account, corecharacter.Character, error) {
	if m.sellCoinsFn != nil {
		return m.sellCoinsFn(ctx, charID, coins, goldReward)
	}
	return casino.Account{CharacterID: charID, Coins: 1000 - coins}, corecharacter.Character{ID: charID, Money: goldReward}, nil
}

func (m *mockCasinoRepo) AdjustCoins(ctx context.Context, charID string, delta int64) (casino.Account, error) {
	if m.adjustFn != nil {
		return m.adjustFn(ctx, charID, delta)
	}
	return casino.Account{CharacterID: charID, Coins: 1000 + delta}, nil
}

func TestCasinoService_Exchanges(t *testing.T) {
	ctx := context.Background()
	repo := &mockCasinoRepo{}
	svc, err := casino.NewService(repo)
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
		acc, char, err := svc.ExchangeCoinsToGold(ctx, "char1", 50)
		if err != nil {
			t.Fatalf("ExchangeCoinsToGold error: %v", err)
		}
		if acc.Coins != 950 || char.Money != 1000 {
			t.Errorf("acc.Coins = %d, char.Money = %d", acc.Coins, char.Money)
		}
	})

	t.Run("Invalid amount returns error", func(t *testing.T) {
		_, _, err := svc.ExchangeGoldToCoins(ctx, "char1", 0)
		if err != casino.ErrInvalidAmount {
			t.Errorf("got %v, want ErrInvalidAmount", err)
		}
	})
}

func TestCasinoService_IndianPokerLifecycle(t *testing.T) {
	ctx := context.Background()
	var currentCoins int64 = 500

	repo := &mockCasinoRepo{
		getAccountFn: func(_ context.Context, charID string) (casino.Account, error) {
			return casino.Account{CharacterID: charID, Coins: currentCoins}, nil
		},
		adjustFn: func(_ context.Context, charID string, delta int64) (casino.Account, error) {
			currentCoins += delta
			return casino.Account{CharacterID: charID, Coins: currentCoins}, nil
		},
	}
	svc, _ := casino.NewService(repo)

	// 1. Start game with rate 10 -> Ante 10 deducted
	game, acc, err := svc.StartIndianPokerGame(ctx, "char1", 10)
	if err != nil {
		t.Fatalf("StartIndianPokerGame error: %v", err)
	}
	if acc.Coins != 490 || game.Pot != 20 {
		t.Errorf("after start: coins=%d, pot=%d", acc.Coins, game.Pot)
	}

	// 2. Play fold
	acc, err = svc.PlayIndianPokerRound(ctx, "char1", game, casino.ActionFold)
	if err != nil {
		t.Fatalf("PlayIndianPokerRound fold error: %v", err)
	}
	if acc.Coins != 490 || game.Status != casino.StatusPlayerFolded {
		t.Errorf("after fold: coins=%d, status=%v", acc.Coins, game.Status)
	}
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
