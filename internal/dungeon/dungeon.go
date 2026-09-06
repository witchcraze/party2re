package dungeon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
)

var (
	ErrDungeonNotFound           = errors.New("dungeon not found")
	ErrCharacterNotFound         = errors.New("character not found")
	ErrLevelRequirementNotMet    = errors.New("character level requirement not met for dungeon")
	ErrActiveExpeditionExists    = errors.New("an active expedition is already in progress")
	ErrNoActiveExpedition        = errors.New("no active dungeon expedition in progress")
	ErrInvalidDirection          = errors.New("invalid move direction")
	ErrImpassableWall            = errors.New("cannot move into wall or out of map bounds")
	ErrExpeditionAlreadyFinished = errors.New("expedition is already completed")
)

type ExpeditionStatus string

const (
	StatusExploring ExpeditionStatus = "exploring"
	StatusCleared   ExpeditionStatus = "cleared"
	StatusEscaped   ExpeditionStatus = "escaped"
	StatusWipedOut  ExpeditionStatus = "wiped_out"
)

type TileEventType string

const (
	EventMove     TileEventType = "move"
	EventBattle   TileEventType = "battle"
	EventTreasure TileEventType = "treasure"
	EventTrap     TileEventType = "trap"
	EventStairs   TileEventType = "stairs"
	EventBoss     TileEventType = "boss"
	EventEscape   TileEventType = "escape"
	EventWipeout  TileEventType = "wipeout"
)

type Direction string

const (
	DirectionNorth Direction = "north"
	DirectionSouth Direction = "south"
	DirectionEast  Direction = "east"
	DirectionWest  Direction = "west"
)

type DungeonMonster struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	HP         int    `json:"hp"`
	Attack     int    `json:"attack"`
	Defense    int    `json:"defense"`
	Agility    int    `json:"agility"`
	ExpReward  int    `json:"exp_reward"`
	GoldReward int    `json:"gold_reward"`
	DropItemID string `json:"drop_item_id,omitempty"`
}

type Floor struct {
	FloorNumber int              `json:"floor_number"`
	Width       int              `json:"width"`
	Height      int              `json:"height"`
	StartX      int              `json:"start_x"`
	StartY      int              `json:"start_y"`
	Grid        []string         `json:"grid"`
	Monsters    []DungeonMonster `json:"monsters"`
	Boss        *DungeonMonster  `json:"boss,omitempty"`
}

type Dungeon struct {
	ID               string  `json:"id"`
	Tier             int     `json:"tier"`
	Name             string  `json:"name"`
	Description      string  `json:"description"`
	MinLevel         int     `json:"min_level"`
	MaxTurnsPerFloor int     `json:"max_turns_per_floor"`
	Floors           []Floor `json:"floors"`
	ClearExpBonus    int     `json:"clear_exp_bonus"`
	ClearGoldBonus   int     `json:"clear_gold_bonus"`
}

type ActiveExpedition struct {
	ID                string           `json:"id"`
	CharacterID       string           `json:"character_id"`
	DungeonID         string           `json:"dungeon_id"`
	CurrentFloor      int              `json:"current_floor"`
	PosX              int              `json:"pos_x"`
	PosY              int              `json:"pos_y"`
	CurrentHP         int              `json:"current_hp"`
	TurnsRemaining    int              `json:"turns_remaining"`
	AccumulatedExp    int              `json:"accumulated_exp"`
	AccumulatedGold   int              `json:"accumulated_gold"`
	AccumulatedItems  []string         `json:"accumulated_items"`
	AccumulatedMedals int              `json:"accumulated_medals"`
	Status            ExpeditionStatus `json:"status"`
	StartedAt         time.Time        `json:"started_at"`
	UpdatedAt         time.Time        `json:"updated_at"`
}

type CharacterDungeonRecord struct {
	CharacterID           string    `json:"character_id"`
	HighestDungeonCleared int       `json:"highest_dungeon_cleared"`
	TotalExpeditions      int       `json:"total_expeditions"`
	TotalFloorsCleared    int       `json:"total_floors_cleared"`
	TotalChestsOpened     int       `json:"total_chests_opened"`
	TotalMonstersSlain    int       `json:"total_monsters_slain"`
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
}

