package adventure

import (
	"context"
	"errors"
	"fmt"
	"time"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/runhost"
)

const StarterAdventure = "stage-00"

var (
	ErrNotFound             = errors.New("adventure not found")
	ErrCannotUseInCombat    = item.ErrCannotUseInCombat
	ErrCharacterUnconscious = runhost.ErrCharacterUnconscious
	ErrCharacterExhausted   = runhost.ErrCharacterExhausted
	ErrDailyOnceAlreadyUsed = errors.New("character has already challenged the once-daily dungeon today")
	ErrInvalidQuestName     = errors.New("quest name must be 1 to 50 characters without whitespace or prohibited characters")
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
	ID                 string
	CharacterID        string
	Type               string
	StageID            string
	Season             string
	SeasonalSubID      int
	MonsterID          string
	StartedAt          time.Time
	FloorsCleared      int
	IsCleared          bool
	PartySize          int
	ExperienceReward   int
	BattleResult       corebattle.Result
	RewardItemID       string `json:"-"` // Retained for previously stored adventure history.
	RewardItemQuantity int    `json:"-"`
	Resolved           bool
}

type Repository interface {
	Save(ctx context.Context, value Adventure) error
	FindByID(ctx context.Context, id string) (Adventure, error)
	ListByCharacterID(ctx context.Context, characterID string, limit, offset int) ([]Adventure, int, error)
	ListByCharacterIDByCursor(ctx context.Context, characterID string, limit int, beforeTime time.Time, beforeID string) ([]Adventure, error)
	GetAggregatedStats(ctx context.Context, characterID string) (AggregatedStats, error)
}

type CharacterRepository interface {
	FindByID(ctx context.Context, id string) (corecharacter.Character, error)
}

// Service reads adventure history and chronicle records.
type Service struct {
	adventures Repository
	characters CharacterRepository
	stages     *StageCatalog
	monsters   *MonsterCatalog
}

// NewService creates a Service with the built-in stage and monster catalogs.
func NewService(adventures Repository, characters CharacterRepository) (*Service, error) {
	stages, err := InitialStageCatalog()
	if err != nil {
		return nil, err
	}
	monsters, err := InitialMonsterCatalog()
	if err != nil {
		return nil, err
	}
	return NewServiceWithCatalogs(adventures, characters, stages, monsters)
}

func NewServiceWithCatalogs(adventures Repository, characters CharacterRepository, stages *StageCatalog, monsters *MonsterCatalog) (*Service, error) {
	if adventures == nil || characters == nil {
		return nil, errors.New("adventure dependencies are nil")
	}
	return &Service{adventures: adventures, characters: characters, stages: stages, monsters: monsters}, nil
}

// Get retrieves an adventure record by its ID.
func (s *Service) Get(ctx context.Context, id string) (Adventure, error) {
	return s.adventures.FindByID(ctx, id)
}
