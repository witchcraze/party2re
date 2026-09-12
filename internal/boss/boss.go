package boss

import (
	"context"
	"errors"
	"time"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/party"
)

var (
	ErrBossNotFound         = errors.New("boss stage not found")
	ErrCharacterNotFound    = errors.New("character not found")
	ErrInvalidBossID        = errors.New("invalid boss id")
	ErrPartyNotFound        = errors.New("party not found")
	ErrNotPartyLeader       = errors.New("only party leader can start sealing battle")
	ErrPartyNotReady        = errors.New("all party members must be ready before starting")
	ErrPartyNotRecruiting   = errors.New("party is not currently recruiting")
	ErrNeedJoinNotMet       = errors.New("character does not meet party join condition")
	ErrCharacterUnconscious = errors.New("character is unconscious (HP <= 0)")
	ErrCharacterExhausted   = errors.New("character is exhausted (tired >= 100)")
)

// BossStage defines an authentic King stage (stage/king1.cgi - king10.cgi, king99.cgi).
type BossStage struct {
	ID              string        `json:"id"`
	Name            string        `json:"name"`
	LeaderName      string        `json:"leader_name"`
	Speed           int           `json:"speed"`
	MaxMembers      int           `json:"max_members"`
	NeedJoin        string        `json:"need_join"`
	Bosses          []BossMonster `json:"bosses"`
	TreasureItemIDs []string      `json:"treasure_item_ids"`
}

// Boss is an alias for BossStage for compatibility.
type Boss = BossStage

// BossMonster represents a combatant in a King sealing battle.
type BossMonster struct {
	Name       string `json:"name"`
	HP         int    `json:"hp"`
	MP         int    `json:"mp"`
	MaxHP      int    `json:"max_hp"`
	MaxMP      int    `json:"max_mp"`
	Attack     int    `json:"attack"`
	Defense    int    `json:"defense"`
	Agility    int    `json:"agility"`
	GetExp     int    `json:"get_exp"`
	GetMoney   int    `json:"get_money"`
	Icon       string `json:"icon"`
	Job        int    `json:"job"`
	SP         int    `json:"sp"`
	OldJob     int    `json:"old_job"`
	OldSP      int    `json:"old_sp"`
	TMP        string `json:"tmp"`
	State      string `json:"state"`
	GetCrystal int    `json:"get_crystal"`
}

