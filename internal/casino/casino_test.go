package casino_test

import (
	"context"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/casino"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

type mockCasinoRepo struct {
	getAccountFn      func(ctx context.Context, charID string) (casino.Account, error)
	buyCoinsFn        func(ctx context.Context, charID string, coins int64, goldCost int) (casino.Account, corecharacter.Character, error)
	sellCoinsFn       func(ctx context.Context, charID string, coins int64, goldReward int) (casino.Account, corecharacter.Character, error)
	adjustFn          func(ctx context.Context, charID string, delta int64) (casino.Account, error)
	deductAndCreditFn func(ctx context.Context, charID string, bet int64, payout int64) (casino.Account, error)
	pokerGames        map[string]*casino.IndianPokerGame
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

func (m *mockCasinoRepo) DeductBetAndCreditPayout(ctx context.Context, charID string, bet int64, payout int64) (casino.Account, error) {
	if m.deductAndCreditFn != nil {
		return m.deductAndCreditFn(ctx, charID, bet, payout)
	}
	return casino.Account{CharacterID: charID, Coins: 1000 - bet + payout}, nil
}

func (m *mockCasinoRepo) SavePokerGame(ctx context.Context, game casino.IndianPokerGame) error {
	if m.pokerGames == nil {
		m.pokerGames = make(map[string]*casino.IndianPokerGame)
	}
	cpy := game
	m.pokerGames[game.CharacterID] = &cpy
	return nil
}

func (m *mockCasinoRepo) GetActivePokerGame(ctx context.Context, charID string) (*casino.IndianPokerGame, error) {
	if m.pokerGames == nil {
		return nil, nil
	}
	g, ok := m.pokerGames[charID]
	if !ok || g.Status != casino.StatusInProgress {
		return nil, nil
	}
	cpy := *g
	return &cpy, nil
}

func (m *mockCasinoRepo) GetActivePokerGameForUpdate(ctx context.Context, charID string) (*casino.IndianPokerGame, error) {
	return m.GetActivePokerGame(ctx, charID)
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
		deductAndCreditFn: func(_ context.Context, charID string, bet int64, payout int64) (casino.Account, error) {
			if currentCoins < bet {
				return casino.Account{CharacterID: charID, Coins: currentCoins}, casino.ErrInsufficientCoins
			}
			currentCoins = currentCoins - bet + payout
			return casino.Account{CharacterID: charID, Coins: currentCoins}, nil
		},
	}
	svc, _ := casino.NewService(repo)

	// 1. Start game with rate 10 -> Ante 10 deducted (490 coins remaining)
	game, acc, err := svc.StartIndianPokerGame(ctx, "char1", 10)
	if err != nil {
		t.Fatalf("StartIndianPokerGame error: %v", err)
	}
	if acc.Coins != 490 || game.Pot != 20 {
		t.Errorf("after start: coins=%d, pot=%d", acc.Coins, game.Pot)
	}
	// Player card must be masked in client view while in progress
	if game.PlayerCard.Rank != 0 || game.PlayerCard.Suit != "?" {
		t.Errorf("expected masked player card, got %+v", game.PlayerCard)
	}

	// Set mid-rank cards in repo to guarantee dealer calls in Round 1
	repo.pokerGames["char1"].PlayerCard = casino.Card{Suit: casino.SuitSpades, Rank: casino.RankSeven}
	repo.pokerGames["char1"].DealerCard = casino.Card{Suit: casino.SuitHearts, Rank: casino.RankSeven}

	// 2. Starting another game while in progress should fail with ErrActiveSessionExists
	_, _, err = svc.StartIndianPokerGame(ctx, "char1", 10)
	if err != casino.ErrActiveSessionExists {
		t.Errorf("expected ErrActiveSessionExists, got %v", err)
	}

	// 3. Query active game
	activeGame, activeAcc, err := svc.GetActiveIndianPokerGame(ctx, "char1")
	if err != nil {
		t.Fatalf("GetActiveIndianPokerGame error: %v", err)
	}
	if activeGame.ID != game.ID || activeAcc.Coins != 490 {
		t.Errorf("unexpected active game: %+v, acc=%+v", activeGame, activeAcc)
	}
	if activeGame.PlayerCard.Rank != 0 {
		t.Errorf("active game player card must remain masked")
	}

	// 4. Play action 'call' -> round advances to 2, bet of 10 deducted (480 coins remaining)
	nextGame, acc, err := svc.PlayIndianPokerAction(ctx, "char1", casino.ActionCall)
	if err != nil {
		t.Fatalf("PlayIndianPokerAction call error: %v", err)
	}
	if nextGame.Status == casino.StatusInProgress {
		if nextGame.Round != 2 {
			t.Errorf("round = %d, want 2", nextGame.Round)
		}
		if acc.Coins != 480 {
			t.Errorf("coins = %d, want 480", acc.Coins)
		}
	}

	// 5. Play action 'fold' -> game terminates with StatusPlayerFolded
	finalGame, acc, err := svc.PlayIndianPokerAction(ctx, "char1", casino.ActionFold)
	if err != nil {
		t.Fatalf("PlayIndianPokerAction fold error: %v", err)
	}
	if finalGame.Status != casino.StatusPlayerFolded {
		t.Errorf("status = %v, want %v", finalGame.Status, casino.StatusPlayerFolded)
	}
	// After game completes, player card is revealed
	if finalGame.PlayerCard.Rank == 0 {
		t.Errorf("expected revealed player card after game completion")
	}

	// 6. Querying active game now returns ErrNoActivePokerGame
	_, _, err = svc.GetActiveIndianPokerGame(ctx, "char1")
	if err != casino.ErrNoActivePokerGame {
		t.Errorf("expected ErrNoActivePokerGame, got %v", err)
	}

	// 7. Starting a new game after completion succeeds
	newGame, newAcc, err := svc.StartIndianPokerGame(ctx, "char1", 20)
	if err != nil {
		t.Fatalf("StartIndianPokerGame after completion error: %v", err)
	}
	if newGame.BaseRate != 20 || newAcc.Coins != acc.Coins-20 {
		t.Errorf("new game unexpected state: base_rate=%d, coins=%d", newGame.BaseRate, newAcc.Coins)
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

func TestCasinoService_PlayDoppel(t *testing.T) {
	ctx := context.Background()
	var currentCoins int64 = 200

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

	// Valid Doppel game with 50 coins and pool size 4
	res, acc, err := svc.PlayDoppel(ctx, "char1", 50, 4, casino.MarkStar)
	if err != nil {
		t.Fatalf("PlayDoppel failed: %v", err)
	}
	if res.BetCoins != 50 || res.PoolSize != 4 {
		t.Errorf("res = %+v", res)
	}
	if acc.Coins != 200+res.NetCoins {
		t.Errorf("account coins = %d, want %d", acc.Coins, 200+res.NetCoins)
	}

	// Insufficient coins
	currentCoins = 10
	if _, _, err := svc.PlayDoppel(ctx, "char1", 50, 4, casino.MarkStar); err != casino.ErrInsufficientCoins {
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

	// 2. Doppel
	_, _, err = svc.PlayDoppel(ctx, "char1", 50, 4, casino.MarkStar)
	if err != nil {
		t.Fatalf("PlayDoppel failed: %v", err)
	}

	// 3. HighLow
	_, _, err = svc.PlayHighLow(ctx, "char1", 10, casino.GuessHigh)
	if err != nil {
		t.Fatalf("PlayHighLow failed: %v", err)
	}

	if len(playedGames) != 3 {
		t.Fatalf("expected 3 games recorded, got %d: %v", len(playedGames), playedGames)
	}
	if playedGames[0] != "slot" || playedGames[1] != "doppel" || playedGames[2] != "highlow" {
		t.Errorf("unexpected playedGames sequence: %v", playedGames)
	}
}
