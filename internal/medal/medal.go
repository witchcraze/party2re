package medal

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"os"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/depot"
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

type DepotRepository interface {
	FindByCharacterIDForUpdate(ctx context.Context, characterID string) (depot.Depot, error)
	Save(ctx context.Context, dep depot.Depot) error
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
	depots          DepotRepository
	txProvider      TransactionProvider
	rewards         []Reward
	achievements    []Achievement
	achievementRepo AchievementRepository
}

func NewService(
	characters CharacterRepository,
	depots DepotRepository,
	rewardsFilePath string,
	opts ...Option,
) (*Service, error) {
	if characters == nil || depots == nil {
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

	return NewServiceWithRewards(characters, depots, rewards, opts...)
}

func NewServiceWithRewards(
	characters CharacterRepository,
	depots DepotRepository,
	rewards []Reward,
	opts ...Option,
) (*Service, error) {
	if characters == nil || depots == nil {
		return nil, ErrNilDependency
	}

	s := &Service{
		characters: characters,
		depots:     depots,
		rewards:    rewards,
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

func (s *Service) Claim(ctx context.Context, characterID string, itemID string) (corecharacter.Character, depot.Depot, error) {
	if characterID == "" {
		return corecharacter.Character{}, depot.Depot{}, corecharacter.ErrNotFound
	}
	if itemID == "" {
		return corecharacter.Character{}, depot.Depot{}, ErrRewardNotFound
	}

	var targetReward *Reward
	for _, r := range s.rewards {
		if r.ItemID == itemID {
			targetReward = &r
			break
		}
	}
	if targetReward == nil {
		return corecharacter.Character{}, depot.Depot{}, ErrRewardNotFound
	}

	var updatedChar corecharacter.Character
	var updatedDepot depot.Depot

	err := s.runInTx(ctx, func(txCtx context.Context) error {
		char, err := s.characters.FindByIDForUpdate(txCtx, characterID)
		if err != nil {
			return err
		}

		if char.SmallMedals < targetReward.Cost {
			return ErrInsufficientMedals
		}

		dep, err := s.depots.FindByCharacterIDForUpdate(txCtx, characterID)
		if errors.Is(err, depot.ErrNotFound) {
			dep, err = depot.NewDepotWithCapacity(char.ID, 0, 0, char.OverDepot)
			if err != nil {
				return err
			}
		} else if err != nil {
			return err
		}

		inst, err := coreitem.NewInstance(targetReward.ItemID, 1)
		if err != nil {
			return err
		}
		if err := dep.AddItem(inst); err != nil {
			return err
		}

		if err := char.DeductSmallMedals(targetReward.Cost); err != nil {
			return err
		}

		if err := s.characters.Update(txCtx, char); err != nil {
			return err
		}
		if err := s.depots.Save(txCtx, dep); err != nil {
			return err
		}

		updatedChar = char
		updatedDepot = dep
		return nil
	})
	if err != nil {
		return corecharacter.Character{}, depot.Depot{}, err
	}

	return updatedChar, updatedDepot, nil
}
