package adventure

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/witchcraze/party2re/internal/battle"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	"github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/core/progression"
	"github.com/witchcraze/party2re/internal/core/random"
	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/runhost"
)

const (
	StarterAdventure = "stage-00"
	AdventureReward  = 20
	AdventureEnemyID = "starter-opponent"
)

var (
	ErrNotFound             = errors.New("adventure not found")
	ErrUnsupportedReward    = errors.New("adventure reward type is unsupported")
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
	PostAdventureMessages map[string]string
	MonsterTames          map[string][]battle.MonsterTameResult `json:"-"`
	ID                    string
	CharacterID           string
	Type                  string
	StageID               string
	Season                string
	SeasonalSubID         int
	MonsterID             string
	StartedAt             time.Time
	FloorsCleared         int
	IsCleared             bool
	PartySize             int
	ExperienceReward      int
	BattleResult          corebattle.Result
	RewardItemID          string `json:"-"` // Retained for previously stored adventure history.
	RewardItemQuantity    int    `json:"-"`
	Resolved              bool
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

type InventoryRepository interface {
	FindByCharacterID(ctx context.Context, characterID string) (coreinventory.Inventory, error)
	Save(ctx context.Context, value coreinventory.Inventory) error
}

type Service struct {
	legacyEntry        func(context.Context, []string, func(context.Context) error) error
	adventures         Repository
	characters         CharacterRepository
	inventories        InventoryRepository
	stages             *StageCatalog
	monsters           *MonsterCatalog
	battle             corebattle.Resolver
	logger             Logger
	clock              Clock
	nowFunc            func() time.Time
	postAdventureHook  PostAdventureHook
	unsealHook         UnsealHook
	participantBuilder ParticipantBuilder
	battleSettler      PostBattleSettler
	blessingProvider   BlessingProvider
	timer              TimerService
	txProvider         TransactionProvider
}

func (s *Service) runInTx(ctx context.Context, fn func(txCtx context.Context) error) error {
	if s.txProvider != nil {
		return s.txProvider.RunInTx(ctx, fn)
	}
	return fn(ctx)
}

func (s *Service) now() time.Time {
	if s.nowFunc != nil {
		return s.nowFunc()
	}
	if s.clock != nil {
		return s.clock.Now()
	}
	return time.Now()
}

func (s *Service) SetBlessingProvider(provider BlessingProvider) {
	s.blessingProvider = provider
}

func (s *Service) SetTimerService(timer TimerService) {
	s.timer = timer
}

func (s *Service) SetNowFunc(nowFunc func() time.Time) {
	s.nowFunc = nowFunc
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
	opts ...Option,
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
	s := &Service{
		adventures:  adventures,
		characters:  characters,
		inventories: inventories,
		stages:      stages,
		monsters:    monsters,
		battle:      battle,
		logger:      logger,
		clock:       clock,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s, nil
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
		if err := s.stages.CanAccessStageAt(char, stageID, s.now()); err != nil {
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
	adv, err := s.adventures.FindByID(ctx, advID)
	if err != nil {
		return Adventure{}, err
	}
	adv.PostAdventureMessages = crawlRes.PostAdventureMessages
	adv.MonsterTames = crawlRes.MonsterTames
	return adv, nil
}

// ExecuteCrawl runs the authentic 10-floor dungeon crawl and Floor 11 treasure room (vs_monster.cgi).
func (s *Service) executeCrawl(ctx context.Context, req DungeonCrawlRequest) (DungeonCrawlResult, error) {
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

	if err := s.stages.CanAccessStageAt(characters[0], stage.ID, s.now()); err != nil {
		return DungeonCrawlResult{}, err
	}

	var lockToken string
	var lockAcquired bool
	var committed bool

	defer func() {
		if !committed && lockAcquired && s.timer != nil && len(characters) > 0 {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			//lint:ignore error-swallow compensatory unlock on aborted crawl
			_, _ = s.timer.ReleaseLockWithToken(cleanupCtx, timer.CategoryDungeonOnce, characters[0].ID, lockToken)
		}
	}()

	if stage.DailyOnce && s.timer != nil && len(characters) > 0 {
		lockToken = id.New()
		ok, err := s.timer.TryLockWithToken(ctx, timer.CategoryDungeonOnce, characters[0].ID, lockToken, 0)
		if err != nil {
			return DungeonCrawlResult{}, err
		}
		if !ok {
			return DungeonCrawlResult{}, fmt.Errorf("%w: %sはすでに本日の限定ダンジョン探索を終えています。自宅で睡眠をとると再度挑戦できます", ErrDailyOnceAlreadyUsed, characters[0].Name)
		}
		lockAcquired = true
	}

	rng := req.Rng
	if rng == nil {
		rng = random.Intn
	}

	stage = stage.SelectSeason(rng)

	var pbr corebattle.PartyBattleResolver = corebattle.Engine{}
	if r, ok := s.battle.(partyBattleResolver); ok {
		pbr = r
	}

	var participants []corebattle.Participant
	if s.participantBuilder != nil {
		participants = make([]corebattle.Participant, len(characters))
		for i, c := range characters {
			p, err := s.participantBuilder.BuildParticipant(ctx, c.ID)
			if err != nil {
				return DungeonCrawlResult{}, err
			}
			participants[i] = p
		}
	}

	session, err := NewCrawlSessionWithParticipants(stage, characters, participants, req.Rng)
	if err != nil {
		return DungeonCrawlResult{}, err
	}
	if req.RngFloat != nil {
		session.RngFloat = req.RngFloat
	}
	if req.Mode != "" {
		if err := session.SetMode(req.Mode); err != nil {
			return DungeonCrawlResult{}, err
		}
	}
	session.nowFunc = s.now

	if err := session.LoadBlessings(ctx, s.blessingProvider); err != nil {
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
		// Only open base allowance (1 box per surviving participant).
		// Additional chest opening using runhost.ItemTreasurePocket requires explicit player choice (#948/#1330).
		for _, c := range characters {
			_, _ = session.ExamineTreasure(c.ID)
		}
	}

	result := session.Result()
	now := s.now()
	if err := s.runInTx(ctx, func(txCtx context.Context) error {
		if err := s.settlePostBattle(txCtx, req, &result); err != nil {
			return err
		}

		// Persist adventure record for each participating character
		for _, c := range characters {
			reward := result.RewardForCharacter(c.ID)
			advIDHash := sha256.Sum256([]byte(fmt.Sprintf("solo:%s:%s", session.ID, c.ID)))
			advID := hex.EncodeToString(advIDHash[:16])
			result.AdventureIDs[c.ID] = advID

			winnerID := ""
			if result.Outcome == corebattle.OutcomeWin {
				winnerID = c.ID
			}

			adv := Adventure{
				ID:               advID,
				CharacterID:      c.ID,
				Type:             session.StageID,
				StageID:          session.StageID,
				Season:           session.Stage.Season,
				SeasonalSubID:    SeasonToSubID(session.Stage.Season),
				StartedAt:        now,
				FloorsCleared:    result.FloorsCleared,
				IsCleared:        result.StageCleared,
				PartySize:        len(characters),
				ExperienceReward: reward.Experience,
				Resolved:         true,
				BattleResult: corebattle.Result{
					Outcome:  result.Outcome,
					WinnerID: winnerID,
					Turns:    result.TotalTurns,
					Reward:   reward,
				},
			}

			if s.adventures != nil {
				if err := s.adventures.Save(txCtx, adv); err != nil {
					return fmt.Errorf("saving adventure record: %w", err)
				}
			}

			// Apply fallback experience, gold, crystal, fatigue, and surviving HP/MP if battleSettler is not configured
			if s.battleSettler == nil {
				remMP := -1
				if mp, ok := result.ParticipantMPs[c.ID]; ok && mp >= 0 {
					remMP = mp
				}
				if hp, ok := result.ParticipantHPs[c.ID]; ok {
					c.ApplyCombatSurvival(hp, remMP, hp <= 0)
				} else if remMP >= 0 {
					c.Stats.MP = remMP
					c.Stats.ClampVitality()
				}
				c.AddTired(result.EffectiveExitFatigue(req.Mode))

				if result.Outcome == corebattle.OutcomeWin || result.Mode != CrawlModeTry {
					if reward.Experience > 0 {
						if _, err := progression.ApplyExperience(&c, reward.Experience); err != nil {
							return fmt.Errorf("adding experience for character %s: %w", c.ID, err)
						}
					}
					if reward.Currency > 0 {
						if err := c.AddMoney(reward.Currency); err != nil {
							return fmt.Errorf("adding gold for character %s: %w", c.ID, err)
						}
					}
					if reward.Crystals > 0 {
						if err := c.AddCrystal(reward.Crystals); err != nil {
							return fmt.Errorf("adding crystals for character %s: %w", c.ID, err)
						}
					}
				}

				if updater, ok := s.characters.(CharacterUpdater); ok {
					if err := updater.Update(txCtx, c); err != nil {
						return fmt.Errorf("updating character %s: %w", c.ID, err)
					}
				}
			}
		}
		return nil
	}); err != nil {
		return DungeonCrawlResult{}, err
	}
	committed = true

	for _, c := range characters {
		if s.postAdventureHook != nil {
			if message, err := s.postAdventureHook(ctx, c.ID); err != nil {
				s.logger.Warn("post adventure hook failed for character %s: %v", c.ID, err)
			} else if message != "" {
				if result.PostAdventureMessages == nil {
					result.PostAdventureMessages = make(map[string]string)
				}
				result.PostAdventureMessages[c.ID] = message
			}
		}
	}

	return result, nil
}

// Get retrieves an adventure record by its ID.
func (s *Service) Get(ctx context.Context, id string) (Adventure, error) {
	return s.adventures.FindByID(ctx, id)
}
