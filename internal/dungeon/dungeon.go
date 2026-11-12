package dungeon

import (
	"context"
	cryptorand "crypto/rand"
	"encoding/binary"
	"encoding/json"
	"errors"
	"time"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
)

var (
	ErrDungeonNotFound           = errors.New("dungeon not found")
	ErrCharacterNotFound         = errors.New("character not found")
	ErrActiveExpeditionExists    = errors.New("an active expedition is already in progress")
	ErrNoActiveExpedition        = errors.New("no active dungeon expedition in progress")
	ErrCombatActive              = errors.New("cannot advance until all enemies are defeated")
	ErrNoActiveCombat            = errors.New("no active dungeon combat")
	ErrExpeditionStateChanged    = errors.New("expedition state changed; retry from current state")
	ErrInvalidDirection          = errors.New("invalid move direction")
	ErrImpassableWall            = errors.New("cannot move into wall or out of map bounds")
	ErrExpeditionAlreadyFinished = errors.New("expedition is already completed")
	ErrMovementExhausted         = errors.New("movement limit reached; escape to leave the dungeon")
)

type ExpeditionStatus string

const (
	StatusExploring ExpeditionStatus = "exploring"
	StatusEscaped   ExpeditionStatus = "escaped"
	StatusWipedOut  ExpeditionStatus = "wiped_out"
)

type TileEventType string

const (
	EventMove     TileEventType = "move"
	EventBattle   TileEventType = "battle"
	EventTreasure TileEventType = "treasure"
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
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	RoundLimit  int    `json:"round_limit"`
	// Variants stay private because their grids disclose hidden passages.
	Variants []Floor `json:"-"`
}

