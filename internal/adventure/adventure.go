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
	StarterAdventure = "stage-01"
	AdventureReward  = 20
	AdventureEnemyID = "starter-opponent"
)

var (
	ErrNotFound             = errors.New("adventure not found")
	ErrUnsupportedReward    = errors.New("adventure reward type is unsupported")
	ErrCannotUseInCombat    = item.ErrCannotUseInCombat
	ErrCharacterUnconscious = errors.New("character is unconscious (HP <= 0) and cannot adventure")
	ErrCharacterExhausted   = errors.New("character is exhausted (tired >= 100) and must rest")
)

// ValidateCombatItem validates that an item is allowed for use in combat actions.
func ValidateCombatItem(def item.Definition) error {
	if !def.CanUseInCombat() {
		return fmt.Errorf("%w: %sは戦闘中では使えません", ErrCannotUseInCombat, def.Name)
	}
	return nil
}

// Adventure represents a completed or active dungeon crawl record.
// Fictional 1-hour expedition timer fields (AvailableAt, Claimed) are purged per Issue #478.
type Adventure struct {
	ID               string
	CharacterID      string
	Type             string
	StageID          string
	MonsterID        string
	StartedAt        time.Time
	FloorsCleared    int
	IsCleared        bool
	PartySize        int
	ExperienceReward int
	BattleResult     corebattle.Result
	Resolved         bool
}

type Clock interface {
	Now() time.Time
}

type RealClock struct{}

func (RealClock) Now() time.Time { return time.Now().UTC() }