type DungeonExpeditionHistory struct {
	ID               string           `json:"id"`
	CharacterID      string           `json:"character_id"`
	DungeonID        string           `json:"dungeon_id"`
	FloorsReached    int              `json:"floors_reached"`
	Outcome          ExpeditionStatus `json:"outcome"`
	ExpReward        int              `json:"exp_reward"`
	GoldReward       int              `json:"gold_reward"`
	MedalsReward     int              `json:"medals_reward"`
	ItemsRewardCount int              `json:"items_reward_count"`
	CreatedAt        time.Time        `json:"created_at"`
}

type DungeonOverview struct {
	Dungeon    Dungeon `json:"dungeon"`
	IsUnlocked bool    `json:"is_unlocked"`
	IsCleared  bool    `json:"is_cleared"`
	LockReason string  `json:"lock_reason,omitempty"`
}

type ExpeditionStepResult struct {
	Expedition   ActiveExpedition   `json:"expedition"`
	EventType    TileEventType      `json:"event_type"`
	Message      string             `json:"message"`
	BattleResult *corebattle.Result `json:"battle_result,omitempty"`
	DamageTaken  int                `json:"damage_taken,omitempty"`
	GoldFound    int                `json:"gold_found,omitempty"`
	MedalsFound  int                `json:"medals_found,omitempty"`
	ItemFound    string             `json:"item_found,omitempty"`
	ExpEarned    int                `json:"exp_earned,omitempty"`
	IsFinished   bool               `json:"is_finished"`
}

// StepParams encapsulates the parameters for advancing an active dungeon expedition step atomically.
type StepParams struct {
	ExpectedExpeditionID string
	NewFloor             int
	NewX                 int
	NewY                 int
	HPDelta              int
	TurnsDelta           int
	ExpDelta             int
	GoldDelta            int
	MedalsDelta          int
	RewardItemID         string
	Now                  time.Time
}

// StepOutcome represents the result of executing an atomic step against the active expedition store.
type StepOutcome struct {
	Expedition ActiveExpedition
	Status     ExpeditionStatus
}

// ActiveExpeditionStore defines the storage contract for transient in-flight dungeon expeditions (Candidate D).
type ActiveExpeditionStore interface {
	GetActiveExpedition(ctx context.Context, characterID string) (*ActiveExpedition, error)
	SaveActiveExpedition(ctx context.Context, exp ActiveExpedition) error
	DeleteActiveExpedition(ctx context.Context, characterID string) error
	Step(ctx context.Context, characterID string, params StepParams) (StepOutcome, error)
}

type Repository interface {
	GetRecord(ctx context.Context, characterID string) (CharacterDungeonRecord, error)
	GetActiveExpedition(ctx context.Context, characterID string) (*ActiveExpedition, error)
	SaveActiveExpedition(ctx context.Context, exp ActiveExpedition) error
	DeleteActiveExpedition(ctx context.Context, characterID string) error
	FinalizeExpedition(
		ctx context.Context,
		history DungeonExpeditionHistory,
		record CharacterDungeonRecord,
		character *corecharacter.Character,
		rewardItems []coreitem.Instance,
	) error
	GetHistory(ctx context.Context, characterID string, limit int) ([]DungeonExpeditionHistory, error)
}

type CharacterRepository interface {
	FindByID(ctx context.Context, id string) (corecharacter.Character, error)
}

// MonsterDefeatedHook is called when a monster or boss is slain during dungeon exploration.
type MonsterDefeatedHook func(ctx context.Context, characterID string, count int) error

type Service struct {
	repo                Repository
	characterRepo       CharacterRepository
	battleEngine        corebattle.Resolver
	activeStore         ActiveExpeditionStore
	dungeons            []Dungeon
	dungeonMap          map[string]Dungeon
	monsterDefeatedHook MonsterDefeatedHook
}

