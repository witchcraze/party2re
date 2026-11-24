package dungeon

import (
	"context"
	cryptorand "crypto/rand"
	"encoding/binary"
	"encoding/json"
	"errors"
	"strings"
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
	ErrJobLevelRequired          = errors.New("leader job level is too low for this dungeon")
)

type ExpeditionStatus string

const (
	StatusExploring ExpeditionStatus = "exploring"
	StatusEscaped   ExpeditionStatus = "escaped"
	// StatusWipedOut is retained only to settle carried-over pre-update frozen buffers.
	StatusWipedOut ExpeditionStatus = "wiped_out"
)

type TileEventType string

const (
	EventMove     TileEventType = "move"
	EventBattle   TileEventType = "battle"
	EventTreasure TileEventType = "treasure"
	EventBoss     TileEventType = "boss"
	EventEscape   TileEventType = "escape"
	// EventWipeout reports resumed settlement of a carried-over wiped_out buffer.
	EventWipeout TileEventType = "wipeout"
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
	combat     *corebattle.LifetimeActor
}

type Floor struct {
	FloorNumber int `json:"-"`
	// Name overrides the dungeon's display name on this floor (legacy $d_name).
	Name     string           `json:"-"`
	Width    int              `json:"width"`
	Height   int              `json:"height"`
	StartX   int              `json:"start_x"`
	StartY   int              `json:"start_y"`
	Grid     []string         `json:"grid"`
	Monsters []DungeonMonster `json:"monsters"`
	Boss     *DungeonMonster  `json:"boss,omitempty"`
	// Warps send a party entering the keyed cell to (x, y) without dispatching the landing cell.
	Warps map[byte][2]int `json:"-"`
	// Pools, when set, replace Monsters/Boss: each event draws one pool, as legacy maps load one stage file per request.
	Pools []EncounterPool `json:"-"`
	// RoundLimit overrides the dungeon's limit on this floor (legacy $max_round per map file).
	RoundLimit int `json:"-"`
	// Exits switch floors: entering the keyed cell picks a uniform candidate variant.
	Exits map[byte]FloorExit `json:"-"`
	// Bosses are map-owned once bosses with fixed content, keyed by cell.
	Bosses map[byte]DungeonMonster `json:"-"`
	// Marks show a cell's symbol in the map view until its once event is consumed, unless the event keeps it.
	Marks map[byte]string `json:"-"`
	// events bind this floor's symbols; see map_events.go.
	events map[byte]mapEvent
}

// FloorExit is a route or pitfall: the round carries over and the position is
// kept; Trap is the party damage base (_trap_d), 0 for none.
type FloorExit struct {
	Variants []int
	Trap     int
}

// EncounterPool is one stage's content; Appears weights monster indices (empty: uniform).
type EncounterPool struct {
	Monsters []DungeonMonster
	Appears  []int
	Boss     *DungeonMonster
}

type Dungeon struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	RoundLimit  int    `json:"round_limit"`
	// LeaderJobLevel is the leader's minimum job level to start; joiners are not gated.
	LeaderJobLevel int `json:"-"`
	// Variants stay private because their grids disclose hidden passages.
	Variants []Floor `json:"-"`
	// Initial counts the leading variants a run may start on; 0 means all.
	Initial int `json:"-"`
	// newState draws the map's run state in the accepted start (map14 points).
	newState func(RandomSource) json.RawMessage
}

func (f Floor) floorNumber() int {
	if f.FloorNumber > 0 {
		return f.FloorNumber
	}
	return 1
}

func (d Dungeon) limitOf(f Floor) int {
	if f.RoundLimit > 0 {
		return f.RoundLimit
	}
	return d.RoundLimit
}

func (d Dungeon) floorName(f Floor) string {
	if f.Name != "" {
		return f.Name
	}
	return d.Name
}

