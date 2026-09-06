package boss

import (
	"context"
	"errors"
	"fmt"
	"time"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
)

const (
	DefaultDailyEntryLimit = 3
)

var (
	ErrBossNotFound           = errors.New("boss encounter not found")
	ErrCharacterNotFound      = errors.New("character not found")
	ErrLevelRequirementNotMet = errors.New("character level requirement not met for boss")
	ErrPrerequisiteNotMet     = errors.New("prerequisite boss tier must be cleared first")
	ErrDailyAttemptsExhausted = errors.New("daily challenge attempts exhausted for today")
	ErrInvalidBossID          = errors.New("invalid boss id")
)

type Boss struct {
	ID                   string   `json:"id"`
	Tier                 int      `json:"tier"`
	Name                 string   `json:"name"`
	Title                string   `json:"title"`
	MinLevel             int      `json:"min_level"`
	HP                   int      `json:"hp"`
	Attack               int      `json:"attack"`
	Defense              int      `json:"defense"`
	Agility              int      `json:"agility"`
	ExperienceReward     int      `json:"experience_reward"`
	GoldReward           int      `json:"gold_reward"`
	DropItemIDs          []string `json:"drop_item_ids"`
	FirstClearExpBonus   int      `json:"first_clear_exp_bonus"`
	FirstClearGoldBonus  int      `json:"first_clear_gold_bonus"`
	SmallMedalReward     int      `json:"small_medal_reward"`
	FirstClearMedalBonus int      `json:"first_clear_medal_bonus"`
	DailyEntryLimit      int      `json:"daily_entry_limit"`
}

type CharacterBossRecord struct {
	CharacterID          string
	HighestTierCleared   int
	TotalBossDefeats     int
	FirstClearedAt       *time.Time
	LastChallengedAt     *time.Time
	DailyAttemptsUsed    int
	DailyAttemptsResetAt time.Time
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

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

type BossEncounterStatus struct {
	Boss              Boss   `json:"boss"`
	IsUnlocked        bool   `json:"is_unlocked"`
	IsCleared         bool   `json:"is_cleared"`
	AttemptsRemaining int    `json:"attempts_remaining"`
	LockReason        string `json:"lock_reason,omitempty"`
}

type ChallengeResult struct {
	BattleResult      corebattle.Result   `json:"battle_result"`
	Outcome           corebattle.Outcome  `json:"outcome"`
	ExperienceReward  int                 `json:"experience_reward"`
	GoldReward        int                 `json:"gold_reward"`
	SmallMedalsReward int                 `json:"small_medals_reward"`
	ItemRewardID      string              `json:"item_reward_id,omitempty"`
	IsFirstClear      bool                `json:"is_first_clear"`
	UpdatedRecord     CharacterBossRecord `json:"updated_record"`
}

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
}

type VictoryBanquetHook func(ctx context.Context, bossID, bossName, slayerID, slayerName string, tier int) error

// VictoryHook is called when a character successfully slays a boss.
type VictoryHook func(ctx context.Context, characterID string, bossID string, tier int) error

type Service struct {
	repo               Repository
	characterRepo      CharacterRepository
	battleEngine       corebattle.Resolver
	bosses             []Boss
	bossMap            map[string]Boss
	victoryBanquetHook VictoryBanquetHook
	victoryHook        VictoryHook
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
	customBosses ...Boss,
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

	bossMap := make(map[string]Boss, len(catalog))
	for _, b := range catalog {
		bossMap[b.ID] = b
	}

	return &Service{
		repo:          repo,
		characterRepo: characterRepo,
		battleEngine:  battleEngine,
		bosses:        catalog,
		bossMap:       bossMap,
	}, nil
}

func (s *Service) ListBosses(ctx context.Context, characterID string) ([]BossEncounterStatus, error) {
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
	rec.ResetDailyAttemptsIfExpired(time.Now().UTC())

	statuses := make([]BossEncounterStatus, 0, len(s.bosses))
	for _, b := range s.bosses {
		isUnlocked := true
		lockReason := ""

		// Check level requirement
		if char.Level < b.MinLevel {
			isUnlocked = false
			lockReason = fmt.Sprintf("Requires Character Level %d", b.MinLevel)
		}

		// Check prerequisite tier requirement
		if isUnlocked && b.Tier > 1 {
			prereqTier := b.Tier - 1
			if b.Tier == 99 {
				prereqTier = 10
			}
			if rec.HighestTierCleared < prereqTier {
				isUnlocked = false
				lockReason = fmt.Sprintf("Requires clearing Tier %d Boss first", prereqTier)
			}
		}

		isCleared := rec.HighestTierCleared >= b.Tier
		attemptsRemaining := max(0, b.DailyEntryLimit-rec.DailyAttemptsUsed)

		statuses = append(statuses, BossEncounterStatus{
			Boss:              b,
			IsUnlocked:        isUnlocked,
			IsCleared:         isCleared,
			AttemptsRemaining: attemptsRemaining,
			LockReason:        lockReason,
		})
	}

	return statuses, nil
}
