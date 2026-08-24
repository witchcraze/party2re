package casino

import (
	"context"
	"errors"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

const (
	GoldPerCoin = 20 // 1 Casino Coin = 20 Gold
)

var (
	ErrInvalidAmount      = errors.New("amount must be positive")
	ErrInsufficientGold   = errors.New("insufficient character gold for coin exchange")
	ErrInsufficientCoins  = errors.New("insufficient casino coins")
	ErrInvalidCharacterID = errors.New("character ID cannot be empty")
)

type Account struct {
	CharacterID string    `json:"character_id"`
	Coins       int64     `json:"coins"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type Repository interface {
	GetAccount(ctx context.Context, characterID string) (Account, error)
	ExchangeGoldToCoins(ctx context.Context, characterID string, coins int64, goldCost int) (Account, corecharacter.Character, error)
	ExchangeCoinsToGold(ctx context.Context, characterID string, coins int64, goldReward int) (Account, corecharacter.Character, error)
	AdjustCoins(ctx context.Context, characterID string, coinDelta int64) (Account, error)
}

type Service struct {
	repo Repository
}

func NewService(repo Repository) (*Service, error) {
	if repo == nil {
		return nil, errors.New("casino repository is required")
	}
	return &Service{repo: repo}, nil
}

func (s *Service) GetAccount(ctx context.Context, characterID string) (Account, error) {
	if characterID == "" {
		return Account{}, ErrInvalidCharacterID
	}
	return s.repo.GetAccount(ctx, characterID)
}

// ExchangeGoldToCoins buys casino coins with character gold (1 coin = 20 gold).
func (s *Service) ExchangeGoldToCoins(ctx context.Context, characterID string, coins int64) (Account, corecharacter.Character, error) {
	if characterID == "" {
		return Account{}, corecharacter.Character{}, ErrInvalidCharacterID
	}
	if coins <= 0 {
		return Account{}, corecharacter.Character{}, ErrInvalidAmount
	}
	goldCost := int(coins * GoldPerCoin)
	return s.repo.ExchangeGoldToCoins(ctx, characterID, coins, goldCost)
}

// ExchangeCoinsToGold sells casino coins back for character gold (1 coin = 20 gold).
func (s *Service) ExchangeCoinsToGold(ctx context.Context, characterID string, coins int64) (Account, corecharacter.Character, error) {
	if characterID == "" {
		return Account{}, corecharacter.Character{}, ErrInvalidCharacterID
	}
	if coins <= 0 {
		return Account{}, corecharacter.Character{}, ErrInvalidAmount
	}
	goldReward := int(coins * GoldPerCoin)
	return s.repo.ExchangeCoinsToGold(ctx, characterID, coins, goldReward)
}

// StartIndianPokerGame initializes a new Indian Poker game and deducts the initial ante from the character's casino account.
func (s *Service) StartIndianPokerGame(ctx context.Context, characterID string, baseRate int64) (*IndianPokerGame, Account, error) {
	if characterID == "" {
		return nil, Account{}, ErrInvalidCharacterID
	}
	if baseRate < MinBaseRate || baseRate > MaxBaseRate {
		return nil, Account{}, ErrInvalidBaseRate
	}

	// Deduct initial ante
	acc, err := s.repo.AdjustCoins(ctx, characterID, -baseRate)
	if err != nil {
		return nil, Account{}, err
	}

	game, err := NewIndianPokerGame(baseRate)
	if err != nil {
		// Refund ante on failure
		_, _ = s.repo.AdjustCoins(ctx, characterID, baseRate)
		return nil, Account{}, err
	}

	return game, acc, nil
}

// PlayIndianPokerRound plays one betting round for the game and settles coin changes when the game concludes.
func (s *Service) PlayIndianPokerRound(ctx context.Context, characterID string, game *IndianPokerGame, action Action) (Account, error) {
	if characterID == "" {
		return Account{}, ErrInvalidCharacterID
	}
	if game == nil {
		return Account{}, errors.New("game is nil")
	}

	acc, err := s.repo.GetAccount(ctx, characterID)
	if err != nil {
		return Account{}, err
	}

	// 1. If action requires bet, deduct from account
	if action == ActionCall || action == ActionShowdown {
		neededBet := game.CurrentBet
		if acc.Coins < neededBet {
			return acc, ErrInsufficientCoins
		}
		// Deduct bet coins
		acc, err = s.repo.AdjustCoins(ctx, characterID, -neededBet)
		if err != nil {
			return acc, err
		}
	}

	// 2. Play the round
	if err := game.PlayRound(action, acc.Coins); err != nil {
		// Refund if round failed unexpectedly
		if action == ActionCall || action == ActionShowdown {
			_, _ = s.repo.AdjustCoins(ctx, characterID, game.CurrentBet)
		}
		return acc, err
	}

	// 3. If game finished and has payout, credit account
	if game.Status != StatusInProgress && game.PayoutCoins > 0 {
		acc, err = s.repo.AdjustCoins(ctx, characterID, game.PayoutCoins)
		if err != nil {
			return acc, err
		}
	}

	return acc, nil
}

// SpinSlot executes a slot machine spin, adjusts coins according to the outcome, and returns the result and updated account.
func (s *Service) SpinSlot(ctx context.Context, characterID string, bet int64) (SpinResult, Account, error) {
	if characterID == "" {
		return SpinResult{}, Account{}, ErrInvalidCharacterID
	}
	if !ValidBetRates[bet] {
		return SpinResult{}, Account{}, ErrInvalidBetRate
	}

	acc, err := s.repo.GetAccount(ctx, characterID)
	if err != nil {
		return SpinResult{}, Account{}, err
	}
	if acc.Coins < bet {
		return SpinResult{}, acc, ErrInsufficientCoins
	}

	res, err := SpinSlotMachine(bet)
	if err != nil {
		return SpinResult{}, acc, err
	}

	// Net coin delta = payout - bet
	coinDelta := res.NetCoins
	acc, err = s.repo.AdjustCoins(ctx, characterID, coinDelta)
	if err != nil {
		return SpinResult{}, acc, err
	}

	return res, acc, nil
}

// PlayDoppel executes a Doppelganger mark-matching game, adjusts coins according to outcome, and returns the result and updated account.
func (s *Service) PlayDoppel(ctx context.Context, characterID string, bet int64, poolSize int, playerMark DoppelMark) (DoppelResult, Account, error) {
	if characterID == "" {
		return DoppelResult{}, Account{}, ErrInvalidCharacterID
	}
	if bet < MinBaseRate || bet > MaxBaseRate {
		return DoppelResult{}, Account{}, ErrInvalidDoppelBet
	}

	acc, err := s.repo.GetAccount(ctx, characterID)
	if err != nil {
		return DoppelResult{}, Account{}, err
	}
	if acc.Coins < bet {
		return DoppelResult{}, acc, ErrInsufficientCoins
	}

	res, err := PlayDoppelGame(bet, poolSize, playerMark)
	if err != nil {
		return DoppelResult{}, acc, err
	}

	// Net coin delta = payout - bet
	coinDelta := res.NetCoins
	acc, err = s.repo.AdjustCoins(ctx, characterID, coinDelta)
	if err != nil {
		return DoppelResult{}, acc, err
	}

	return res, acc, nil
}
