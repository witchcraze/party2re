package casino

import (
	"context"
	"errors"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	"github.com/witchcraze/party2re/internal/economy"
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
	GetAccountForUpdate(ctx context.Context, characterID string) (Account, error)
	AdjustCoins(ctx context.Context, characterID string, coinDelta int64) (Account, error)
	DeductBetAndCreditPayout(ctx context.Context, characterID string, bet int64, payout int64) (Account, error)
}

// CharacterRepository defines character persistence required for casino operations and economy transactions.
type CharacterRepository interface {
	FindByID(ctx context.Context, id string) (corecharacter.Character, error)
	FindByIDForUpdate(ctx context.Context, id string) (corecharacter.Character, error)
	Update(ctx context.Context, character corecharacter.Character) error
}

// InventoryRepository defines inventory persistence required for economy transactions.
type InventoryRepository interface {
	FindByCharacterID(ctx context.Context, characterID string) (coreinventory.Inventory, error)
	FindByCharacterIDForUpdate(ctx context.Context, characterID string) (coreinventory.Inventory, error)
	Save(ctx context.Context, inventory coreinventory.Inventory) error
}

type noopInventoryRepo struct{}

func (noopInventoryRepo) FindByCharacterID(_ context.Context, charID string) (coreinventory.Inventory, error) {
	return coreinventory.New(charID)
}

func (noopInventoryRepo) FindByCharacterIDForUpdate(_ context.Context, charID string) (coreinventory.Inventory, error) {
	return coreinventory.New(charID)
}

func (noopInventoryRepo) Save(_ context.Context, _ coreinventory.Inventory) error {
	return nil
}

// TransactionProvider can be injected into Service to orchestrate database transactions.
type TransactionProvider interface {
	RunInTx(ctx context.Context, fn func(ctx context.Context) error) error
}

// TransactionRunner defines the contract for executing cross-domain transactions.
type TransactionRunner interface {
	ExecuteTransaction(ctx context.Context, req economy.TransactionRequest, fn economy.TransactionCallback) (*economy.TransactionResult, error)
}

// GamePlayedHook is called whenever a casino game round concludes.
type GamePlayedHook func(ctx context.Context, characterID string, gameName string) error

type Service struct {
	repo           Repository
	roomRepo       RoomRepository
	depotRepo      DepotRepository
	charRepo       CharacterRepository
	invRepo        InventoryRepository
	txProvider     TransactionProvider
	runner         TransactionRunner
	gamePlayedHook GamePlayedHook
}

type Option func(*Service)

func WithTransactionProvider(tx TransactionProvider) Option {
	return func(s *Service) {
		s.txProvider = tx
	}
}

func WithTransactionRunner(runner TransactionRunner) Option {
	return func(s *Service) {
		s.runner = runner
	}
}

func WithEconomy(eco *economy.Service) Option {
	return func(s *Service) {
		s.runner = eco
	}
}

func WithDepotRepository(depotRepo DepotRepository) Option {
	return func(s *Service) {
		s.depotRepo = depotRepo
	}
}

func WithCharacterRepository(charRepo CharacterRepository) Option {
	return func(s *Service) {
		s.charRepo = charRepo
	}
}

func WithInventoryRepository(invRepo InventoryRepository) Option {
	return func(s *Service) {
		s.invRepo = invRepo
	}
}

func WithRoomRepository(roomRepo RoomRepository) Option {
	return func(s *Service) {
		s.roomRepo = roomRepo
	}
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
	if s.runner == nil && s.charRepo != nil {
		var ecoOpts []economy.Option
		if s.txProvider != nil {
			ecoOpts = append(ecoOpts, economy.WithTransactionProvider(s.txProvider))
		}
		invRepo := s.invRepo
		if invRepo == nil {
			invRepo = noopInventoryRepo{}
		}
		eco, err := economy.NewService(s.charRepo, invRepo, ecoOpts...)
		if err != nil {
			return nil, err
		}
		s.runner = eco
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
	if s.runner == nil {
		return Account{}, corecharacter.Character{}, errors.New("transaction runner is required for coin exchange")
	}
	goldCost := int(coins * GoldPerCoin)

	req := economy.TransactionRequest{
		CharacterID: characterID,
		Cost: economy.ResourceCost{
			Gold: goldCost,
		},
	}
	var acc Account
	res, err := s.runner.ExecuteTransaction(ctx, req, func(tc *economy.TxContext) error {
		var err error
		acc, err = s.repo.AdjustCoins(tc.Context, characterID, coins)
		return err
	})
	if err != nil {
		if errors.Is(err, economy.ErrInsufficientGold) {
			return Account{}, corecharacter.Character{}, ErrInsufficientGold
		}
		if errors.Is(err, economy.ErrCharacterNotFound) {
			return Account{}, corecharacter.Character{}, corecharacter.ErrNotFound
		}
		return Account{}, corecharacter.Character{}, err
	}
	return acc, res.Character, nil
}

// ExchangeCoinsToGold sells casino coins back for character gold (1 coin = 20 gold).
func (s *Service) ExchangeCoinsToGold(ctx context.Context, characterID string, coins int64) (Account, corecharacter.Character, error) {
	if characterID == "" {
		return Account{}, corecharacter.Character{}, ErrInvalidCharacterID
	}
	if coins <= 0 {
		return Account{}, corecharacter.Character{}, ErrInvalidAmount
	}
	if s.runner == nil {
		return Account{}, corecharacter.Character{}, errors.New("transaction runner is required for coin exchange")
	}
	goldReward := int(coins * GoldPerCoin)

	req := economy.TransactionRequest{
		CharacterID: characterID,
		Grant: economy.ResourceGrant{
			Gold: goldReward,
		},
	}
	var acc Account
	res, err := s.runner.ExecuteTransaction(ctx, req, func(tc *economy.TxContext) error {
		var err error
		acc, err = s.repo.DeductBetAndCreditPayout(tc.Context, characterID, coins, 0)
		return err
	})
	if err != nil {
		if errors.Is(err, economy.ErrCharacterNotFound) {
			return Account{}, corecharacter.Character{}, corecharacter.ErrNotFound
		}
		return Account{}, corecharacter.Character{}, err
	}
	return acc, res.Character, nil
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
