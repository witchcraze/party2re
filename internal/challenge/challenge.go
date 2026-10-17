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
	"github.com/witchcraze/party2re/internal/party"
)

//go:embed data/challenge_tiers.json
var challengeTiersData []byte

var (
	ErrSessionNotFound      = errors.New("ERR_SESSION_NOT_FOUND: challenge session not found")
	ErrSessionNotActive     = errors.New("ERR_SESSION_NOT_ACTIVE: challenge session is not active")
	ErrSessionIDMismatch    = errors.New("ERR_SESSION_ID_MISMATCH: challenge session id mismatch")
	ErrActiveSessionExists  = errors.New("active challenge session already exists")
	ErrTierNotFound         = errors.New("challenge tier not found")
	ErrLevelTooLow          = errors.New("character level is too low for this tier")
	ErrCharacterNotFound    = errors.New("character not found")
	ErrForbidden            = errors.New("forbidden: character does not own this challenge session")
	ErrPartyNotFound        = errors.New("party not found")
	ErrNotPartyLeader       = errors.New("only party leader can start party session")
	ErrPartyNotRecruiting   = errors.New("party is not currently recruiting")
	ErrPartyNotReady        = errors.New("all party members must be ready before starting")
	ErrDuplicatePartyMember = errors.New("party has duplicate members")
	ErrCharacterUnconscious = errors.New("character is unconscious (HP <= 0)")
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
	ID               string             `json:"id"`
	Name             string             `json:"name"`
	Description      string             `json:"description"`
	MinLevel         int                `json:"min_level"`
	MaxParticipants  int                `json:"max_participants"`
	NeedJoin         string             `json:"need_join,omitempty"`
	NeedOverLv       bool               `json:"need_over_lv,omitempty"`
	TreasureRound    int                `json:"treasure_round"`
	TreasureItemPool []string           `json:"treasure_item_pool,omitempty"`
	BaseMonster      ChallengeMonster   `json:"base_monster,omitempty"`
	MonsterPool      []ChallengeMonster `json:"monster_pool,omitempty"`
}

type ChallengeMember struct {
	CharacterID        string `json:"character_id"`
	CharacterName      string `json:"character_name"`
	Icon               string `json:"icon"`
	JobID              string `json:"job_id"`
	OldJobID           string `json:"old_job_id,omitempty"`
	Level              int    `json:"level"`
	CharacterCurrentHP int    `json:"character_current_hp"`
	MaxHP              int    `json:"max_hp"`
	MaxMP              int    `json:"max_mp"`
	Attack             int    `json:"attack"`
	Defense            int    `json:"defense"`
	Agility            int    `json:"agility"`
}

type HallOfFameMember struct {
	CharacterID   string `json:"character_id"`
	CharacterName string `json:"character_name"`
	Icon          string `json:"icon"`
	JobID         string `json:"job_id"`
	JobName       string `json:"job_name,omitempty"`
	OldJobID      string `json:"old_job_id,omitempty"`
	OldJobName    string `json:"old_job_name,omitempty"`
	HP            int    `json:"hp"`
	MP            int    `json:"mp"`
	Attack        int    `json:"attack"`
	Defense       int    `json:"defense"`
	Agility       int    `json:"agility"`
}

type HallOfFameEntry struct {
	TierID       string             `json:"tier_id"`
	HighestRound int                `json:"highest_round"`
	PartyName    string             `json:"party_name"`
	PartyColor   string             `json:"party_color"`
	ClearedAt    time.Time          `json:"cleared_at"`
	Members      []HallOfFameMember `json:"members"`
}

