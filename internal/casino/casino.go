package casino

import (
	"context"
	"errors"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/id"
)

const (
	GoldPerCoin = 20 // 1 Casino Coin = 20 Gold
)

var (
	ErrInvalidAmount       = errors.New("amount must be positive")
	ErrInsufficientGold    = errors.New("insufficient character gold for coin exchange")
	ErrInsufficientCoins   = errors.New("insufficient casino coins")
	ErrInvalidCharacterID  = errors.New("character ID cannot be empty")
	ErrActiveSessionExists = errors.New("active indian poker game already exists")
	ErrNoActivePokerGame   = errors.New("no active indian poker game found")
)

type Account struct {
	CharacterID string    `json:"character_id"`
	Coins       int64     `json:"coins"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type Repository interface {
	GetAccount(ctx context.Context, characterID string) (Account, error)
	GetAccountForUpdate(ctx context.Context, characterID string) (Account, error)
	ExchangeGoldToCoins(ctx context.Context, characterID string, coins int64, goldCost int) (Account, corecharacter.Character, error)
	ExchangeCoinsToGold(ctx context.Context, characterID string, coins int64, goldReward int) (Account, corecharacter.Character, error)
	AdjustCoins(ctx context.Context, characterID string, coinDelta int64) (Account, error)
	DeductBetAndCreditPayout(ctx context.Context, characterID string, bet int64, payout int64) (Account, error)
	SavePokerGame(ctx context.Context, game IndianPokerGame) error
	GetActivePokerGame(ctx context.Context, characterID string) (*IndianPokerGame, error)
	GetActivePokerGameForUpdate(ctx context.Context, characterID string) (*IndianPokerGame, error)
}

// TransactionProvider can be injected into Service to orchestrate database transactions.
type TransactionProvider interface {
	RunInTx(ctx context.Context, fn func(ctx context.Context) error) error
}

// GamePlayedHook is called whenever a casino game round concludes.
type GamePlayedHook func(ctx context.Context, characterID string, gameName string) error

type Service struct {
	repo           Repository
	txProvider     TransactionProvider
	gamePlayedHook GamePlayedHook
}

type Option func(*Service)

func WithTransactionProvider(tx TransactionProvider) Option {
	return func(s *Service) {
		s.txProvider = tx
	}
}

func (s *Service) SetTransactionProvider(tx TransactionProvider) {
	s.txProvider = tx
}

func (s *Service) runInTx(ctx context.Context, fn func(ctx context.Context) error) error {
	if s.txProvider != nil {
		return s.txProvider.RunInTx(ctx, fn)
	}
	return fn(ctx)
}

func (s *Service) SetGamePlayedHook(hook GamePlayedHook) {
	s.gamePlayedHook = hook
}

func NewService(repo Repository, opts ...Option) (*Service, error) {
	if repo == nil {
		return nil, errors.New("casino repository is required")
	}
	s := &Service{repo: repo}
	for _, opt := range opts {
		opt(s)
	}
	return s, nil
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

// StartIndianPokerGame initializes a new Indian Poker game, persists the active session,
// and deducts the initial ante from the character's casino account atomically.
func (s *Service) StartIndianPokerGame(ctx context.Context, characterID string, baseRate int64) (*IndianPokerGame, Account, error) {
	if characterID == "" {
		return nil, Account{}, ErrInvalidCharacterID
	}
	if baseRate < MinBaseRate || baseRate > MaxBaseRate {
		return nil, Account{}, ErrInvalidBaseRate
	}

	var newGame *IndianPokerGame
	var acc Account

	err := s.runInTx(ctx, func(txCtx context.Context) error {
		var err error
		acc, err = s.repo.GetAccountForUpdate(txCtx, characterID)
		if err != nil {
			return err
		}

		existing, err := s.repo.GetActivePokerGame(txCtx, characterID)
		if err != nil && !errors.Is(err, ErrNoActivePokerGame) {
			return err
		}
		if existing != nil && existing.Status == StatusInProgress {
			return ErrActiveSessionExists
		}

		acc, err = s.repo.DeductBetAndCreditPayout(txCtx, characterID, baseRate, 0)
		if err != nil {
			return err
		}

		game, err := NewIndianPokerGame(baseRate)
		if err != nil {
			return err
		}

		now := time.Now().UTC()
		game.ID = id.New()
		game.CharacterID = characterID
		game.CreatedAt = now
		game.UpdatedAt = now

		if err := s.repo.SavePokerGame(txCtx, *game); err != nil {
			return err
		}

		newGame = game
		return nil
	})
	if err != nil {
		return nil, Account{}, err
	}

	return newGame.ClientView(), acc, nil
}

// GetActiveIndianPokerGame returns the active in-progress Indian Poker game for the character.
func (s *Service) GetActiveIndianPokerGame(ctx context.Context, characterID string) (*IndianPokerGame, Account, error) {
	if characterID == "" {
		return nil, Account{}, ErrInvalidCharacterID
	}

	game, err := s.repo.GetActivePokerGame(ctx, characterID)
	if err != nil {
		return nil, Account{}, err
	}
	if game == nil || game.Status != StatusInProgress {
		return nil, Account{}, ErrNoActivePokerGame
	}

	acc, err := s.repo.GetAccount(ctx, characterID)
	if err != nil {
		return nil, Account{}, err
	}

	return game.ClientView(), acc, nil
}

// PlayIndianPokerAction plays one round action ('call', 'showdown', or 'fold') on the active poker game,
// atomically locking the session and settling bets/payouts in casino_accounts.
func (s *Service) PlayIndianPokerAction(ctx context.Context, characterID string, action Action) (*IndianPokerGame, Account, error) {
	if characterID == "" {
		return nil, Account{}, ErrInvalidCharacterID
	}
	if !action.Valid() {
		return nil, Account{}, ErrInvalidAction
	}

	var updatedGame *IndianPokerGame
	var updatedAcc Account

	err := s.runInTx(ctx, func(txCtx context.Context) error {
		acc, err := s.repo.GetAccountForUpdate(txCtx, characterID)
		if err != nil {
			return err
		}

		game, err := s.repo.GetActivePokerGame(txCtx, characterID)
		if err != nil {
			return err
		}
		if game == nil || game.Status != StatusInProgress {
			return ErrNoActivePokerGame
		}

		if action == ActionCall || action == ActionShowdown {
			neededBet := game.CurrentBet
			if acc.Coins < neededBet {
				return ErrInsufficientCoin
			}
			acc, err = s.repo.DeductBetAndCreditPayout(txCtx, characterID, neededBet, 0)
			if err != nil {
				return err
			}
		}

		if err := game.PlayRound(action, acc.Coins); err != nil {
			return err
		}

		if game.Status != StatusInProgress && game.PayoutCoins > 0 {
			acc, err = s.repo.DeductBetAndCreditPayout(txCtx, characterID, 0, game.PayoutCoins)
			if err != nil {
				return err
			}
		}

		game.UpdatedAt = time.Now().UTC()
		if err := s.repo.SavePokerGame(txCtx, *game); err != nil {
			return err
		}

		updatedGame = game
		updatedAcc = acc
		return nil
	})
	if err != nil {
		return nil, Account{}, err
	}

	if updatedGame.Status != StatusInProgress && s.gamePlayedHook != nil {
		_ = s.gamePlayedHook(ctx, characterID, "indian_poker")
	}

	return updatedGame.ClientView(), updatedAcc, nil
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

	// 1. If action requires bet, deduct atomically from account
	if action == ActionCall || action == ActionShowdown {
		neededBet := game.CurrentBet
		acc, err = s.repo.DeductBetAndCreditPayout(ctx, characterID, neededBet, 0)
		if err != nil {
			return acc, err
		}
	}

	// 2. Play the round
	if err := game.PlayRound(action, acc.Coins); err != nil {
		// Refund if round failed unexpectedly
		if action == ActionCall || action == ActionShowdown {
			_, _ = s.repo.DeductBetAndCreditPayout(ctx, characterID, 0, game.CurrentBet)
		}
		return acc, err
	}

	// 3. If game finished and has payout, credit account
	if game.Status != StatusInProgress && game.PayoutCoins > 0 {
		acc, err = s.repo.DeductBetAndCreditPayout(ctx, characterID, 0, game.PayoutCoins)
		if err != nil {
			return acc, err
		}
	}

	if game.Status != StatusInProgress && s.gamePlayedHook != nil {
		_ = s.gamePlayedHook(ctx, characterID, "indian_poker")
	}

	return acc, nil
}

// SpinSlot executes a slot machine spin, adjusts coins atomically according to the outcome, and returns the result and updated account.
func (s *Service) SpinSlot(ctx context.Context, characterID string, bet int64) (SpinResult, Account, error) {
	if characterID == "" {
		return SpinResult{}, Account{}, ErrInvalidCharacterID
	}
	if !ValidBetRates[bet] {
		return SpinResult{}, Account{}, ErrInvalidBetRate
	}

	res, err := SpinSlotMachine(bet)
	if err != nil {
		return SpinResult{}, Account{}, err
	}

	// Atomically verify/deduct bet and credit payout
	acc, err := s.repo.DeductBetAndCreditPayout(ctx, characterID, bet, res.PayoutCoins)
	if err != nil {
		return SpinResult{}, Account{}, err
	}

	if s.gamePlayedHook != nil {
		_ = s.gamePlayedHook(ctx, characterID, "slot")
	}

	return res, acc, nil
}

// PlayDoppel executes a Doppelganger mark-matching game, adjusts coins atomically according to outcome, and returns the result and updated account.
func (s *Service) PlayDoppel(ctx context.Context, characterID string, bet int64, poolSize int, playerMark DoppelMark) (DoppelResult, Account, error) {
	if characterID == "" {
		return DoppelResult{}, Account{}, ErrInvalidCharacterID
	}
	if bet < MinBaseRate || bet > MaxBaseRate {
		return DoppelResult{}, Account{}, ErrInvalidDoppelBet
	}

	res, err := PlayDoppelGame(bet, poolSize, playerMark)
	if err != nil {
		return DoppelResult{}, Account{}, err
	}

	// Atomically verify/deduct bet and credit payout
	acc, err := s.repo.DeductBetAndCreditPayout(ctx, characterID, bet, res.PayoutCoins)
	if err != nil {
		return DoppelResult{}, Account{}, err
	}

	if s.gamePlayedHook != nil {
		_ = s.gamePlayedHook(ctx, characterID, "doppel")
	}

	return res, acc, nil
}
