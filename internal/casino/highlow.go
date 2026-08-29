package casino

import (
	"context"
	"errors"
	"fmt"
)

type GuessType string

const (
	GuessHigh GuessType = "HIGH"
	GuessLow  GuessType = "LOW"
)

const (
	MinHighLowBet int64 = 1
	MaxHighLowBet int64 = 5000
)

var (
	ErrInvalidHighLowBet = errors.New("high & low bet must be between 1 and 5000 coins")
	ErrInvalidGuess      = errors.New("guess must be HIGH or LOW")
	ErrSessionGameOver   = errors.New("high & low session is already over")
	ErrSessionNoStreak   = errors.New("no winnings to cash out")
)

type HighLowOutcome string

const (
	OutcomeWin  HighLowOutcome = "WIN"
	OutcomeLoss HighLowOutcome = "LOSS"
	OutcomeTie  HighLowOutcome = "TIE"
)

type HighLowResult struct {
	BetCoins    int64          `json:"bet_coins"`
	CurrentCard Card           `json:"current_card"`
	NextCard    Card           `json:"next_card"`
	Guess       GuessType      `json:"guess"`
	Outcome     HighLowOutcome `json:"outcome"`
	Multiplier  int            `json:"multiplier"`
	PayoutCoins int64          `json:"payout_coins"`
	NetCoins    int64          `json:"net_coins"`
	Message     string         `json:"message"`
}

// EvaluateHighLow evaluates a single High & Low round between current card and next card.
func EvaluateHighLow(currentCard, nextCard Card, guess GuessType, betCoins int64) HighLowResult {
	res := HighLowResult{
		BetCoins:    betCoins,
		CurrentCard: currentCard,
		NextCard:    nextCard,
		Guess:       guess,
	}

	if nextCard.Rank == currentCard.Rank {
		res.Outcome = OutcomeTie
		res.Multiplier = 1
		res.PayoutCoins = betCoins
		res.NetCoins = 0
		res.Message = fmt.Sprintf("PUSH! Current [%s] vs Next [%s] (Tie). Bet of %d coins returned.", currentCard, nextCard, betCoins)
		return res
	}

	isHigh := nextCard.Rank > currentCard.Rank
	isWin := (guess == GuessHigh && isHigh) || (guess == GuessLow && !isHigh)

	if isWin {
		res.Outcome = OutcomeWin
		res.Multiplier = 2
		res.PayoutCoins = betCoins * 2
		res.NetCoins = betCoins
		res.Message = fmt.Sprintf("WIN! Current [%s] vs Next [%s] (Guess: %s). Won %d coins (2x payout)!", currentCard, nextCard, guess, res.PayoutCoins)
	} else {
		res.Outcome = OutcomeLoss
		res.Multiplier = 0
		res.PayoutCoins = 0
		res.NetCoins = -betCoins
		res.Message = fmt.Sprintf("LOSE! Current [%s] vs Next [%s] (Guess: %s). Lost %d coins.", currentCard, nextCard, guess, betCoins)
	}

	return res
}

// HighLowSession tracks a multi-round consecutive streak game.
type HighLowSession struct {
	InitialBet       int64 `json:"initial_bet"`
	CurrentCard      Card  `json:"current_card"`
	Streak           int   `json:"streak"`
	AccumulatedCoins int64 `json:"accumulated_coins"`
	IsOver           bool  `json:"is_over"`
	Deck             *Deck `json:"-"`
}

func NewHighLowSession(initialBet int64) (*HighLowSession, error) {
	if initialBet < MinHighLowBet || initialBet > MaxHighLowBet {
		return nil, ErrInvalidHighLowBet
	}

	deck := NewStandardDeck()
	if err := deck.Shuffle(); err != nil {
		return nil, err
	}

	initialCard, err := deck.Draw()
	if err != nil {
		return nil, err
	}

	return &HighLowSession{
		InitialBet:       initialBet,
		CurrentCard:      initialCard,
		Streak:           0,
		AccumulatedCoins: initialBet,
		IsOver:           false,
		Deck:             deck,
	}, nil
}

func (s *HighLowSession) Step(guess GuessType) (HighLowResult, error) {
	if s.IsOver {
		return HighLowResult{}, ErrSessionGameOver
	}
	if guess != GuessHigh && guess != GuessLow {
		return HighLowResult{}, ErrInvalidGuess
	}

	// Reshuffle if low on cards
	if s.Deck.Remaining() == 0 {
		s.Deck = NewStandardDeck()
		_ = s.Deck.Shuffle()
	}

	nextCard, err := s.Deck.Draw()
	if err != nil {
		return HighLowResult{}, err
	}

	eval := EvaluateHighLow(s.CurrentCard, nextCard, guess, s.AccumulatedCoins)
	switch eval.Outcome {
	case OutcomeWin:
		s.Streak++
		s.AccumulatedCoins *= 2
		s.CurrentCard = nextCard
	case OutcomeTie:
		s.CurrentCard = nextCard
	case OutcomeLoss:
		s.Streak = 0
		s.AccumulatedCoins = 0
		s.IsOver = true
	}

	return eval, nil
}

// PlayHighLow plays an instant one-round High & Low game.
func (s *Service) PlayHighLow(ctx context.Context, characterID string, betCoins int64, guess GuessType) (HighLowResult, Account, error) {
	if characterID == "" {
		return HighLowResult{}, Account{}, ErrInvalidCharacterID
	}
	if betCoins < MinHighLowBet || betCoins > MaxHighLowBet {
		return HighLowResult{}, Account{}, ErrInvalidHighLowBet
	}
	if guess != GuessHigh && guess != GuessLow {
		return HighLowResult{}, Account{}, ErrInvalidGuess
	}

	deck := NewStandardDeck()
	if err := deck.Shuffle(); err != nil {
		return HighLowResult{}, Account{}, err
	}

	c1, err := deck.Draw()
	if err != nil {
		return HighLowResult{}, Account{}, err
	}
	c2, err := deck.Draw()
	if err != nil {
		return HighLowResult{}, Account{}, err
	}

	res := EvaluateHighLow(c1, c2, guess, betCoins)

	// Deduct bet and credit payout atomically
	acc, err := s.repo.DeductBetAndCreditPayout(ctx, characterID, betCoins, res.PayoutCoins)
	if err != nil {
		return HighLowResult{}, Account{}, err
	}

	return res, acc, nil
}
