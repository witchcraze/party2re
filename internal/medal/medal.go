package medal

import (
	"context"
	"encoding/json"
	"errors"
	"os"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	"github.com/witchcraze/party2re/internal/core/item"
)

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
	Update(ctx context.Context, value corecharacter.Character) error
}

type InventoryRepository interface {
	FindByCharacterID(ctx context.Context, characterID string) (coreinventory.Inventory, error)
	Save(ctx context.Context, value coreinventory.Inventory) error
}

type TransactionRepository interface {
	CommitTransaction(ctx context.Context, character corecharacter.Character, inventory coreinventory.Inventory) error
}

type Service struct {
	characters   CharacterRepository
	inventories  InventoryRepository
	transactions TransactionRepository
	rewards      []Reward
}

func NewService(
	characters CharacterRepository,
	inventories InventoryRepository,
	transactions TransactionRepository,
	rewardsFilePath string,
) (*Service, error) {
	if characters == nil || inventories == nil {
		return nil, ErrNilDependency
	}

	data, err := os.ReadFile(rewardsFilePath)
	if err != nil {
		return nil, err
	}

	var rewards []Reward
	if err := json.Unmarshal(data, &rewards); err != nil {
		return nil, err
	}

	return NewServiceWithRewards(characters, inventories, transactions, rewards)
}

func NewServiceWithRewards(
	characters CharacterRepository,
	inventories InventoryRepository,
	transactions TransactionRepository,
	rewards []Reward,
) (*Service, error) {
	if characters == nil || inventories == nil {
		return nil, ErrNilDependency
	}

	return &Service{
		characters:   characters,
		inventories:  inventories,
		transactions: transactions,
		rewards:      rewards,
	}, nil
}

func (s *Service) GetRewards() []Reward {
	return s.rewards
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

	char, err := s.characters.FindByID(ctx, characterID)
	if err != nil {
		return corecharacter.Character{}, coreinventory.Inventory{}, err
	}

	if char.SmallMedals < targetReward.Cost {
		return corecharacter.Character{}, coreinventory.Inventory{}, ErrInsufficientMedals
	}

	inv, err := s.inventories.FindByCharacterID(ctx, characterID)
	if err != nil {
		return corecharacter.Character{}, coreinventory.Inventory{}, err
	}

	instance, err := item.NewInstance(targetReward.ItemID, 1)
	if err != nil {
		return corecharacter.Character{}, coreinventory.Inventory{}, err
	}

	if err := inv.Add(instance); err != nil {
		return corecharacter.Character{}, coreinventory.Inventory{}, err
	}

	char.SmallMedals -= targetReward.Cost

	if err := s.commit(ctx, char, inv); err != nil {
		return corecharacter.Character{}, coreinventory.Inventory{}, err
	}

	return char, inv, nil
}

func (s *Service) commit(ctx context.Context, char corecharacter.Character, inv coreinventory.Inventory) error {
	if s.transactions != nil {
		return s.transactions.CommitTransaction(ctx, char, inv)
	}
	if err := s.characters.Update(ctx, char); err != nil {
		return err
	}
	return s.inventories.Save(ctx, inv)
}