type Repository interface {
	Save(ctx context.Context, value Adventure) error
	FindByID(ctx context.Context, id string) (Adventure, error)
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

type CharacterUpdater interface {
	Update(ctx context.Context, value corecharacter.Character) error
}

type Logger interface {
	Warn(msg string, args ...any)
}

type nopLogger struct{}

func (nopLogger) Warn(msg string, args ...any) {}

// VictoryHook is called when an adventure stage concludes with a player victory.
type VictoryHook func(ctx context.Context, characterID string, monstersDefeated int, goldEarned int) error

// PostAdventureHook is called after an adventure concludes and state is applied.
type PostAdventureHook func(ctx context.Context, characterID string) error

type partyBattleResolver interface {
	ResolvePartyBattle(req corebattle.PartyBattleRequest) (corebattle.PartyBattleResult, error)
}

type Service struct {
	adventures        Repository
	characters        CharacterRepository
	inventories       InventoryRepository
	stages            *StageCatalog
	monsters          *MonsterCatalog
	battle            corebattle.Resolver
	logger            Logger
	clock             Clock
	victoryHook       VictoryHook
	postAdventureHook PostAdventureHook
}

func (s *Service) SetVictoryHook(hook VictoryHook) {
	s.victoryHook = hook
}

func (s *Service) SetPostAdventureHook(hook PostAdventureHook) {
	s.postAdventureHook = hook
}

func NewService(adventures Repository, characters CharacterRepository, battle corebattle.Resolver, scheduler any, logger Logger) (*Service, error) {
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

func NewServiceWithClock(adventures Repository, characters CharacterRepository, battle corebattle.Resolver, scheduler any, logger Logger, clock Clock) (*Service, error) {
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
	_ any, // scheduler unused per Issue #478 (timer purged)
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
		logger:      logger,
		clock:       clock,
	}, nil
}

// Start executes a default StarterAdventure crawl.
func (s *Service) Start(ctx context.Context, characterID string) (Adventure, error) {
	return s.StartStage(ctx, characterID, StarterAdventure)
}

// StartStage executes the authentic 10-floor dungeon crawl for a character, immediately resolving it.
// The fictional 1-hour expedition timer is completely abolished.
func (s *Service) StartStage(ctx context.Context, characterID string, stageID string) (Adventure, error) {
	if characterID == "" {
		return Adventure{}, corecharacter.ErrNotFound
	}
	char, err := s.characters.FindByID(ctx, characterID)
	if err != nil {
		return Adventure{}, err
	}

	if stageID == "" {
		stageID = StarterAdventure
	}

	if s.stages != nil {
		if err := s.stages.CanAccessStage(char, stageID); err != nil {
			return Adventure{}, err
		}
	}

	crawlRes, err := s.ExecuteCrawl(ctx, DungeonCrawlRequest{
		CharacterIDs: []string{characterID},
		StageID:      stageID,
	})
	if err != nil {
		return Adventure{}, err
	}

	advID := crawlRes.AdventureIDs[characterID]
	return s.adventures.FindByID(ctx, advID)
}

// ExecuteCrawl runs the authentic 10-floor dungeon crawl and Floor 11 treasure room (vs_monster.cgi).
func (s *Service) ExecuteCrawl(ctx context.Context, req DungeonCrawlRequest) (DungeonCrawlResult, error) {
	if len(req.CharacterIDs) == 0 {
		return DungeonCrawlResult{}, ErrNoParticipants
	}
	if len(req.CharacterIDs) > 4 {
		return DungeonCrawlResult{}, ErrTooManyParticipants
	}

	characters := make([]corecharacter.Character, 0, len(req.CharacterIDs))
	for _, cID := range req.CharacterIDs {
		c, err := s.characters.FindByID(ctx, cID)
		if err != nil {
			return DungeonCrawlResult{}, err
		}
		if c.Stats.HP <= 0 {
			return DungeonCrawlResult{}, ErrCharacterUnconscious
		}
		if c.Tired >= 100 {
			return DungeonCrawlResult{}, ErrCharacterExhausted
		}
		characters = append(characters, c)
	}

	stageID := req.StageID
	if stageID == "" {
		stageID = StarterAdventure
	}
	stage, err := s.stages.FindByID(stageID)
	if err != nil {
		return DungeonCrawlResult{}, ErrStageNotFound
	}

	if err := s.stages.CanAccessStage(characters[0], stage.ID); err != nil {
		return DungeonCrawlResult{}, err
	}

	var pbr corebattle.PartyBattleResolver = corebattle.Engine{}
	if r, ok := s.battle.(partyBattleResolver); ok {
		pbr = r
	}

	session, err := NewCrawlSession(stage, characters, req.Rng)
	if err != nil {
		return DungeonCrawlResult{}, err
	}

	// Advance through all 10 floors
	for floor := 1; floor <= BossFloor; floor++ {
		floorRes, err := session.AdvanceFloor(s.stages, s.monsters, pbr)
		if err != nil {
			return DungeonCrawlResult{}, err
		}
		if !floorRes.Cleared {
			break
		}
	}

	// If boss was defeated, advance to Floor 11 (Treasure Room) and open boxes
	if session.StageCleared {
		_, _ = session.AdvanceFloor(s.stages, s.monsters, pbr)
		for _, c := range characters {
			_, _ = session.ExamineTreasure(c.ID)
		}
	}

	result := session.Result()

	// Persist adventure record for each participating character
	now := s.clock.Now()
	for _, c := range characters {
		advID := id.New()
		result.AdventureIDs[c.ID] = advID

		adv := Adventure{
			ID:               advID,
			CharacterID:      c.ID,
			Type:             stage.ID,
			StageID:          stage.ID,
			StartedAt:        now,
			FloorsCleared:    result.FloorsCleared,
			IsCleared:        result.StageCleared,
			PartySize:        len(characters),
			ExperienceReward: result.TotalEXP,
			Resolved:         true,
			BattleResult: corebattle.Result{
				Outcome:  result.Outcome,
				WinnerID: characters[0].ID,
				Turns:    result.TotalTurns,
				Reward: corebattle.Reward{
					Experience: result.TotalEXP,
					Currency:   result.TotalGold,
				},
			},
		}

		if s.adventures != nil {
			_ = s.adventures.Save(ctx, adv)
		}

		// Apply experience and gold rewards
		if result.Outcome == corebattle.OutcomeWin {
			if result.TotalEXP > 0 {
				_, _ = progression.ApplyExperience(&c, result.TotalEXP)
			}
			if result.TotalGold > 0 {
				_ = c.AddMoney(result.TotalGold)
			}
			if updater, ok := s.characters.(CharacterUpdater); ok {
				_ = updater.Update(ctx, c)
			}

			if s.victoryHook != nil {
				_ = s.victoryHook(ctx, c.ID, result.FloorsCleared, result.TotalGold)
			}
		}

		if s.postAdventureHook != nil {
			_ = s.postAdventureHook(ctx, c.ID)
		}
	}

	return result, nil
}

// Get retrieves an adventure record by its ID.
func (s *Service) Get(ctx context.Context, id string) (Adventure, error) {
	return s.adventures.FindByID(ctx, id)
}
