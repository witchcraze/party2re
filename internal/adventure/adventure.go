package adventure

import (
	"context"
	"errors"
	"fmt"
	"time"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	"github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/core/progression"
	"github.com/witchcraze/party2re/internal/id"
)

const (
	StarterAdventure            = "stage-01"
	AdventureDuration           = time.Hour
	AdventureReward             = 20
	AdventureEnemyID            = "starter-opponent"
	AdventureActionTypeComplete = "adventure:complete"
)

var (
	ErrNotFound               = errors.New("adventure not found")
	ErrNotReady               = errors.New("adventure is not ready")
	ErrAlreadyClaimed         = errors.New("adventure result already claimed")
	ErrUnsupportedReward      = errors.New("adventure reward type is unsupported")
	ErrLevelRequirementNotMet = errors.New("character level requirement not met for stage")
)

type Adventure struct {
	ID               string
	CharacterID      string
	Type             string
	StageID          string
	MonsterID        string
	StartedAt        time.Time
	AvailableAt      time.Time
	ExperienceReward int
	BattleResult     corebattle.Result
	Resolved         bool
	Claimed          bool
}

type Clock interface {
	Now() time.Time
}

type RealClock struct{}

func (RealClock) Now() time.Time { return time.Now().UTC() }

type Repository interface {
	Save(ctx context.Context, value Adventure) error
	FindByID(ctx context.Context, id string) (Adventure, error)
	ClaimAndApply(ctx context.Context, value Adventure, character corecharacter.Character) error
	ListByCharacterID(ctx context.Context, characterID string, limit, offset int) ([]Adventure, int, error)
	ListByCharacterIDByCursor(ctx context.Context, characterID string, limit int, beforeTime time.Time, beforeID string) ([]Adventure, error)
	GetAggregatedStats(ctx context.Context, characterID string) (AggregatedStats, error)
}

type InventoryRepository interface {
	FindByCharacterID(ctx context.Context, characterID string) (coreinventory.Inventory, error)
	Save(ctx context.Context, value coreinventory.Inventory) error
}

type CharacterRepository interface {
	FindByID(ctx context.Context, id string) (corecharacter.Character, error)
}

type Scheduler interface {
	Schedule(ctx context.Context, actionType, actorID string, params map[string]string, executeAt time.Time) (string, error)
}

type Logger interface {
	Warn(msg string, args ...any)
}

type nopLogger struct{}

func (nopLogger) Warn(msg string, args ...any) {}

// VictoryHook is called when an adventure stage concludes with a player victory.
type VictoryHook func(ctx context.Context, characterID string, monstersDefeated int, goldEarned int) error

type Service struct {
	adventures  Repository
	characters  CharacterRepository
	inventories InventoryRepository
	stages      *StageCatalog
	monsters    *MonsterCatalog
	battle      corebattle.Resolver
	scheduler   Scheduler
	logger      Logger
	clock       Clock
	victoryHook VictoryHook
}

func (s *Service) SetVictoryHook(hook VictoryHook) {
	s.victoryHook = hook
}

func NewService(adventures Repository, characters CharacterRepository, battle corebattle.Resolver, scheduler Scheduler, logger Logger) (*Service, error) {
	stages, err := InitialStageCatalog()
	if err != nil {
		return nil, err
	}
	monsters, err := InitialMonsterCatalog()
	if err != nil {
		return nil, err
	}
	return NewServiceWithCatalogs(adventures, characters, nil, stages, monsters, battle, scheduler, logger, RealClock{})
}

func NewServiceWithClock(adventures Repository, characters CharacterRepository, battle corebattle.Resolver, scheduler Scheduler, logger Logger, clock Clock) (*Service, error) {
	if clock == nil {
		return nil, errors.New("adventure clock is nil")
	}
	stages, err := InitialStageCatalog()
	if err != nil {
		return nil, err
	}
	monsters, err := InitialMonsterCatalog()
	if err != nil {
		return nil, err
	}
	return NewServiceWithCatalogs(adventures, characters, nil, stages, monsters, battle, scheduler, logger, clock)
}

func NewServiceWithCatalogs(
	adventures Repository,
	characters CharacterRepository,
	inventories InventoryRepository,
	stages *StageCatalog,
	monsters *MonsterCatalog,
	battle corebattle.Resolver,
	scheduler Scheduler,
	logger Logger,
	clock Clock,
) (*Service, error) {
	if adventures == nil || characters == nil || battle == nil {
		return nil, errors.New("adventure dependencies are nil")
	}
	if clock == nil {
		return nil, errors.New("adventure clock is nil")
	}
	if logger == nil {
		logger = nopLogger{}
	}
	return &Service{
		adventures:  adventures,
		characters:  characters,
		inventories: inventories,
		stages:      stages,
		monsters:    monsters,
		battle:      battle,
		scheduler:   scheduler,
		logger:      logger,
		clock:       clock,
	}, nil
}

func (s *Service) Start(ctx context.Context, characterID string) (Adventure, error) {
	return s.StartStage(ctx, characterID, "stage-01")
}

