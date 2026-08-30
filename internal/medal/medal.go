package medal

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"os"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	"github.com/witchcraze/party2re/internal/core/item"
)

//go:embed medal_rewards.json
var defaultMedalRewardsData []byte

// InitialRewards returns the default embedded small medal reward tiers.
func InitialRewards() ([]Reward, error) {
	var rewards []Reward
	if err := json.Unmarshal(defaultMedalRewardsData, &rewards); err != nil {
		return nil, err
	}
	return rewards, nil
}

var (
	ErrNilDependency      = errors.New("medal dependency is nil")
	ErrInsufficientMedals = errors.New("insufficient small medals")
	ErrRewardNotFound     = errors.New("reward tier not found")
)

type Reward struct {
	Cost   int    `json:"cost"`
	ItemID string `json:"item_id"`
}

type CharacterRepository interface {
	FindByID(ctx context.Context, id string) (corecharacter.Character, error)
	FindByIDForUpdate(ctx context.Context, id string) (corecharacter.Character, error)
	Update(ctx context.Context, value corecharacter.Character) error
}

type InventoryRepository interface {
	FindByCharacterID(ctx context.Context, characterID string) (coreinventory.Inventory, error)
	FindByCharacterIDForUpdate(ctx context.Context, characterID string) (coreinventory.Inventory, error)
	Save(ctx context.Context, value coreinventory.Inventory) error
}

type TransactionRepository interface {
	CommitTransaction(ctx context.Context, character corecharacter.Character, inventory coreinventory.Inventory) error
}

type TransactionProvider interface {
	RunInTx(ctx context.Context, fn func(ctx context.Context) error) error
}

type Option func(*Service)

func WithTransactionProvider(txProvider TransactionProvider) Option {
	return func(s *Service) {
		s.txProvider = txProvider
	}
}

type Service struct {
	characters   CharacterRepository
	inventories  InventoryRepository
	transactions TransactionRepository
	txProvider   TransactionProvider
	rewards      []Reward
}

func NewService(
	characters CharacterRepository,
	inventories InventoryRepository,
	transactions TransactionRepository,
	rewardsFilePath string,
	opts ...Option,
) (*Service, error) {
	if characters == nil || inventories == nil {
		return nil, ErrNilDependency
	}

	var data []byte
	if rewardsFilePath == "" {
		data = defaultMedalRewardsData
	} else {
		var err error
		data, err = os.ReadFile(rewardsFilePath)
		if err != nil {
			return nil, err
		}
	}

	var rewards []Reward
	if err := json.Unmarshal(data, &rewards); err != nil {
		return nil, err
	}

	return NewServiceWithRewards(characters, inventories, transactions, rewards, opts...)
}

func NewServiceWithRewards(
	characters CharacterRepository,
	inventories InventoryRepository,
	transactions TransactionRepository,
	rewards []Reward,
	opts ...Option,
) (*Service, error) {
	if characters == nil || inventories == nil {
		return nil, ErrNilDependency
	}

	s := &Service{
		characters:   characters,
		inventories:  inventories,
		transactions: transactions,
		rewards:      rewards,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s, nil
}

func (s *Service) GetRewards() []Reward {
	return s.rewards
}

func (s *Service) runInTx(ctx context.Context, fn func(ctx context.Context) error) error {
	if s.txProvider != nil {
		return s.txProvider.RunInTx(ctx, fn)
	}
	return fn(ctx)
}

func (s *Service) findCharacter(ctx context.Context, characterID string) (corecharacter.Character, error) {
	if s.txProvider != nil {
		return s.characters.FindByIDForUpdate(ctx, characterID)
	}
	return s.characters.FindByID(ctx, characterID)
}

func (s *Service) findInventory(ctx context.Context, characterID string) (coreinventory.Inventory, error) {
	if s.txProvider != nil {
		return s.inventories.FindByCharacterIDForUpdate(ctx, characterID)
	}
	return s.inventories.FindByCharacterID(ctx, characterID)
}

func (s *Service) Claim(ctx context.Context, characterID string, itemID string) (corecharacter.Character, coreinventory.Inventory, error) {
	if characterID == "" {
		return corecharacter.Character{}, coreinventory.Inventory{}, corecharacter.ErrNotFound
	}
	if itemID == "" {
		return corecharacter.Character{}, coreinventory.Inventory{}, ErrRewardNotFound
	}

	var targetReward *Reward
	for _, r := range s.rewards {
		if r.ItemID == itemID {
			targetReward = &r
			break
		}
	}
	if targetReward == nil {
		return corecharacter.Character{}, coreinventory.Inventory{}, ErrRewardNotFound
	}

	var updatedChar corecharacter.Character
	var updatedInv coreinventory.Inventory

	err := s.runInTx(ctx, func(txCtx context.Context) error {
		char, err := s.findCharacter(txCtx, characterID)
		if err != nil {
			return err
		}

		if char.SmallMedals < targetReward.Cost {
			return ErrInsufficientMedals
		}

		inv, err := s.findInventory(txCtx, characterID)
		if err != nil {
			return err
		}

		instance, err := item.NewInstance(targetReward.ItemID, 1)
		if err != nil {
			return err
		}

		if err := inv.Add(instance); err != nil {
			return err
		}

		char.SmallMedals -= targetReward.Cost

		if err := s.commit(txCtx, char, inv); err != nil {
			return err
		}

		updatedChar = char
		updatedInv = inv
		return nil
	})
	if err != nil {
		return corecharacter.Character{}, coreinventory.Inventory{}, err
	}

	return updatedChar, updatedInv, nil
}

func (s *Service) commit(ctx context.Context, char corecharacter.Character, inv coreinventory.Inventory) error {
	if s.transactions != nil && s.txProvider == nil {
		return s.transactions.CommitTransaction(ctx, char, inv)
	}
	if err := s.characters.Update(ctx, char); err != nil {
		return err
	}
	return s.inventories.Save(ctx, inv)
}
