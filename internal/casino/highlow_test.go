package casino_test

import (
	"context"
	"errors"
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

	// 5. Tie / Push with GuessLow -> PUSH / TIE (1x refund)
	res = casino.EvaluateHighLow(c5, c5Spades, casino.GuessLow, 100)
	if res.Outcome != casino.OutcomeTie || res.Multiplier != 1 || res.PayoutCoins != 100 || res.NetCoins != 0 {
		t.Errorf("5 vs 5 Guess Low: outcome=%v, payout=%d, net=%d", res.Outcome, res.PayoutCoins, res.NetCoins)
	}
}

func TestHighLowSession_Step_FullCoverage(t *testing.T) {
	// Bet boundary validation
	if _, err := casino.NewHighLowSession(0); !errors.Is(err, casino.ErrInvalidHighLowBet) {
		t.Errorf("expected ErrInvalidHighLowBet for 0 bet, got %v", err)
	}
	if _, err := casino.NewHighLowSession(5001); !errors.Is(err, casino.ErrInvalidHighLowBet) {
		t.Errorf("expected ErrInvalidHighLowBet for 5001 bet, got %v", err)
	}

	session, err := casino.NewHighLowSession(100)
	if err != nil {
		t.Fatalf("NewHighLowSession failed: %v", err)
	}
	if session.AccumulatedCoins != 100 || session.Streak != 0 || session.IsOver {
		t.Fatalf("unexpected initial session state: %+v", session)
	}

	// 1. Invalid guess
	if _, err := session.Step("INVALID"); !errors.Is(err, casino.ErrInvalidGuess) {
		t.Errorf("expected ErrInvalidGuess, got %v", err)
	}

	// 2. Win with GuessHigh: current 5, next 10 -> coins double (200), streak=1
	session.CurrentCard = casino.Card{Suit: casino.SuitHearts, Rank: casino.RankFive}
	c10 := casino.Card{Suit: casino.SuitSpades, Rank: casino.RankTen}
	session.Deck = casino.NewCustomDeck([]casino.Card{c10})
	res, err := session.Step(casino.GuessHigh)
	if err != nil {
		t.Fatalf("Step high win error: %v", err)
	}
	if res.Outcome != casino.OutcomeWin || session.Streak != 1 || session.AccumulatedCoins != 200 || session.CurrentCard != c10 {
		t.Errorf("unexpected win state: outcome=%v, streak=%d, coins=%d, card=%+v", res.Outcome, session.Streak, session.AccumulatedCoins, session.CurrentCard)
	}

	// 3. Tie / Push: current 10, next 10 -> coins preserved (200), streak preserved (1)
	c10Clubs := casino.Card{Suit: casino.SuitClubs, Rank: casino.RankTen}
	session.Deck = casino.NewCustomDeck([]casino.Card{c10Clubs})
	res, err = session.Step(casino.GuessHigh)
	if err != nil {
		t.Fatalf("Step tie error: %v", err)
	}
	if res.Outcome != casino.OutcomeTie || session.Streak != 1 || session.AccumulatedCoins != 200 || session.CurrentCard != c10Clubs {
		t.Errorf("unexpected tie state: outcome=%v, streak=%d, coins=%d", res.Outcome, session.Streak, session.AccumulatedCoins)
	}

	// 4. Win with GuessLow: current 10, next 3 -> coins double (400), streak=2
	c3 := casino.Card{Suit: casino.SuitDiamonds, Rank: casino.RankThree}
	session.Deck = casino.NewCustomDeck([]casino.Card{c3})
	res, err = session.Step(casino.GuessLow)
	if err != nil {
		t.Fatalf("Step low win error: %v", err)
	}
	if res.Outcome != casino.OutcomeWin || session.Streak != 2 || session.AccumulatedCoins != 400 || session.CurrentCard != c3 {
		t.Errorf("unexpected low win state: outcome=%v, streak=%d, coins=%d", res.Outcome, session.Streak, session.AccumulatedCoins)
	}

	// 5. Empty deck triggers reshuffle and draws successfully
	emptyDeckSession, err := casino.NewHighLowSession(100)
	if err != nil {
		t.Fatal(err)
	}
	emptyDeckSession.Deck = casino.NewCustomDeck([]casino.Card{})
	if emptyDeckSession.Deck.Remaining() != 0 {
		t.Fatalf("expected 0 remaining cards")
	}
	_, err = emptyDeckSession.Step(casino.GuessHigh)
	if err != nil {
		t.Fatalf("Step with empty deck reshuffle error: %v", err)
	}
	if emptyDeckSession.Deck.Remaining() == 0 {
		t.Errorf("expected reshuffled deck to have cards remaining")
	}

	// 6. Loss: current King, next 2, guess High -> streak=0, coins=0, IsOver=true
	session.CurrentCard = casino.Card{Suit: casino.SuitSpades, Rank: casino.RankKing}
	c2 := casino.Card{Suit: casino.SuitHearts, Rank: casino.RankTwo}
	session.Deck = casino.NewCustomDeck([]casino.Card{c2})
	res, err = session.Step(casino.GuessHigh)
	if err != nil {
		t.Fatalf("Step loss error: %v", err)
	}
	if res.Outcome != casino.OutcomeLoss || session.Streak != 0 || session.AccumulatedCoins != 0 || !session.IsOver {
		t.Errorf("unexpected loss state: outcome=%v, streak=%d, coins=%d, isOver=%v", res.Outcome, session.Streak, session.AccumulatedCoins, session.IsOver)
	}

	// 7. Step when game is already over returns ErrSessionGameOver
	if _, err := session.Step(casino.GuessHigh); !errors.Is(err, casino.ErrSessionGameOver) {
		t.Errorf("expected ErrSessionGameOver, got %v", err)
	}
}

func TestService_PlayHighLow(t *testing.T) {
	ctx := context.Background()
	var hookCalled bool
	hook := func(_ context.Context, _ string, gameType string) error {
		if gameType == "highlow" {
			hookCalled = true
		}
		return nil
	}

	mockRepo := &mockCasinoRepo{}
	svc, _ := casino.NewService(mockRepo)
	svc.SetGamePlayedHook(hook)

	// Validation errors
	if _, _, err := svc.PlayHighLow(ctx, "", 50, casino.GuessHigh); !errors.Is(err, casino.ErrInvalidCharacterID) {
		t.Errorf("expected ErrInvalidCharacterID, got %v", err)
	}
	if _, _, err := svc.PlayHighLow(ctx, "char1", 0, casino.GuessHigh); !errors.Is(err, casino.ErrInvalidHighLowBet) {
		t.Errorf("expected ErrInvalidHighLowBet for 0, got %v", err)
	}
	if _, _, err := svc.PlayHighLow(ctx, "char1", 5001, casino.GuessHigh); !errors.Is(err, casino.ErrInvalidHighLowBet) {
		t.Errorf("expected ErrInvalidHighLowBet for 5001, got %v", err)
	}
	if _, _, err := svc.PlayHighLow(ctx, "char1", 50, "BAD_GUESS"); !errors.Is(err, casino.ErrInvalidGuess) {
		t.Errorf("expected ErrInvalidGuess, got %v", err)
	}

	// Valid play
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
	if !hookCalled {
		t.Errorf("expected gamePlayedHook to be called")
	}
}
