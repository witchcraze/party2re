package medal

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/depot"
)

//go:embed medal_rewards.json
var defaultMedalRewardsData []byte

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

type Service struct {
	mutationCheck func(context.Context, string) error
	characters    CharacterRepository
	depots        DepotRepository
	txProvider    TransactionProvider
	rewards       []Reward
}

// NewService serves the embedded medal.cgi prize tiers.
func NewService(characters CharacterRepository, depots DepotRepository, txProvider TransactionProvider) (*Service, error) {
	var rewards []Reward
	if err := json.Unmarshal(defaultMedalRewardsData, &rewards); err != nil {
		return nil, err
	}
	return NewServiceWithRewards(characters, depots, txProvider, rewards)
}

func NewServiceWithRewards(characters CharacterRepository, depots DepotRepository, txProvider TransactionProvider, rewards []Reward) (*Service, error) {
	if characters == nil || depots == nil || txProvider == nil {
		return nil, ErrNilDependency
	}
	return &Service{characters: characters, depots: depots, txProvider: txProvider, rewards: rewards}, nil
}

func (s *Service) GetRewards() []Reward {
	return s.rewards
}

func (s *Service) Claim(ctx context.Context, characterID string, itemID string) (corecharacter.Character, error) {
	if characterID == "" {
		return corecharacter.Character{}, corecharacter.ErrNotFound
	}
	if itemID == "" {
		return corecharacter.Character{}, ErrRewardNotFound
	}

	var targetReward *Reward
	for _, r := range s.rewards {
		if r.ItemID == itemID {
			targetReward = &r
			break
		}
	}
	if targetReward == nil {
		return corecharacter.Character{}, ErrRewardNotFound
	}

	var updatedChar corecharacter.Character
	err := s.txProvider.RunInTx(ctx, func(txCtx context.Context) error {
		char, err := s.characters.FindByIDForUpdate(txCtx, characterID)
		if err != nil {
			return err
		}
		if err := s.checkMutation(txCtx, characterID); err != nil {
			return err
		}

		if char.SmallMedals < targetReward.Cost {
			return ErrInsufficientMedals
		}

		dep, err := depot.FindOrCreate(txCtx, s.depots, char)
		if err != nil {
			return err
		}

		inst, err := coreitem.NewInstance(targetReward.ItemID, 1)
		if err != nil {
			return err
		}
		dep.Deliver(inst)

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
		return nil
	})
	if err != nil {
		return corecharacter.Character{}, err
	}

	return updatedChar, nil
}
