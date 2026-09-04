package boss

import (
	"context"
	"errors"
	"fmt"
	"time"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/core/progression"
	"github.com/witchcraze/party2re/internal/id"
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

func (r *CharacterBossRecord) ResetDailyAttemptsIfExpired(today time.Time) {
	todayDate := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, time.UTC)
	resetDate := time.Date(r.DailyAttemptsResetAt.Year(), r.DailyAttemptsResetAt.Month(), r.DailyAttemptsResetAt.Day(), 0, 0, 0, 0, time.UTC)
	if todayDate.After(resetDate) {
		r.DailyAttemptsUsed = 0
		r.DailyAttemptsResetAt = todayDate
	}
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

func DefaultBossCatalog() []Boss {
	return []Boss{
		{
			ID:                   "king-01",
			Tier:                 1,
			Name:                 "レッドストーン・ガーディアン",
			Title:                "封印の尖兵",
			MinLevel:             15,
			HP:                   250,
			Attack:               45,
			Defense:              30,
			Agility:              20,
			ExperienceReward:     300,
			GoldReward:           500,
			DropItemIDs:          []string{"potion"},
			FirstClearExpBonus:   500,
			FirstClearGoldBonus:  1000,
			SmallMedalReward:     1,
			FirstClearMedalBonus: 1,
			DailyEntryLimit:      DefaultDailyEntryLimit,
		},
		{
			ID:                   "king-02",
			Tier:                 2,
			Name:                 "ブルーストーン・ゴーレム",
			Title:                "氷結の守護神",
			MinLevel:             25,
			HP:                   500,
			Attack:               80,
			Defense:              60,
			Agility:              35,
			ExperienceReward:     600,
			GoldReward:           1000,
			DropItemIDs:          []string{"high-potion"},
			FirstClearExpBonus:   1000,
			FirstClearGoldBonus:  2000,
			SmallMedalReward:     1,
			FirstClearMedalBonus: 1,
			DailyEntryLimit:      DefaultDailyEntryLimit,
		},
		{
			ID:                   "king-03",
			Tier:                 3,
			Name:                 "エメラルド・ワイバーン",
			Title:                "碧空の暴君",
			MinLevel:             35,
			HP:                   900,
			Attack:               130,
			Defense:              95,
			Agility:              55,
			ExperienceReward:     1000,
			GoldReward:           1800,
			DropItemIDs:          []string{"ether"},
			FirstClearExpBonus:   1500,
			FirstClearGoldBonus:  3000,
			SmallMedalReward:     1,
			FirstClearMedalBonus: 2,
			DailyEntryLimit:      DefaultDailyEntryLimit,
		},
		{
			ID:                   "king-04",
			Tier:                 4,
			Name:                 "アメジスト・ロード",
			Title:                "紫電の魔将",
			MinLevel:             45,
			HP:                   1400,
			Attack:               190,
			Defense:              140,
			Agility:              75,
			ExperienceReward:     1600,
			GoldReward:           2800,
			DropItemIDs:          []string{"high-ether"},
			FirstClearExpBonus:   2500,
			FirstClearGoldBonus:  5000,
			SmallMedalReward:     2,
			FirstClearMedalBonus: 2,
			DailyEntryLimit:      DefaultDailyEntryLimit,
		},
		{
			ID:                   "king-05",
			Tier:                 5,
			Name:                 "トパーズ・キメラ",
			Title:                "砂塵の獣王",
			MinLevel:             55,
			HP:                   2000,
			Attack:               260,
			Defense:              190,
			Agility:              100,
			ExperienceReward:     2400,
			GoldReward:           4000,
			DropItemIDs:          []string{"elixir"},
			FirstClearExpBonus:   3500,
			FirstClearGoldBonus:  7000,
			SmallMedalReward:     2,
			FirstClearMedalBonus: 3,
			DailyEntryLimit:      DefaultDailyEntryLimit,
		},
		{
			ID:                   "king-06",
			Tier:                 6,
			Name:                 "オブシディアン・ナイト",
			Title:                "黒曜の覇者",
			MinLevel:             65,
			HP:                   2800,
			Attack:               340,
			Defense:              250,
			Agility:              130,
			ExperienceReward:     3400,
			GoldReward:           5500,
			DropItemIDs:          []string{"crystal-01"},
			FirstClearExpBonus:   5000,
			FirstClearGoldBonus:  10000,
			SmallMedalReward:     2,
			FirstClearMedalBonus: 3,
			DailyEntryLimit:      DefaultDailyEntryLimit,
		},
		{
			ID:                   "king-07",
			Tier:                 7,
			Name:                 "クリスタル・ドラゴン",
			Title:                "光彩の巨竜",
			MinLevel:             75,
			HP:                   3800,
			Attack:               430,
			Defense:              320,
			Agility:              165,
			ExperienceReward:     4600,
			GoldReward:           7500,
			DropItemIDs:          []string{"crystal-02"},
			FirstClearExpBonus:   7000,
			FirstClearGoldBonus:  14000,
			SmallMedalReward:     3,
			FirstClearMedalBonus: 4,
			DailyEntryLimit:      DefaultDailyEntryLimit,
		},
		{
			ID:                   "king-08",
			Tier:                 8,
			Name:                 "ダークネス・ベヒモス",
			Title:                "深淵の殲滅者",
			MinLevel:             85,
			HP:                   5000,
			Attack:               530,
			Defense:              400,
			Agility:              200,
			ExperienceReward:     6000,
			GoldReward:           10000,
			DropItemIDs:          []string{"crystal-03"},
			FirstClearExpBonus:   9500,
			FirstClearGoldBonus:  20000,
			SmallMedalReward:     3,
			FirstClearMedalBonus: 4,
			DailyEntryLimit:      DefaultDailyEntryLimit,
		},
		{
			ID:                   "king-09",
			Tier:                 9,
			Name:                 "アビス・ルーラー",
			Title:                "黄泉の帝王",
			MinLevel:             95,
			HP:                   6500,
			Attack:               640,
			Defense:              490,
			Agility:              245,
			ExperienceReward:     8000,
			GoldReward:           14000,
			DropItemIDs:          []string{"orb-dark"},
			FirstClearExpBonus:   13000,
			FirstClearGoldBonus:  30000,
			SmallMedalReward:     4,
			FirstClearMedalBonus: 5,
			DailyEntryLimit:      DefaultDailyEntryLimit,
		},
		{
			ID:                   "king-10",
			Tier:                 10,
			Name:                 "全てを無に還す者",
			Title:                "終焉の破壊神",
			MinLevel:             99,
			HP:                   8500,
			Attack:               760,
			Defense:              590,
			Agility:              300,
			ExperienceReward:     12000,
			GoldReward:           20000,
			DropItemIDs:          []string{"orb-light"},
			FirstClearExpBonus:   20000,
			FirstClearGoldBonus:  50000,
			SmallMedalReward:     5,
			FirstClearMedalBonus: 5,
			DailyEntryLimit:      DefaultDailyEntryLimit,
		},
		{
			ID:                   "king-world",
			Tier:                 99,
			Name:                 "太古の創世神",
			Title:                "天界の守護龍神",
			MinLevel:             99,
			HP:                   12000,
			Attack:               920,
			Defense:              720,
			Agility:              360,
			ExperienceReward:     25000,
			GoldReward:           50000,
			DropItemIDs:          []string{"orb-rainbow"},
			FirstClearExpBonus:   50000,
			FirstClearGoldBonus:  100000,
			SmallMedalReward:     10,
			FirstClearMedalBonus: 20,
			DailyEntryLimit:      DefaultDailyEntryLimit,
		},
	}
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

func (s *Service) GetCharacterRecord(ctx context.Context, characterID string) (CharacterBossRecord, error) {
	if characterID == "" {
		return CharacterBossRecord{}, ErrCharacterNotFound
	}
	rec, err := s.repo.GetOrCreateRecord(ctx, characterID)
	if err != nil {
		return CharacterBossRecord{}, err
	}
	rec.ResetDailyAttemptsIfExpired(time.Now().UTC())
	return rec, nil
}

func (s *Service) GetHistory(ctx context.Context, characterID string, limit int) ([]BossChallengeHistory, error) {
	if characterID == "" {
		return nil, ErrCharacterNotFound
	}
	if limit <= 0 {
		limit = 20
	}
	return s.repo.GetHistory(ctx, characterID, limit)
}

func (s *Service) GetLeaderboard(ctx context.Context, limit int) ([]BossLeaderboardEntry, error) {
	if limit <= 0 {
		limit = 50
	}
	return s.repo.GetLeaderboard(ctx, limit)
}

func (s *Service) ChallengeBoss(ctx context.Context, characterID, bossID string) (ChallengeResult, error) {
	if characterID == "" {
		return ChallengeResult{}, ErrCharacterNotFound
	}
	if bossID == "" {
		return ChallengeResult{}, ErrInvalidBossID
	}

	boss, ok := s.bossMap[bossID]
	if !ok {
		return ChallengeResult{}, ErrBossNotFound
	}

	char, err := s.characterRepo.FindByID(ctx, characterID)
	if err != nil {
		return ChallengeResult{}, ErrCharacterNotFound
	}

	// 1. Validate level requirement
	if char.Level < boss.MinLevel {
		return ChallengeResult{}, ErrLevelRequirementNotMet
	}

	// 2. Validate boss record & prerequisites
	rec, err := s.repo.GetOrCreateRecord(ctx, characterID)
	if err != nil {
		return ChallengeResult{}, err
	}

	now := time.Now().UTC()
	rec.ResetDailyAttemptsIfExpired(now)

	if boss.Tier > 1 {
		prereqTier := boss.Tier - 1
		if boss.Tier == 99 {
			prereqTier = 10
		}
		if rec.HighestTierCleared < prereqTier {
			return ChallengeResult{}, ErrPrerequisiteNotMet
		}
	}

	// 3. Validate daily limit
	if rec.DailyAttemptsUsed >= boss.DailyEntryLimit {
		return ChallengeResult{}, ErrDailyAttemptsExhausted
	}

	// Consume 1 daily attempt
	rec.DailyAttemptsUsed++
	rec.LastChallengedAt = &now

	// 4. Combat Resolution
	req := corebattle.Request{
		Participants: []corebattle.Participant{
			corebattle.NewParticipantFromCharacter(char),
			corebattle.MustNewParticipant(boss.ID, boss.HP, boss.Attack, boss.Defense),
		},
	}

	battleResult, err := s.battleEngine.Resolve(req)
	if err != nil {
		return ChallengeResult{}, fmt.Errorf("battle resolution failed: %w", err)
	}

	outcome := battleResult.Outcome
	var expGained, goldGained, medalsGained int
	var rewardItemID string
	isFirstClear := false
	var rewardItemInstance *coreitem.Instance

	if outcome == corebattle.OutcomeWin && battleResult.WinnerID == char.ID {
		isFirstClear = rec.HighestTierCleared < boss.Tier
		expGained = boss.ExperienceReward
		goldGained = boss.GoldReward
		medalsGained = boss.SmallMedalReward

		if isFirstClear {
			expGained += boss.FirstClearExpBonus
			goldGained += boss.FirstClearGoldBonus
			medalsGained += boss.FirstClearMedalBonus
			if rec.FirstClearedAt == nil {
				rec.FirstClearedAt = &now
			}
			if boss.Tier > rec.HighestTierCleared {
				rec.HighestTierCleared = boss.Tier
			}
		}

		rec.TotalBossDefeats++

		if len(boss.DropItemIDs) > 0 {
			rewardItemID = boss.DropItemIDs[0]
			itemInstanceID := id.New()
			rewardItemInstance = &coreitem.Instance{
				ID:               itemInstanceID,
				DefinitionID:     rewardItemID,
				Quantity:         1,
				EnhancementLevel: 0,
			}
		}

		// Apply EXP, Gold, and SmallMedals to character
		if expGained > 0 {
			_, _ = progression.ApplyExperience(&char, expGained)
		}
		_ = char.AddMoney(goldGained)
		_ = char.AddSmallMedals(medalsGained)
	}

	historyID := id.New()

	history := BossChallengeHistory{
		ID:                historyID,
		CharacterID:       char.ID,
		BossID:            boss.ID,
		Tier:              boss.Tier,
		Outcome:           outcome,
		Turns:             battleResult.Turns,
		RewardExp:         expGained,
		RewardGold:        goldGained,
		RewardSmallMedals: medalsGained,
		RewardItemID:      rewardItemID,
		IsFirstClear:      isFirstClear,
		CreatedAt:         now,
	}

	if err := s.repo.RecordChallenge(ctx, history, rec, char, rewardItemInstance); err != nil {
		return ChallengeResult{}, fmt.Errorf("record boss challenge: %w", err)
	}

	if outcome == corebattle.OutcomeWin && battleResult.WinnerID == char.ID {
		if s.victoryBanquetHook != nil {
			_ = s.victoryBanquetHook(ctx, boss.ID, boss.Name, char.ID, char.Name, boss.Tier)
		}
		if s.victoryHook != nil {
			_ = s.victoryHook(ctx, char.ID, boss.ID, boss.Tier)
		}
	}

	return ChallengeResult{
		BattleResult:      battleResult,
		Outcome:           outcome,
		ExperienceReward:  expGained,
		GoldReward:        goldGained,
		SmallMedalsReward: medalsGained,
		ItemRewardID:      rewardItemID,
		IsFirstClear:      isFirstClear,
		UpdatedRecord:     rec,
	}, nil
}
