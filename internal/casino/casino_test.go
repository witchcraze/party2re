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
	getAccountFn         func(ctx context.Context, charID string) (casino.Account, error)
	buyCoinsFn           func(ctx context.Context, charID string, coins int64, goldCost int) (casino.Account, corecharacter.Character, error)
	sellCoinsFn          func(ctx context.Context, charID string, coins int64, goldReward int) (casino.Account, corecharacter.Character, error)
	adjustFn             func(ctx context.Context, charID string, delta int64) (casino.Account, error)
	deductAndCreditFn    func(ctx context.Context, charID string, bet int64, payout int64) (casino.Account, error)
	savePokerGameFn      func(ctx context.Context, game casino.IndianPokerGame) error
	getActivePokerGameFn func(ctx context.Context, charID string) (*casino.IndianPokerGame, error)
	pokerGames           map[string]*casino.IndianPokerGame
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
	if m.savePokerGameFn != nil {
		return m.savePokerGameFn(ctx, game)
	}
	if m.pokerGames == nil {
		m.pokerGames = make(map[string]*casino.IndianPokerGame)
	}
	cpy := game
	m.pokerGames[game.CharacterID] = &cpy
	return nil
}

func (m *mockCasinoRepo) GetActivePokerGame(ctx context.Context, charID string) (*casino.IndianPokerGame, error) {
	if m.getActivePokerGameFn != nil {
		return m.getActivePokerGameFn(ctx, charID)
	}
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

func TestCasinoService_PlayIndianPokerAction_EdgeCases(t *testing.T) {
	ctx := context.Background()

	t.Run("Invalid Character ID", func(t *testing.T) {
		repo := &mockCasinoRepo{}
		svc, _ := casino.NewService(repo)
		_, _, err := svc.PlayIndianPokerAction(ctx, "", casino.ActionCall)
		if !errors.Is(err, casino.ErrInvalidCharacterID) {
			t.Errorf("expected ErrInvalidCharacterID, got %v", err)
		}
	})

	t.Run("Invalid Action String", func(t *testing.T) {
		repo := &mockCasinoRepo{}
		svc, _ := casino.NewService(repo)
		_, _, err := svc.PlayIndianPokerAction(ctx, "char1", "invalid_action")
		if !errors.Is(err, casino.ErrInvalidAction) {
			t.Errorf("expected ErrInvalidAction, got %v", err)
		}
	})

	t.Run("No Active Poker Game", func(t *testing.T) {
		repo := &mockCasinoRepo{}
		svc, _ := casino.NewService(repo)
		_, _, err := svc.PlayIndianPokerAction(ctx, "char1", casino.ActionCall)
		if !errors.Is(err, casino.ErrNoActivePokerGame) {
			t.Errorf("expected ErrNoActivePokerGame, got %v", err)
		}
	})

	t.Run("GetAccountForUpdate Error", func(t *testing.T) {
		repo := &mockCasinoRepo{
			getAccountFn: func(_ context.Context, _ string) (casino.Account, error) {
				return casino.Account{}, errors.New("db error")
			},
		}
		svc, _ := casino.NewService(repo)
		_, _, err := svc.PlayIndianPokerAction(ctx, "char1", casino.ActionCall)
		if err == nil || err.Error() != "db error" {
			t.Errorf("expected db error, got %v", err)
		}
	})

	t.Run("GetActivePokerGame Error", func(t *testing.T) {
		repo := &mockCasinoRepo{
			getActivePokerGameFn: func(_ context.Context, _ string) (*casino.IndianPokerGame, error) {
				return nil, errors.New("game db error")
			},
		}
		svc, _ := casino.NewService(repo)
		_, _, err := svc.PlayIndianPokerAction(ctx, "char1", casino.ActionCall)
		if err == nil || err.Error() != "game db error" {
			t.Errorf("expected game db error, got %v", err)
		}
	})

	t.Run("Insufficient Coins for Call or Showdown", func(t *testing.T) {
		repo := &mockCasinoRepo{
			getAccountFn: func(_ context.Context, charID string) (casino.Account, error) {
				return casino.Account{CharacterID: charID, Coins: 5}, nil
			},
		}
		svc, _ := casino.NewService(repo)
		// Active game with CurrentBet = 10
		game, _ := casino.NewIndianPokerGame(10)
		game.CharacterID = "char1"
		repo.SavePokerGame(ctx, *game)

		_, _, err := svc.PlayIndianPokerAction(ctx, "char1", casino.ActionCall)
		if !errors.Is(err, casino.ErrInsufficientCoins) {
			t.Errorf("expected ErrInsufficientCoins, got %v", err)
		}
	})

	t.Run("Deduct Bet Error on Call", func(t *testing.T) {
		repo := &mockCasinoRepo{
			getAccountFn: func(_ context.Context, charID string) (casino.Account, error) {
				return casino.Account{CharacterID: charID, Coins: 500}, nil
			},
			deductAndCreditFn: func(_ context.Context, _ string, bet, payout int64) (casino.Account, error) {
				if bet > 0 {
					return casino.Account{}, errors.New("deduct error")
				}
				return casino.Account{}, nil
			},
		}
		svc, _ := casino.NewService(repo)
		game, _ := casino.NewIndianPokerGame(10)
		game.CharacterID = "char1"
		repo.SavePokerGame(ctx, *game)

		_, _, err := svc.PlayIndianPokerAction(ctx, "char1", casino.ActionCall)
		if err == nil || err.Error() != "deduct error" {
			t.Errorf("expected deduct error, got %v", err)
		}
	})

	t.Run("Showdown: Player Wins", func(t *testing.T) {
		var hookCalled bool
		repo := &mockCasinoRepo{}
		svc, _ := casino.NewService(repo)
		svc.SetGamePlayedHook(func(_ context.Context, _ string, gameType string) error {
			if gameType == "indian_poker" {
				hookCalled = true
			}
			return nil
		})

		game, _ := casino.NewIndianPokerGame(10)
		game.CharacterID = "char1"
		game.PlayerCard = casino.Card{Suit: casino.SuitSpades, Rank: casino.RankTen}
		game.DealerCard = casino.Card{Suit: casino.SuitHearts, Rank: casino.RankFive}
		repo.SavePokerGame(ctx, *game)

		finalGame, acc, err := svc.PlayIndianPokerAction(ctx, "char1", casino.ActionShowdown)
		if err != nil {
			t.Fatalf("Showdown error: %v", err)
		}
		if finalGame.Status != casino.StatusPlayerWon || finalGame.Winner != "player" {
			t.Errorf("unexpected game status: status=%v, winner=%s", finalGame.Status, finalGame.Winner)
		}
		if finalGame.PayoutCoins <= 0 {
			t.Errorf("expected payout > 0, got %d", finalGame.PayoutCoins)
		}
		if acc.Coins <= 0 {
			t.Errorf("expected credited coins, got %d", acc.Coins)
		}
		if !hookCalled {
			t.Errorf("expected gamePlayedHook to be called")
		}
	})

	t.Run("Showdown: Dealer Wins", func(t *testing.T) {
		repo := &mockCasinoRepo{}
		svc, _ := casino.NewService(repo)

		game, _ := casino.NewIndianPokerGame(10)
		game.CharacterID = "char1"
		game.PlayerCard = casino.Card{Suit: casino.SuitSpades, Rank: casino.RankThree}
		game.DealerCard = casino.Card{Suit: casino.SuitHearts, Rank: casino.RankTen}
		repo.SavePokerGame(ctx, *game)

		finalGame, _, err := svc.PlayIndianPokerAction(ctx, "char1", casino.ActionShowdown)
		if err != nil {
			t.Fatalf("Showdown error: %v", err)
		}
		if finalGame.Status != casino.StatusDealerWon || finalGame.Winner != "dealer" {
			t.Errorf("unexpected game status: status=%v, winner=%s", finalGame.Status, finalGame.Winner)
		}
		if finalGame.PayoutCoins != 0 {
			t.Errorf("expected 0 payout, got %d", finalGame.PayoutCoins)
		}
	})

	t.Run("Showdown: Tie / Draw", func(t *testing.T) {
		repo := &mockCasinoRepo{}
		svc, _ := casino.NewService(repo)

		game, _ := casino.NewIndianPokerGame(10)
		game.CharacterID = "char1"
		game.PlayerCard = casino.Card{Suit: casino.SuitSpades, Rank: casino.RankSeven}
		game.DealerCard = casino.Card{Suit: casino.SuitHearts, Rank: casino.RankSeven}
		repo.SavePokerGame(ctx, *game)

		finalGame, _, err := svc.PlayIndianPokerAction(ctx, "char1", casino.ActionShowdown)
		if err != nil {
			t.Fatalf("Showdown tie error: %v", err)
		}
		if finalGame.Status != casino.StatusTie || finalGame.Winner != "tie" {
			t.Errorf("unexpected tie status: status=%v, winner=%s", finalGame.Status, finalGame.Winner)
		}
		if finalGame.PayoutCoins <= 0 {
			t.Errorf("expected positive tie payout, got %d", finalGame.PayoutCoins)
		}
	})

	t.Run("SavePokerGame Error", func(t *testing.T) {
		repo := &mockCasinoRepo{
			savePokerGameFn: func(_ context.Context, _ casino.IndianPokerGame) error {
				return errors.New("save error")
			},
		}
		svc, _ := casino.NewService(repo)

		game, _ := casino.NewIndianPokerGame(10)
		game.CharacterID = "char1"
		repo.pokerGames = map[string]*casino.IndianPokerGame{"char1": game}

		_, _, err := svc.PlayIndianPokerAction(ctx, "char1", casino.ActionFold)
		if err == nil || err.Error() != "save error" {
			t.Errorf("expected save error, got %v", err)
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

func TestCasinoService_PlayIndianPokerAction_ExactCoinsBoundary(t *testing.T) {
	ctx := context.Background()
	// Start with exactly 20 coins: 10 for ante, 10 for round 1 bet
	var currentCoins int64 = 20

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
	svc, err := casino.NewService(repo)
	if err != nil {
		t.Fatal(err)
	}

	// 1. Start game with rate 10 -> Ante 10 deducted (exact 10 coins remaining)
	_, acc, err := svc.StartIndianPokerGame(ctx, "char1", 10)
	if err != nil {
		t.Fatalf("StartIndianPokerGame failed: %v", err)
	}
	if acc.Coins != 10 {
		t.Fatalf("expected 10 coins remaining, got %d", acc.Coins)
	}

	// Set cards so dealer calls
	repo.pokerGames["char1"].PlayerCard = casino.Card{Suit: casino.SuitSpades, Rank: casino.RankSeven}
	repo.pokerGames["char1"].DealerCard = casino.Card{Suit: casino.SuitHearts, Rank: casino.RankSeven}

	// 2. Play action 'call': current bet is 10, player has exactly 10 coins.
	// This must succeed and leave player with 0 coins during the round.
	_, updatedAcc, err := svc.PlayIndianPokerAction(ctx, "char1", casino.ActionCall)
	if err != nil {
		t.Fatalf("PlayIndianPokerAction failed on exact balance: %v", err)
	}
	if updatedAcc.Coins != 0 {
		t.Errorf("expected 0 coins remaining after bet, got %d", updatedAcc.Coins)
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
