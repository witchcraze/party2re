package medal

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"os"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	"github.com/witchcraze/party2re/internal/economy"
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

type AchievementRepository interface {
	RecordProgress(ctx context.Context, characterID string, metric MetricType, amount int, matchingAchievements []Achievement) error
	GetCharacterAchievements(ctx context.Context, characterID string) ([]AchievementRecord, error)
	GetAchievementForUpdate(ctx context.Context, characterID string, achievementID string) (AchievementRecord, error)
	MarkAchievementClaimed(ctx context.Context, characterID string, achievementID string, claimedAt time.Time) error
	SaveMedal(ctx context.Context, medal CharacterMedal) error
	GetCharacterMedals(ctx context.Context, characterID string) ([]CharacterMedal, error)
}

type Option func(*Service)

func WithTransactionProvider(txProvider TransactionProvider) Option {
	return func(s *Service) {
		s.txProvider = txProvider
	}
}

func WithAchievementRepository(repo AchievementRepository, achievements ...Achievement) Option {
	return func(s *Service) {
		s.achievementRepo = repo
		if len(achievements) > 0 {
			s.achievements = achievements
		}
	}
}

func WithAchievementCatalog(achievements []Achievement) Option {
	return func(s *Service) {
		s.achievements = achievements
	}
}

type Service struct {
	characters      CharacterRepository
	inventories     InventoryRepository
	transactions    TransactionRepository
	txProvider      TransactionProvider
	rewards         []Reward
	economy         *economy.Service
	achievements    []Achievement
	achievementRepo AchievementRepository
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
	if len(s.achievements) == 0 {
		defaultAchs, err := InitialAchievements()
		if err == nil {
			s.achievements = defaultAchs
		}
	}
	var ecoOpts []economy.Option
	if s.txProvider != nil {
		ecoOpts = append(ecoOpts, economy.WithTransactionProvider(s.txProvider))
	}
	eco, err := economy.NewService(characters, inventories, ecoOpts...)
	if err != nil {
		return nil, err
	}
	s.economy = eco
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

	res, err := s.economy.Exchange(ctx, economy.ExchangeRequest{
		CharacterID:       characterID,
		DeductMedals:      targetReward.Cost,
		GrantDefinitionID: targetReward.ItemID,
		GrantQuantity:     1,
	})
	if err != nil {
		if errors.Is(err, economy.ErrInsufficientMedals) {
			return corecharacter.Character{}, coreinventory.Inventory{}, ErrInsufficientMedals
		}
		if errors.Is(err, economy.ErrCharacterNotFound) {
			return corecharacter.Character{}, coreinventory.Inventory{}, corecharacter.ErrNotFound
		}
		return corecharacter.Character{}, coreinventory.Inventory{}, err
	}

	return res.Character, res.Inventory, nil
}
