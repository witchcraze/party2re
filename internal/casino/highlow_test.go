package casino_test

import (
	"context"
	"testing"

	"github.com/witchcraze/party2re/internal/casino"
)

func TestEvaluateHighLow(t *testing.T) {
	c5 := casino.Card{Suit: casino.SuitHearts, Rank: casino.RankFive}
	c10 := casino.Card{Suit: casino.SuitSpades, Rank: casino.RankTen}
	cKing := casino.Card{Suit: casino.SuitDiamonds, Rank: casino.RankKing}
	cAce := casino.Card{Suit: casino.SuitClubs, Rank: casino.RankAce}

	// 1. Current 5, Next 10, Guess High -> WIN (2x)
	res := casino.EvaluateHighLow(c5, c10, casino.GuessHigh, 50)
	if res.Outcome != casino.OutcomeWin || res.Multiplier != 2 || res.PayoutCoins != 100 || res.NetCoins != 50 {
		t.Errorf("5 vs 10 Guess High: outcome=%v, payout=%d, net=%d", res.Outcome, res.PayoutCoins, res.NetCoins)
	}

	// 2. Current 10, Next King, Guess Low -> LOSE
	res = casino.EvaluateHighLow(c10, cKing, casino.GuessLow, 50)
	if res.Outcome != casino.OutcomeLoss || res.Multiplier != 0 || res.PayoutCoins != 0 || res.NetCoins != -50 {
		t.Errorf("10 vs King Guess Low: outcome=%v, payout=%d, net=%d", res.Outcome, res.PayoutCoins, res.NetCoins)
	}

	// 3. Current 10, Next Ace, Guess Low -> WIN
	res = casino.EvaluateHighLow(c10, cAce, casino.GuessLow, 100)
	if res.Outcome != casino.OutcomeWin || res.Multiplier != 2 || res.PayoutCoins != 200 || res.NetCoins != 100 {
		t.Errorf("10 vs Ace Guess Low: outcome=%v, payout=%d, net=%d", res.Outcome, res.PayoutCoins, res.NetCoins)
	}

	// 4. Tie / Push (Current 5 vs Next 5) -> PUSH / TIE (1x refund)
	c5Spades := casino.Card{Suit: casino.SuitSpades, Rank: casino.RankFive}
	res = casino.EvaluateHighLow(c5, c5Spades, casino.GuessHigh, 100)
	if res.Outcome != casino.OutcomeTie || res.Multiplier != 1 || res.PayoutCoins != 100 || res.NetCoins != 0 {
		t.Errorf("5 vs 5 Guess High: outcome=%v, payout=%d, net=%d", res.Outcome, res.PayoutCoins, res.NetCoins)
	}
}

func TestHighLowSession_Streak(t *testing.T) {
	session, err := casino.NewHighLowSession(100)
	if err != nil {
		t.Fatalf("NewHighLowSession failed: %v", err)
	}

	if session.AccumulatedCoins != 100 || session.Streak != 0 {
		t.Errorf("initial session: accumulated=%d, streak=%d", session.AccumulatedCoins, session.Streak)
	}

	// Invalid Guess
	if _, err := session.Step("INVALID"); err != casino.ErrInvalidGuess {
		t.Errorf("expected ErrInvalidGuess, got %v", err)
	}
}

func TestService_PlayHighLow(t *testing.T) {
	ctx := context.Background()
	mockRepo := &mockCasinoRepo{}
	svc, _ := casino.NewService(mockRepo)

	res, acc, err := svc.PlayHighLow(ctx, "char1", 50, casino.GuessHigh)
	if err != nil {
		t.Fatalf("PlayHighLow failed: %v", err)
	}
	if res.BetCoins != 50 {
		t.Errorf("res.BetCoins = %d, want 50", res.BetCoins)
	}
	if acc.Coins != 1000+res.NetCoins {
		t.Errorf("account coins = %d, want %d", acc.Coins, 1000+res.NetCoins)
	}
}