type ChallengeSession struct {
	ID                 string            `json:"id"`
	CharacterID        string            `json:"character_id"`
	PartyID            string            `json:"party_id,omitempty"`
	PartyName          string            `json:"party_name,omitempty"`
	PartyColor         string            `json:"party_color,omitempty"`
	Members            []ChallengeMember `json:"members,omitempty"`
	TierID             string            `json:"tier_id"`
	CurrentRound       int               `json:"current_round"`
	CharacterCurrentHP int               `json:"character_current_hp"`
	AccumulatedExp     int               `json:"accumulated_exp"`
	AccumulatedGold    int               `json:"accumulated_gold"`
	AccumulatedItems   []string          `json:"accumulated_items"`
	Status             SessionStatus     `json:"status"`
	CreatedAt          time.Time         `json:"created_at"`
	UpdatedAt          time.Time         `json:"updated_at"`
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

type SessionRepository interface {
	SaveSession(ctx context.Context, session ChallengeSession) error
	FindSessionByID(ctx context.Context, id string) (*ChallengeSession, error)
	FindActiveSessionByCharacter(ctx context.Context, characterID string) (*ChallengeSession, error)
	UpdateSession(ctx context.Context, session ChallengeSession) error
	FinalizeSession(ctx context.Context, session ChallengeSession, expReward int, goldReward int, items []string, newStreak int) error
}

type RecordRepository interface {
	SaveRecord(ctx context.Context, record CharacterChallengeRecord) error
	FindRecord(ctx context.Context, characterID string, tierID string) (*CharacterChallengeRecord, error)
	FindRecordsByCharacter(ctx context.Context, characterID string) ([]CharacterChallengeRecord, error)
	GetLeaderboard(ctx context.Context, tierID string, limit int) ([]LeaderboardEntry, error)
}

type HallOfFameRepository interface {
	SaveHallOfFame(ctx context.Context, entry HallOfFameEntry) error
	GetHallOfFame(ctx context.Context, tierID string) (*HallOfFameEntry, error)
	ListHallOfFame(ctx context.Context) ([]HallOfFameEntry, error)
}

type Repository interface {
	SessionRepository
	RecordRepository
	HallOfFameRepository
}

type PartyRepository interface {
	GetParty(ctx context.Context, id string) (party.Party, error)
	GetMembers(ctx context.Context, partyID string) ([]party.Member, error)
	UpdateParty(ctx context.Context, p party.Party) error
	UpdateMemberReady(ctx context.Context, partyID string, characterID string, ready bool) error
	WithPartyLock(ctx context.Context, partyID string, fn func(ctx context.Context) error) error
}

// ActivityChecker validates whether a character has an active activity in another domain.
type ActivityChecker interface {
	HasActiveActivity(ctx context.Context, characterID string) (bool, error)
}

// ActivityCheckerFunc allows using a function as an ActivityChecker.
type ActivityCheckerFunc func(ctx context.Context, characterID string) (bool, error)

func (f ActivityCheckerFunc) HasActiveActivity(ctx context.Context, characterID string) (bool, error) {
	return f(ctx, characterID)
}

// ParticipantBuilder constructs a Participant from character ID with current HP.
type ParticipantBuilder interface {
	BuildParticipantWithCurrentHP(ctx context.Context, characterID string, currentHP int) (corebattle.Participant, error)
}

type Service struct {
	repo               Repository
	charRepo           CharacterRepository
	battleEngine       corebattle.Resolver
	activeStore        ActiveSessionStore
	partyRepo          PartyRepository
	activityChecker    ActivityChecker
	tiers              map[string]ChallengeTier
	participantBuilder ParticipantBuilder
}

func (s *Service) Configure(opts ...Option) {
	for _, opt := range opts {
		opt(s)
	}
}

func (s *Service) resetPartyState(ctx context.Context, partyID string) error {
	if partyID == "" || s.partyRepo == nil {
		return nil
	}

	resetFn := func(lockedCtx context.Context) error {
		p, err := s.partyRepo.GetParty(lockedCtx, partyID)
		if err != nil {
			return err
		}
		if p.Status != party.StatusInProgress {
			return nil
		}
		members, err := s.partyRepo.GetMembers(lockedCtx, partyID)
		if err != nil {
			return err
		}
		for _, m := range members {
			if err := s.partyRepo.UpdateMemberReady(lockedCtx, partyID, m.CharacterID, false); err != nil {
				return err
			}
		}
		p.Status = party.StatusRecruiting
		return s.partyRepo.UpdateParty(lockedCtx, p)
	}

	// Use detached context with a safety timeout so party reset is not aborted if the incoming HTTP request context is cancelled.
	resetCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()

	const maxRetries = 5
	backoff := 20 * time.Millisecond
	var lastErr error

	for attempt := 0; attempt < maxRetries; attempt++ {
		lastErr = s.partyRepo.WithPartyLock(resetCtx, partyID, resetFn)
		if lastErr == nil {
			return nil
		}
		if resetCtx.Err() != nil {
			return resetCtx.Err()
		}
		if !errors.Is(lastErr, party.ErrPartyNotRecruiting) {
			return lastErr
		}
		select {
		case <-resetCtx.Done():
			return resetCtx.Err()
		case <-time.After(backoff):
		}
		backoff *= 2
	}

	return lastErr
}

// Option configures optional parameters on Service.
type Option func(*Service)

// WithPartyRepository configures the PartyRepository for party roster derivation.
func WithPartyRepository(repo PartyRepository) Option {
	return func(s *Service) {
		s.partyRepo = repo
	}
}

// WithActivityChecker configures an optional external ActivityChecker on Service.
func WithActivityChecker(checker ActivityChecker) Option {
	return func(s *Service) {
		s.activityChecker = checker
	}
}

// WithParticipantBuilder configures the ParticipantBuilder.
func WithParticipantBuilder(builder ParticipantBuilder) Option {
	return func(s *Service) {
		s.participantBuilder = builder
	}
}

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
	order := []string{"0", "1", "2", "3", "4", "5", "6", "7", "8"}
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