// CharacterBossRecord tracks boss defeat progress in MariaDB.
type CharacterBossRecord struct {
	CharacterID        string
	HighestTierCleared int
	TotalBossDefeats   int
	FirstClearedAt     *time.Time
	LastChallengedAt   *time.Time
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// BossChallengeHistory records a completed sealing battle.
type BossChallengeHistory struct {
	ID                string
	CharacterID       string
	BossID            string
	Tier              int
	Outcome           corebattle.Outcome
	Turns             int
	RewardExp         int
	RewardGold        int
	RewardSmallMedals int
	RewardItemID      string
	IsFirstClear      bool
	CreatedAt         time.Time
}

// BossStageStatus represents the availability of a King stage for a character.
type BossStageStatus struct {
	Boss             BossStage `json:"boss"`
	IsEligible       bool      `json:"is_eligible"`
	IsUnlocked       bool      `json:"is_unlocked"`
	IsCleared        bool      `json:"is_cleared"`
	IneligibleReason string    `json:"ineligible_reason,omitempty"`
	LockReason       string    `json:"lock_reason,omitempty"`
}

// BossEncounterStatus is an alias for BossStageStatus.
type BossEncounterStatus = BossStageStatus

// SealingBattleResult captures the outcome of an authentic Sealing Battle (vs_king.cgi).
type SealingBattleResult struct {
	StageID           string                       `json:"stage_id"`
	StageName         string                       `json:"stage_name"`
	Outcome           corebattle.Outcome           `json:"outcome"`
	Turns             int                          `json:"turns"`
	RewardExp         int                          `json:"reward_exp"`
	RewardGold        int                          `json:"reward_gold"`
	RewardItemID      string                       `json:"reward_item_id,omitempty"`
	HeroCountGained   int                          `json:"hero_count_gained"`
	BanishedMemberIDs []string                     `json:"banished_member_ids,omitempty"`
	NewsMessage       string                       `json:"news_message,omitempty"`
	BanquetHeld       bool                         `json:"banquet_held"`
	BattleResult      corebattle.PartyBattleResult `json:"battle_result"`
}

// ChallengeResult is an alias for SealingBattleResult for backward compatibility.
type ChallengeResult = SealingBattleResult

// BossLeaderboardEntry tracks top boss slayers.
type BossLeaderboardEntry struct {
	CharacterID        string     `json:"character_id"`
	CharacterName      string     `json:"character_name"`
	CharacterLevel     int        `json:"character_level"`
	JobID              string     `json:"job_id"`
	HighestTierCleared int        `json:"highest_tier_cleared"`
	TotalBossDefeats   int        `json:"total_boss_defeats"`
	FirstClearedAt     *time.Time `json:"first_cleared_at,omitempty"`
}

type Repository interface {
	GetOrCreateRecord(ctx context.Context, characterID string) (CharacterBossRecord, error)
	RecordChallenge(
		ctx context.Context,
		history BossChallengeHistory,
		record CharacterBossRecord,
		character corecharacter.Character,
		rewardItem *coreitem.Instance,
	) error
	GetHistory(ctx context.Context, characterID string, limit int) ([]BossChallengeHistory, error)
	GetLeaderboard(ctx context.Context, limit int) ([]BossLeaderboardEntry, error)
}

type CharacterRepository interface {
	FindByID(ctx context.Context, id string) (corecharacter.Character, error)
	FindByIDForUpdate(ctx context.Context, id string) (corecharacter.Character, error)
	Update(ctx context.Context, char corecharacter.Character) error
}

type PartyRepository interface {
	GetPartyForUpdate(ctx context.Context, id string) (party.Party, error)
	GetMembers(ctx context.Context, partyID string) ([]party.Member, error)
	DeleteParty(ctx context.Context, id string) error
	UpdateParty(ctx context.Context, p party.Party) error
}

type NewsPublisher interface {
	PublishNews(ctx context.Context, category, title, content, author string, publishedAt time.Time) error
}

type NewsPublisherFunc func(ctx context.Context, category, title, content, author string, publishedAt time.Time) error

func (f NewsPublisherFunc) PublishNews(ctx context.Context, category, title, content, author string, publishedAt time.Time) error {
	return f(ctx, category, title, content, author, publishedAt)
}

type TransactionProvider interface {
	RunInTx(ctx context.Context, fn func(txCtx context.Context) error) error
}

type VictoryBanquetHook func(ctx context.Context, bossID, bossName, slayerID, slayerName string, tier int) error
type VictoryHook func(ctx context.Context, characterID string, bossID string, tier int) error

type ServiceOption func(*Service)

func WithPartyRepository(repo PartyRepository) ServiceOption {
	return func(s *Service) {
		s.partyRepo = repo
	}
}

func WithNewsPublisher(pub NewsPublisher) ServiceOption {
	return func(s *Service) {
		s.newsPub = pub
	}
}

func WithTransactionProvider(tx TransactionProvider) ServiceOption {
	return func(s *Service) {
		s.txProvider = tx
	}
}

func WithRandomSource(rng corecharacter.RandomSource) ServiceOption {
	return func(s *Service) {
		s.rng = rng
	}
}

type Service struct {
	repo               Repository
	characterRepo      CharacterRepository
	partyRepo          PartyRepository
	battleEngine       corebattle.Resolver
	stages             []BossStage
	stageMap           map[string]BossStage
	victoryBanquetHook VictoryBanquetHook
	victoryHook        VictoryHook
	newsPub            NewsPublisher
	txProvider         TransactionProvider
	rng                corecharacter.RandomSource
}

func (s *Service) SetVictoryBanquetHook(hook VictoryBanquetHook) {
	s.victoryBanquetHook = hook
}

func (s *Service) SetVictoryHook(hook VictoryHook) {
	s.victoryHook = hook
}

func NewService(
	repo Repository,
	characterRepo CharacterRepository,
	battleEngine corebattle.Resolver,
	customBosses ...BossStage,
) (*Service, error) {
	if repo == nil {
		return nil, errors.New("boss repository is required")
	}
	if characterRepo == nil {
		return nil, errors.New("character repository is required")
	}
	if battleEngine == nil {
		return nil, errors.New("battle engine is required")
	}

	catalog := DefaultBossCatalog()
	if len(customBosses) > 0 {
		catalog = customBosses
	}

	stageMap := make(map[string]BossStage, len(catalog))
	for _, b := range catalog {
		stageMap[b.ID] = b
	}

	return &Service{
		repo:          repo,
		characterRepo: characterRepo,
		battleEngine:  battleEngine,
		stages:        catalog,
		stageMap:      stageMap,
	}, nil
}

func (s *Service) Configure(opts ...ServiceOption) {
	for _, opt := range opts {
		opt(s)
	}
}

func (s *Service) ListStages(ctx context.Context, characterID string) ([]BossStageStatus, error) {
	if characterID == "" {
		return nil, ErrCharacterNotFound
	}
	char, err := s.characterRepo.FindByID(ctx, characterID)
	if err != nil {
		return nil, ErrCharacterNotFound
	}
	rec, err := s.repo.GetOrCreateRecord(ctx, characterID)
	if err != nil {
		return nil, err
	}

	statuses := make([]BossStageStatus, 0, len(s.stages))
	for _, stage := range s.stages {
		isEligible := true
		ineligibleReason := ""

		if char.Stats.HP <= 0 {
			isEligible = false
			ineligibleReason = "Character is unconscious"
		} else if char.Tired >= 100 {
			isEligible = false
			ineligibleReason = "Character is exhausted (Tired >= 100)"
		} else if err := party.ValidateNeedJoin(stage.NeedJoin, char); err != nil {
			isEligible = false
			ineligibleReason = err.Error()
		}

		isCleared := rec.TotalBossDefeats > 0

		statuses = append(statuses, BossStageStatus{
			Boss:             stage,
			IsEligible:       isEligible,
			IsUnlocked:       isEligible,
			IsCleared:        isCleared,
			IneligibleReason: ineligibleReason,
			LockReason:       ineligibleReason,
		})
	}

	return statuses, nil
}

// ListBosses is an alias for ListStages.
func (s *Service) ListBosses(ctx context.Context, characterID string) ([]BossStageStatus, error) {
	return s.ListStages(ctx, characterID)
}

func (s *Service) runInTx(ctx context.Context, fn func(txCtx context.Context) error) error {
	if s.txProvider != nil {
		return s.txProvider.RunInTx(ctx, fn)
	}
	return fn(ctx)
}
