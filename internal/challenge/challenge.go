package challenge

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

//go:embed data/challenge_tiers.json
var challengeTiersData []byte

var (
	ErrSessionNotFound     = errors.New("ERR_SESSION_NOT_FOUND: challenge session not found")
	ErrSessionNotActive    = errors.New("ERR_SESSION_NOT_ACTIVE: challenge session is not active")
	ErrSessionIDMismatch   = errors.New("ERR_SESSION_ID_MISMATCH: challenge session id mismatch")
	ErrActiveSessionExists = errors.New("active challenge session already exists")
	ErrTierNotFound        = errors.New("challenge tier not found")
	ErrLevelTooLow         = errors.New("character level is too low for this tier")
	ErrCharacterNotFound   = errors.New("character not found")
	ErrForbidden           = errors.New("forbidden: character does not own this challenge session")
)

type SessionStatus string

const (
	StatusActive   SessionStatus = "active"
	StatusClaimed  SessionStatus = "claimed"
	StatusDefeated SessionStatus = "defeated"
)

type ChallengeMonster struct {
	Name        string `json:"name"`
	BaseHP      int    `json:"base_hp"`
	BaseAttack  int    `json:"base_attack"`
	BaseDefense int    `json:"base_defense"`
	BaseExp     int    `json:"base_exp"`
	BaseGold    int    `json:"base_gold"`
}

type ChallengeTier struct {
	ID                string           `json:"id"`
	Name              string           `json:"name"`
	Description       string           `json:"description"`
	MinLevel          int              `json:"min_level"`
	BaseMonster       ChallengeMonster `json:"base_monster"`
	ScaleFactor       float64          `json:"scale_factor"`
	MilestoneInterval int              `json:"milestone_interval"`
	MilestoneItemPool []string         `json:"milestone_item_pool"`
}

type ChallengeSession struct {
	ID                 string        `json:"id"`
	CharacterID        string        `json:"character_id"`
	TierID             string        `json:"tier_id"`
	CurrentRound       int           `json:"current_round"`
	CharacterCurrentHP int           `json:"character_current_hp"`
	AccumulatedExp     int           `json:"accumulated_exp"`
	AccumulatedGold    int           `json:"accumulated_gold"`
	AccumulatedItems   []string      `json:"accumulated_items"`
	Status             SessionStatus `json:"status"`
	CreatedAt          time.Time     `json:"created_at"`
	UpdatedAt          time.Time     `json:"updated_at"`
}

type CharacterChallengeRecord struct {
	CharacterID    string    `json:"character_id"`
	TierID         string    `json:"tier_id"`
	HighestRound   int       `json:"highest_round"`
	TotalAttempts  int       `json:"total_attempts"`
	TotalVictories int       `json:"total_victories"`
	BestClearedAt  time.Time `json:"best_cleared_at"`
}

type LeaderboardEntry struct {
	CharacterID   string    `json:"character_id"`
	CharacterName string    `json:"character_name"`
	Level         int       `json:"level"`
	JobID         string    `json:"job_id"`
	HighestRound  int       `json:"highest_round"`
	BestClearedAt time.Time `json:"best_cleared_at"`
}

type RoundResult struct {
	Round              int               `json:"round"`
	MonsterName        string            `json:"monster_name"`
	BattleResult       corebattle.Result `json:"battle_result"`
	Won                bool              `json:"won"`
	RecoveredHP        int               `json:"recovered_hp"`
	CharacterCurrentHP int               `json:"character_current_hp"`
	RoundExp           int               `json:"round_exp"`
	RoundGold          int               `json:"round_gold"`
	AwardedItem        string            `json:"awarded_item,omitempty"`
	SessionEnded       bool              `json:"session_ended"`
	SessionStatus      SessionStatus     `json:"session_status"`
}

type CashoutResult struct {
	RoundsCleared  int      `json:"rounds_cleared"`
	AwardedExp     int      `json:"awarded_exp"`
	AwardedGold    int      `json:"awarded_gold"`
	AwardedItems   []string `json:"awarded_items"`
	NewRecordRound int      `json:"new_record_round"`
}

// AdvanceRoundParams encapsulates the parameters for advancing an active challenge session round atomically.
type AdvanceRoundParams struct {
	ExpectedSessionID string
	SurvivingHP       int
	ExpDelta          int
	GoldDelta         int
	RewardItemID      string
	Now               time.Time
}

// AdvanceRoundOutcome represents the result of executing an atomic round advancement against the active session store.
type AdvanceRoundOutcome struct {
	Session ChallengeSession
	Status  SessionStatus
}

