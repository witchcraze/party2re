package casino_test

import (
	"testing"

	"github.com/witchcraze/party2re/internal/casino"
)

func TestNewIndianPokerGame_Validation(t *testing.T) {
	tests := []struct {
		name     string
		baseRate int64
		wantErr  bool
	}{
		{"Zero base rate", 0, true},
		{"Negative base rate", -10, true},
		{"Too high base rate", 6000, true},
		{"Valid min base rate", 1, false},
		{"Valid normal base rate", 50, false},
		{"Valid max base rate", 5000, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			game, err := casino.NewIndianPokerGame(tt.baseRate)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewIndianPokerGame() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr {
				if game.BaseRate != tt.baseRate {
					t.Errorf("base rate = %d, want %d", game.BaseRate, tt.baseRate)
				}
				if game.Round != 1 {
					t.Errorf("initial round = %d, want 1", game.Round)
				}
				if game.Pot != tt.baseRate*2 {
					t.Errorf("initial pot = %d, want %d", game.Pot, tt.baseRate*2)
				}
				if game.Status != casino.StatusInProgress {
					t.Errorf("initial status = %v, want %v", game.Status, casino.StatusInProgress)
				}
			}
		})
	}
}

func TestIndianPokerGame_PlayerFold(t *testing.T) {
	game, err := casino.NewIndianPokerGame(10)
	if err != nil {
		t.Fatalf("NewIndianPokerGame error: %v", err)
	}

	if err := game.PlayRound(casino.ActionFold, 100); err != nil {
		t.Fatalf("PlayRound fold error: %v", err)
	}

	if game.Status != casino.StatusPlayerFolded {
		t.Errorf("status = %v, want %v", game.Status, casino.StatusPlayerFolded)
	}
	if game.Winner != "dealer" {
		t.Errorf("winner = %s, want dealer", game.Winner)
	}
	if game.PayoutCoins != 0 {
		t.Errorf("payout = %d, want 0", game.PayoutCoins)
	}

	// Playing after game over should error
	if err := game.PlayRound(casino.ActionCall, 100); err != casino.ErrGameAlreadyOver {
		t.Errorf("err after game over = %v, want ErrGameAlreadyOver", err)
	}
}

func TestIndianPokerGame_Showdown(t *testing.T) {
	t.Run("Player card higher -> Player wins", func(t *testing.T) {
		game, _ := casino.NewIndianPokerGame(10)
		game.PlayerCard = casino.Card{Suit: casino.SuitSpades, Rank: casino.RankKing}
		game.DealerCard = casino.Card{Suit: casino.SuitHearts, Rank: casino.RankTwo}

		err := game.PlayRound(casino.ActionShowdown, 100)
		if err != nil {
			t.Fatalf("PlayRound error: %v", err)
		}
		if game.Status != casino.StatusPlayerWon && game.Status != casino.StatusDealerFolded {
			t.Errorf("expected player win or dealer fold, got %v", game.Status)
		}
		if game.PayoutCoins < 30 { // Initial 20 pot + 10 player round bet (dealer might fold or match)
			t.Errorf("payout = %d, expected >= 30", game.PayoutCoins)
		}
	})

	t.Run("Dealer card higher -> Dealer wins", func(t *testing.T) {
		game, _ := casino.NewIndianPokerGame(10)
		game.PlayerCard = casino.Card{Suit: casino.SuitSpades, Rank: casino.RankTwo}
		game.DealerCard = casino.Card{Suit: casino.SuitHearts, Rank: casino.RankKing}

		err := game.PlayRound(casino.ActionShowdown, 100)
		if err != nil {
			t.Fatalf("PlayRound error: %v", err)
		}
		if game.Status != casino.StatusDealerWon {
			t.Errorf("expected dealer win, got status %v", game.Status)
		}
		if game.PayoutCoins != 0 {
			t.Errorf("payout = %d, want 0", game.PayoutCoins)
		}
	})

	t.Run("Tie ranks -> Tie pot returned", func(t *testing.T) {
		game, _ := casino.NewIndianPokerGame(10)
		game.PlayerCard = casino.Card{Suit: casino.SuitSpades, Rank: casino.RankTen}
		game.DealerCard = casino.Card{Suit: casino.SuitHearts, Rank: casino.RankTen}

		err := game.PlayRound(casino.ActionShowdown, 100)
		if err != nil {
			t.Fatalf("PlayRound error: %v", err)
		}
		if game.Status != casino.StatusTie && game.Status != casino.StatusDealerFolded {
			t.Errorf("expected tie or dealer fold, got %v", game.Status)
		}
		if game.Status == casino.StatusTie && game.PayoutCoins != game.PlayerCommittedCoins {
			t.Errorf("payout = %d, want %d", game.PayoutCoins, game.PlayerCommittedCoins)
		}
	})
}

func TestIndianPokerGame_MultiRoundProgression(t *testing.T) {
	game, _ := casino.NewIndianPokerGame(10)
	game.PlayerCard = casino.Card{Suit: casino.SuitSpades, Rank: casino.RankSeven}
	game.DealerCard = casino.Card{Suit: casino.SuitHearts, Rank: casino.RankSeven}

	// Play until round reaches showdown or max rounds
	for game.Status == casino.StatusInProgress {
		prevRound := game.Round
		err := game.PlayRound(casino.ActionCall, 1000)
		if err != nil {
			t.Fatalf("PlayRound error: %v", err)
		}
		if game.Status == casino.StatusInProgress {
			if game.Round != prevRound+1 {
				t.Errorf("round = %d, want %d", game.Round, prevRound+1)
			}
			if game.CurrentBet != game.BaseRate*int64(game.Round) {
				t.Errorf("current bet = %d, want %d", game.CurrentBet, game.BaseRate*int64(game.Round))
			}
		}
	}

	if game.Round > game.MaxRounds {
		t.Errorf("game round exceeded max rounds: %d", game.Round)
	}
}

func TestIndianPokerGame_InsufficientCoins(t *testing.T) {
	game, _ := casino.NewIndianPokerGame(50)
	err := game.PlayRound(casino.ActionCall, 10) // Only 10 coins available, but current bet is 50
	if err != casino.ErrInsufficientCoin {
		t.Errorf("err = %v, want ErrInsufficientCoin", err)
	}
}