func (s *Service) SetMonsterDefeatedHook(hook MonsterDefeatedHook) {
	s.monsterDefeatedHook = hook
}

// Option configures optional parameters on Service.
type Option func(*Service)

// WithActiveExpeditionStore configures the transient active expedition store (e.g. ValkeyExpeditionRepository).
func WithActiveExpeditionStore(store ActiveExpeditionStore) Option {
	return func(s *Service) {
		s.activeStore = store
	}
}

// WithCustomDungeons overrides the default dungeon catalog.
func WithCustomDungeons(catalog []Dungeon) Option {
	return func(s *Service) {
		s.dungeons = catalog
		s.dungeonMap = make(map[string]Dungeon, len(catalog))
		for _, d := range catalog {
			s.dungeonMap[d.ID] = d
		}
	}
}

func NewService(
	repo Repository,
	characterRepo CharacterRepository,
	battleEngine corebattle.Resolver,
	opts ...Option,
) (*Service, error) {
	if repo == nil {
		return nil, errors.New("dungeon repository is required")
	}
	if characterRepo == nil {
		return nil, errors.New("character repository is required")
	}
	if battleEngine == nil {
		return nil, errors.New("battle engine is required")
	}

	catalog := DefaultDungeonCatalog()
	dMap := make(map[string]Dungeon, len(catalog))
	for _, d := range catalog {
		dMap[d.ID] = d
	}

	s := &Service{
		repo:          repo,
		characterRepo: characterRepo,
		battleEngine:  battleEngine,
		dungeons:      catalog,
		dungeonMap:    dMap,
	}

	for _, opt := range opts {
		opt(s)
	}

	if s.activeStore == nil {
		s.activeStore = NewMemoryExpeditionRepository()
	}

	return s, nil
}

func (s *Service) ListDungeons(ctx context.Context, characterID string) ([]DungeonOverview, error) {
	if characterID == "" {
		return nil, ErrCharacterNotFound
	}
	char, err := s.characterRepo.FindByID(ctx, characterID)
	if err != nil {
		return nil, ErrCharacterNotFound
	}
	rec, err := s.repo.GetRecord(ctx, characterID)
	if err != nil {
		return nil, err
	}

	overviews := make([]DungeonOverview, 0, len(s.dungeons))
	for _, d := range s.dungeons {
		isUnlocked := true
		lockReason := ""

		if char.Level < d.MinLevel {
			isUnlocked = false
			lockReason = fmt.Sprintf("Requires Character Level %d", d.MinLevel)
		}
		if isUnlocked && d.Tier > 1 {
			prereqTier := d.Tier - 1
			if rec.HighestDungeonCleared < prereqTier {
				isUnlocked = false
				lockReason = fmt.Sprintf("Requires clearing Tier %d Dungeon first", prereqTier)
			}
		}

		isCleared := rec.HighestDungeonCleared >= d.Tier
		overviews = append(overviews, DungeonOverview{
			Dungeon:    d,
			IsUnlocked: isUnlocked,
			IsCleared:  isCleared,
			LockReason: lockReason,
		})
	}

	return overviews, nil
}

func (s *Service) GetHistory(ctx context.Context, characterID string, limit int) ([]DungeonExpeditionHistory, error) {
	if characterID == "" {
		return nil, ErrCharacterNotFound
	}
	if limit <= 0 {
		limit = 20
	}
	return s.repo.GetHistory(ctx, characterID, limit)
}

func (s *Service) GetRecord(ctx context.Context, characterID string) (CharacterDungeonRecord, error) {
	if characterID == "" {
		return CharacterDungeonRecord{}, ErrCharacterNotFound
	}
	return s.repo.GetRecord(ctx, characterID)
}

func EncodeItems(items []string) string {
	b, _ := json.Marshal(items)
	return string(b)
}

func DecodeItems(data string) []string {
	var items []string
	_ = json.Unmarshal([]byte(data), &items)
	if items == nil {
		items = []string{}
	}
	return items
}