// ActiveSessionStore defines the storage contract for transient in-flight challenge sessions (Candidate D).
type ActiveSessionStore interface {
	GetActiveSession(ctx context.Context, characterID string) (*ChallengeSession, error)
	SaveActiveSession(ctx context.Context, session ChallengeSession) error
	DeleteActiveSession(ctx context.Context, characterID string) error
	AdvanceRound(ctx context.Context, characterID string, params AdvanceRoundParams) (AdvanceRoundOutcome, error)
}

type CharacterRepository interface {
	FindByID(ctx context.Context, id string) (corecharacter.Character, error)
}

type Repository interface {
	SaveSession(ctx context.Context, session ChallengeSession) error
	FindSessionByID(ctx context.Context, id string) (*ChallengeSession, error)
	FindActiveSessionByCharacter(ctx context.Context, characterID string) (*ChallengeSession, error)
	UpdateSession(ctx context.Context, session ChallengeSession) error
	SaveRecord(ctx context.Context, record CharacterChallengeRecord) error
	FindRecord(ctx context.Context, characterID string, tierID string) (*CharacterChallengeRecord, error)
	FindRecordsByCharacter(ctx context.Context, characterID string) ([]CharacterChallengeRecord, error)
	GetLeaderboard(ctx context.Context, tierID string, limit int) ([]LeaderboardEntry, error)
	FinalizeSession(ctx context.Context, session ChallengeSession, expReward int, goldReward int, items []string, newStreak int) error
}

type Service struct {
	repo         Repository
	charRepo     CharacterRepository
	battleEngine corebattle.Resolver
	activeStore  ActiveSessionStore
	tiers        map[string]ChallengeTier
}

// Option configures optional parameters on Service.
type Option func(*Service)

// WithActiveSessionStore configures the transient active challenge session store (e.g. ValkeySessionRepository).
func WithActiveSessionStore(store ActiveSessionStore) Option {
	return func(s *Service) {
		s.activeStore = store
	}
}

// WithCustomTiers overrides the default challenge tiers catalog.
func WithCustomTiers(tiers map[string]ChallengeTier) Option {
	return func(s *Service) {
		s.tiers = tiers
	}
}

func NewService(
	repo Repository,
	charRepo CharacterRepository,
	battleEngine corebattle.Resolver,
	opts ...Option,
) (*Service, error) {
	if repo == nil {
		return nil, errors.New("challenge repository is required")
	}
	if charRepo == nil {
		return nil, errors.New("character repository is required")
	}
	if battleEngine == nil {
		battleEngine = corebattle.Engine{}
	}

	tiers, err := loadTiers()
	if err != nil {
		return nil, fmt.Errorf("load challenge tiers catalog: %w", err)
	}

	s := &Service{
		repo:         repo,
		charRepo:     charRepo,
		battleEngine: battleEngine,
		tiers:        tiers,
	}

	for _, opt := range opts {
		opt(s)
	}

	if s.activeStore == nil {
		s.activeStore = NewMemorySessionRepository()
	}

	return s, nil
}

func loadTiers() (map[string]ChallengeTier, error) {
	var list []ChallengeTier
	if err := json.Unmarshal(challengeTiersData, &list); err != nil {
		return nil, fmt.Errorf("decode challenge tiers JSON: %w", err)
	}
	tiers := make(map[string]ChallengeTier, len(list))
	for _, t := range list {
		if t.ID == "" {
			return nil, fmt.Errorf("challenge tier entry has empty id")
		}
		tiers[t.ID] = t
	}
	return tiers, nil
}

func (s *Service) ListTiers() []ChallengeTier {
	order := []string{"novice", "intermediate", "master", "abyss"}
	var list []ChallengeTier
	for _, id := range order {
		if t, ok := s.tiers[id]; ok {
			list = append(list, t)
		}
	}
	return list
}

func (s *Service) GetTier(tierID string) (*ChallengeTier, error) {
	t, ok := s.tiers[tierID]
	if !ok {
		return nil, ErrTierNotFound
	}
	return &t, nil
}

func (s *Service) GetCharacterRecords(ctx context.Context, characterID string) ([]CharacterChallengeRecord, error) {
	if strings.TrimSpace(characterID) == "" {
		return nil, errors.New("character id is required")
	}
	return s.repo.FindRecordsByCharacter(ctx, characterID)
}

func (s *Service) GetRecord(ctx context.Context, characterID string, tierID string) (*CharacterChallengeRecord, error) {
	return s.repo.FindRecord(ctx, characterID, tierID)
}

func (s *Service) GetLeaderboard(ctx context.Context, tierID string, limit int) ([]LeaderboardEntry, error) {
	if limit <= 0 {
		limit = 20
	}
	return s.repo.GetLeaderboard(ctx, tierID, limit)
}

func EncodeJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func DecodeJSON[T any](data string) (T, error) {
	var target T
	err := json.Unmarshal([]byte(data), &target)
	return target, err
}