func (s *Service) StartStage(ctx context.Context, characterID string, stageID string) (Adventure, error) {
	if characterID == "" {
		return Adventure{}, corecharacter.ErrNotFound
	}
	character, err := s.characters.FindByID(ctx, characterID)
	if err != nil {
		return Adventure{}, err
	}

	if stageID == "" {
		stageID = "stage-01"
	}

	var stage Stage
	var monster Monster
	duration := AdventureDuration
	expReward := AdventureReward

	if s.stages != nil {
		st, err := s.stages.FindByID(stageID)
		if err != nil {
			return Adventure{}, ErrStageNotFound
		}
		if character.Level < st.MinLevel {
			return Adventure{}, ErrLevelRequirementNotMet
		}
		stage = st
		duration = st.Duration

		if len(st.MonsterIDs) > 0 && s.monsters != nil {
			chosenMonsterID := st.MonsterIDs[0]
			m, err := s.monsters.FindByID(chosenMonsterID)
			if err != nil {
				return Adventure{}, ErrMonsterNotFound
			}
			monster = m
			expReward = m.ExperienceReward
		}
	}

	now := s.clock.Now()
	value := Adventure{
		ID:               id.New(),
		CharacterID:      characterID,
		Type:             stageID,
		StageID:          stage.ID,
		MonsterID:        monster.ID,
		StartedAt:        now,
		AvailableAt:      now.Add(duration),
		ExperienceReward: expReward,
	}
	if err := s.adventures.Save(ctx, value); err != nil {
		return Adventure{}, err
	}

	if s.scheduler != nil {
		if _, err := s.scheduler.Schedule(
			ctx,
			AdventureActionTypeComplete,
			characterID,
			map[string]string{
				"adventure_id": value.ID,
				"stage_id":     stage.ID,
				"monster_id":   monster.ID,
			},
			value.AvailableAt,
		); err != nil {
			s.logger.Warn("failed to schedule adventure completion", "adventure_id", value.ID, "error", err)
		}
	}

	return value, nil
}

func (s *Service) Get(ctx context.Context, id string) (Adventure, error) {
	return s.adventures.FindByID(ctx, id)
}

func (s *Service) Claim(ctx context.Context, id string) (Adventure, error) {
	value, err := s.adventures.FindByID(ctx, id)
	if err != nil {
		return Adventure{}, err
	}
	if value.Claimed {
		return Adventure{}, ErrAlreadyClaimed
	}
	if s.clock.Now().Before(value.AvailableAt) {
		return Adventure{}, ErrNotReady
	}
	character, err := s.characters.FindByID(ctx, value.CharacterID)
	if err != nil {
		return Adventure{}, err
	}

	// Prepare combat participants and reward
	enemyID := AdventureEnemyID
	enemyHP := 8
	enemyAttack := 1
	enemyDefense := 0
	rewardExp := value.ExperienceReward
	rewardGold := 0
	rewardItemID := ""

	if value.MonsterID != "" && s.monsters != nil {
		if m, err := s.monsters.FindByID(value.MonsterID); err == nil {
			enemyID = m.ID
			enemyHP = m.HP
			enemyAttack = m.Attack
			enemyDefense = m.Defense
			rewardExp = m.ExperienceReward
			rewardGold = m.GoldReward
			if len(m.DropItemIDs) > 0 {
				rewardItemID = m.DropItemIDs[0]
			}
		}
	}

	itemQuantity := 0
	if rewardItemID != "" {
		itemQuantity = 1
	}

	req := corebattle.Request{
		Participants: []corebattle.Participant{
			corebattle.NewParticipantFromCharacter(character),
			corebattle.MustNewParticipant(enemyID, enemyHP, enemyAttack, enemyDefense),
		},
		VictoryReward: corebattle.Reward{
			Experience:       rewardExp,
			Currency:         rewardGold,
			ItemDefinitionID: rewardItemID,
			ItemQuantity:     itemQuantity,
		},
	}

	result, err := s.battle.Resolve(req)
	if err != nil {
		return Adventure{}, fmt.Errorf("resolve adventure battle: %w", err)
	}
	value.BattleResult = result
	value.Resolved = true

	if result.Outcome == corebattle.OutcomeWin {
		if result.Reward.Experience > 0 {
			if _, err := progression.ApplyExperience(&character, result.Reward.Experience); err != nil {
				return Adventure{}, fmt.Errorf("apply adventure reward: %w", err)
			}
		}
		if result.Reward.Currency > 0 {
			_ = character.AddMoney(result.Reward.Currency)
		}
		if result.Reward.SmallMedals > 0 {
			_ = character.AddSmallMedals(result.Reward.SmallMedals)
		}
		if result.Reward.ItemDefinitionID != "" {
			if s.inventories == nil {
				return Adventure{}, ErrUnsupportedReward
			}
			inv, err := s.inventories.FindByCharacterID(ctx, character.ID)
			if err == nil {
				inst, err := item.NewInstance(result.Reward.ItemDefinitionID, result.Reward.ItemQuantity)
				if err == nil {
					_ = inv.Add(inst)
					_ = s.inventories.Save(ctx, inv)
				}
			}
		}
	}

	value.Claimed = true
	if err := s.adventures.ClaimAndApply(ctx, value, character); err != nil {
		return Adventure{}, err
	}

	if result.Outcome == corebattle.OutcomeWin && s.victoryHook != nil {
		if err := s.victoryHook(ctx, character.ID, 1, result.Reward.Currency); err != nil {
			s.logger.Warn("victory hook failed", "character_id", character.ID, "error", err)
		}
	}
	return value, nil
}