type ActiveExpedition struct {
	Revision         int                `json:"revision"`
	Encounter        *ActiveEncounter   `json:"encounter,omitempty"`
	ID               string             `json:"id"`
	CharacterID      string             `json:"character_id"`
	PartyID          string             `json:"party_id,omitempty"`
	PartyName        string             `json:"party_name,omitempty"`
	Members          []ExpeditionMember `json:"members,omitempty"`
	DungeonID        string             `json:"dungeon_id"`
	MapVariant       int                `json:"-"` // 1-based native layout; 0 for catalog floors
	PosX             int                `json:"pos_x"`
	PosY             int                `json:"pos_y"`
	CurrentHP        int                `json:"current_hp"`
	Round            int                `json:"round"`
	RoundLimit       int                `json:"round_limit"` // the current floor's limit
	AccumulatedExp   int                `json:"accumulated_exp"`
	AccumulatedGold  int                `json:"accumulated_gold"`
	AccumulatedItems []string           `json:"accumulated_items"`
	ConsumedEvents   []string           `json:"consumed_events,omitempty"`
	// MapState is the map's own typed run state (e.g. map14 routes), JSON
	// encoded by that map and private to it.
	MapState  json.RawMessage  `json:"-"`
	Status    ExpeditionStatus `json:"status"`
	StartedAt time.Time        `json:"started_at"`
	UpdatedAt time.Time        `json:"updated_at"`
}

type DungeonExpeditionHistory struct {
	ID               string           `json:"id"`
	CharacterID      string           `json:"character_id"`
	DungeonID        string           `json:"dungeon_id"`
	Outcome          ExpeditionStatus `json:"outcome"`
	ExpReward        int              `json:"exp_reward"`
	GoldReward       int              `json:"gold_reward"`
	ItemsRewardCount int              `json:"items_reward_count"`
	CreatedAt        time.Time        `json:"created_at"`
	// ExitRound is the shared round at Escape, the settlement input for nigeru's exit effects; not persisted.
	ExitRound int `json:"-"`
}

type ExpeditionStepResult struct {
	Expedition    ActiveExpedition   `json:"expedition"`
	EventType     TileEventType      `json:"event_type"`
	Monster       *DungeonMonster    `json:"monster,omitempty"`
	CombatOutcome corebattle.Outcome `json:"combat_outcome,omitempty"`
	BattleResult  *corebattle.Result `json:"battle_result,omitempty"`
	DamageTaken   int                `json:"damage_taken,omitempty"`
	MemberDamages map[string]int     `json:"member_damages,omitempty"`
	GoldFound     int                `json:"gold_found,omitempty"`
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
	NewX                 int
	NewY                 int
	HPDelta              int
	MemberHPDeltas       map[string]int
	MemberMPDeltas       map[string]int
	MemberExpDeltas      map[string]int
	MemberGoldDeltas     map[string]int
	ConsumedItems        map[string][]corebattle.ConsumedItem
	RoundDelta           int
	// MapVariant, when nonzero, moves the run to that floor with its RoundLimit.
	MapVariant   int
	RoundLimit   int
	ExpDelta     int
	GoldDelta    int
	RewardItemID string
	EventKey     string
	// MapState, when non-nil, replaces the run's map state.
	MapState json.RawMessage
	Now      time.Time
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
	admission          Admission
	repo               Repository
	characterRepo      CharacterRepository
	invRepo            InventoryProvider
	battleEngine       corebattle.Resolver
	activeStore        ActiveExpeditionStore
	dungeons           []Dungeon
	dungeonMap         map[string]Dungeon
	rng                RandomSource
	participantBuilder ParticipantBuilder
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
		return nil, errors.New("active expedition store is required")
	}
	if s.rng == nil {
		s.rng = defaultRandomSource{}
	}

	return s, nil
}

// ListDungeons returns the maps a leader of jobLevel may start, like the legacy menu.
func (s *Service) ListDungeons(jobLevel int) []Dungeon {
	out := make([]Dungeon, 0, len(s.dungeons))
	for _, d := range s.dungeons {
		if jobLevel >= d.LeaderJobLevel {
			out = append(out, d)
		}
	}
	return out
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

func EncodeItems(items []string) string {
	if len(items) == 0 {
		return "[]"
	}
	b, _ := json.Marshal(items)
	return string(b)
}

func DecodeItems(data string) []string {
	items, _ := decodeItems(data)
	return items
}

func decodeItems(data string) ([]string, error) {
	trimmed := strings.TrimSpace(data)
	if trimmed == "" || trimmed == "null" {
		return []string{}, nil
	}
	var items []string
	if err := json.Unmarshal([]byte(trimmed), &items); err != nil {
		return nil, err
	}
	if items == nil {
		items = []string{}
	}
	return items, nil
}

func encodeJSONMap[V any](m map[string]V) (string, error) {
	if len(m) == 0 {
		return "{}", nil
	}
	b, err := json.Marshal(m)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