type ActiveExpedition struct {
	Revision          int                `json:"revision"`
	Encounter         *ActiveEncounter   `json:"encounter,omitempty"`
	ID                string             `json:"id"`
	CharacterID       string             `json:"character_id"`
	PartyID           string             `json:"party_id,omitempty"`
	PartyName         string             `json:"party_name,omitempty"`
	Members           []ExpeditionMember `json:"members,omitempty"`
	DungeonID         string             `json:"dungeon_id"`
	MapVariant        int                `json:"-"` // 1-based native layout; 0 for catalog floors
	CurrentFloor      int                `json:"current_floor"`
	PosX              int                `json:"pos_x"`
	PosY              int                `json:"pos_y"`
	CurrentHP         int                `json:"current_hp"`
	TurnsRemaining    int                `json:"turns_remaining"`
	AccumulatedExp    int                `json:"accumulated_exp"`
	AccumulatedGold   int                `json:"accumulated_gold"`
	AccumulatedItems  []string           `json:"accumulated_items"`
	AccumulatedMedals int                `json:"accumulated_medals"`
	ConsumedEvents    []string           `json:"consumed_events,omitempty"`
	Status            ExpeditionStatus   `json:"status"`
	StartedAt         time.Time          `json:"started_at"`
	UpdatedAt         time.Time          `json:"updated_at"`
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

type ExpeditionStepResult struct {
	Expedition    ActiveExpedition   `json:"expedition"`
	EventType     TileEventType      `json:"event_type"`
	Message       string             `json:"message"`
	BattleResult  *corebattle.Result `json:"battle_result,omitempty"`
	DamageTaken   int                `json:"damage_taken,omitempty"`
	MemberDamages map[string]int     `json:"member_damages,omitempty"`
	GoldFound     int                `json:"gold_found,omitempty"`
	MedalsFound   int                `json:"medals_found,omitempty"`
	ItemFound     string             `json:"item_found,omitempty"`
	ChestsOpened  int                `json:"chests_opened,omitempty"`
	ExpEarned     int                `json:"exp_earned,omitempty"`
	IsFinished    bool               `json:"is_finished"`
}

// StepParams encapsulates the parameters for advancing an active dungeon expedition step atomically.
type StepParams struct {
	ExpectedExpeditionID string
	ExpectedRevision     *int
	FinishStatus         ExpeditionStatus
	Combat               bool
	Encounter            *ActiveEncounter
	NewFloor             int
	NewX                 int
	NewY                 int
	HPDelta              int
	MemberHPDeltas       map[string]int
	MemberMPDeltas       map[string]int
	MemberExpDeltas      map[string]int
	MemberGoldDeltas     map[string]int
	ConsumedItems        map[string][]corebattle.ConsumedItem
	TurnsDelta           int
	ExpDelta             int
	GoldDelta            int
	MedalsDelta          int
	RewardItemID         string
	EventKey             string
	Now                  time.Time
}

// StepOutcome represents the result of executing an atomic step against the active expedition store.
type StepOutcome struct {
	Expedition    ActiveExpedition
	Status        ExpeditionStatus
	EventConsumed bool
}

// ActiveExpeditionStore defines the storage contract for transient in-flight dungeon expeditions (Candidate D).
type ActiveExpeditionStore interface {
	GetActiveExpedition(ctx context.Context, characterID string) (*ActiveExpedition, error)
	SaveActiveExpedition(ctx context.Context, exp ActiveExpedition) error
	// DeleteActiveExpedition atomically removes only the owner's matching run; absent/replaced runs are a no-op.
	DeleteActiveExpedition(ctx context.Context, characterID, expeditionID string) error
	Step(ctx context.Context, characterID string, params StepParams) (StepOutcome, error)
}

type Repository interface {
	GetActiveExpedition(ctx context.Context, characterID string) (*ActiveExpedition, error)
	SaveActiveExpedition(ctx context.Context, exp ActiveExpedition) error
	DeleteActiveExpedition(ctx context.Context, characterID, expeditionID string) error
	FinalizeExpedition(
		ctx context.Context,
		history DungeonExpeditionHistory,
		character *corecharacter.Character,
		rewardItems []coreitem.Instance,
		members ...ExpeditionMember,
	) error
	GetHistory(ctx context.Context, characterID string, limit int) ([]DungeonExpeditionHistory, error)
}

type CharacterRepository interface {
	FindByID(ctx context.Context, id string) (corecharacter.Character, error)
}

type InventoryProvider interface {
	FindByCharacterID(ctx context.Context, characterID string) (coreinventory.Inventory, error)
}

// MonsterDefeatedHook is called when a monster or boss is slain during dungeon exploration.
type MonsterDefeatedHook func(ctx context.Context, characterID string, count int) error

type RandomSource interface {
	Intn(n int) int
	Float64() float64
}

type defaultRandomSource struct{}

func (defaultRandomSource) Intn(n int) int {
	if n <= 0 {
		return 0
	}
	var b [4]byte
	_, _ = cryptorand.Read(b[:])
	val := binary.BigEndian.Uint32(b[:])
	return int(val % uint32(n))
}

func (defaultRandomSource) Float64() float64 {
	var b [8]byte
	_, _ = cryptorand.Read(b[:])
	val := binary.BigEndian.Uint64(b[:])
	return float64(val&(1<<53-1)) / float64(1<<53)
}

// ParticipantBuilder constructs a Participant from character ID with current HP or full state.
type ParticipantBuilder interface {
	BuildParticipantWithCurrentHP(ctx context.Context, characterID string, currentHP int) (corebattle.Participant, error)
	BuildParticipantWithState(ctx context.Context, characterID string, hp int, mp *int, consumed []corebattle.ConsumedItem) (corebattle.Participant, error)
}

type Service struct {
	admission           Admission
	repo                Repository
	characterRepo       CharacterRepository
	invRepo             InventoryProvider
	battleEngine        corebattle.Resolver
	activeStore         ActiveExpeditionStore
	dungeons            []Dungeon
	dungeonMap          map[string]Dungeon
	monsterDefeatedHook MonsterDefeatedHook
	rng                 RandomSource
	participantBuilder  ParticipantBuilder
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

// WithInventoryProvider sets the inventory repository for checking items like Scope Goggles (197).
func WithInventoryProvider(invRepo InventoryProvider) Option {
	return func(s *Service) {
		s.invRepo = invRepo
	}
}

// WithRNG sets custom RandomSource for deterministic testing.
func WithRNG(rng RandomSource) Option {
	return func(s *Service) {
		s.rng = rng
	}
}

// WithParticipantBuilder configures the ParticipantBuilder.
func WithParticipantBuilder(builder ParticipantBuilder) Option {
	return func(s *Service) {
		s.participantBuilder = builder
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
	if s.rng == nil {
		s.rng = defaultRandomSource{}
	}

	return s, nil
}

// ListDungeons returns the enterable maps; admission is not yet modeled (#1955).
func (s *Service) ListDungeons() []Dungeon { return s.dungeons }

func (s *Service) GetHistory(ctx context.Context, characterID string, limit int) ([]DungeonExpeditionHistory, error) {
	if characterID == "" {
		return nil, ErrCharacterNotFound
	}
	if limit <= 0 {
		limit = 20
	}
	return s.repo.GetHistory(ctx, characterID, limit)
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

func EncodeMemberHPDeltas(deltas map[string]int) string {
	if len(deltas) == 0 {
		return "{}"
	}
	b, err := json.Marshal(deltas)
	if err != nil {
		return "{}"
	}
	return string(b)
}

func EncodeConsumedItems(items map[string][]corebattle.ConsumedItem) string {
	if len(items) == 0 {
		return "{}"
	}
	b, err := json.Marshal(items)
	if err != nil {
		return "{}"
	}
	return string(b)
}
