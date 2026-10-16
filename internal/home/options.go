package home

import (
	"context"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/random"
	"github.com/witchcraze/party2re/internal/economy"
)

// CharacterUpdater abstracts updating full character state.
type CharacterUpdater interface {
	FindByIDForUpdate(ctx context.Context, id string) (corecharacter.Character, error)
	Update(ctx context.Context, char corecharacter.Character) error
}

// TimerLockService manages sleep timers and token/claim locks.
type TimerLockService interface {
	SetLock(ctx context.Context, category, targetID string, duration time.Duration) error
	TryLockWithToken(ctx context.Context, category, targetID, token string, duration time.Duration) (bool, error)
	RenewLock(ctx context.Context, category, targetID, token string, duration time.Duration) (bool, error)
	IsLocked(ctx context.Context, category, targetID string) (bool, error)
	GetRemainingLock(ctx context.Context, category, targetID string) (time.Duration, error)
	ReleaseLock(ctx context.Context, category, targetID string) error
	ReleaseLockWithToken(ctx context.Context, category, targetID, token string) (bool, error)
	ReleaseLockWithClaim(ctx context.Context, category, targetID, claimCategory, claimToken string) (bool, error)
}

// TimerQuotaService manages daily quota resets and claim-guarded resets.
type TimerQuotaService interface {
	ResetDailyQuota(ctx context.Context, action, targetID string) error
	ResetDailyQuotaWithClaim(ctx context.Context, action, targetID, claimCategory, claimToken string) (bool, error)
	ResetDungeonOnceWithClaim(ctx context.Context, targetID, claimCategory, claimToken string) (bool, error)
}

// TimerService manages sleep timers, locks, and daily quotas.
type TimerService interface {
	TimerLockService
	TimerQuotaService
}

// FullnessResetter resets character eating/fullness state upon rest.
type FullnessResetter interface {
	ResetFullness(ctx context.Context, characterID string) error
}

// BlessingCleaner clears chapel daily prayers and blessings upon rest.
type BlessingCleaner interface {
	ClearBlessing(ctx context.Context, characterID string) error
}

// AlchemyCompleter completes ongoing alchemy synthesis upon rest.
type AlchemyCompleter interface {
	CompleteOngoingSynthesis(ctx context.Context, characterID string) error
}

// CostumeResetter resets rented costume state upon rest.
type CostumeResetter interface {
	ResetCostume(ctx context.Context, characterID string) error
}

// CostumeClaimResetter resets rented costume state with atomic claim verification.
type CostumeClaimResetter interface {
	ResetCostumeWithClaim(ctx context.Context, characterID, claimCategory, claimToken string) (bool, error)
}

// JobStateRestorer restores character_jobs.current_job_id upon rest when reverting JobMemory.
type JobStateRestorer interface {
	RestoreActiveJob(ctx context.Context, characterID, jobID string) error
}

// OnlineCounter counts currently logged in players for sleep duration scaling.
type OnlineCounter interface {
	GetOnlineCount(ctx context.Context) (int, error)
}

// TransactionRunner defines the interface for running atomic transactions.
type TransactionRunner interface {
	ExecuteTransaction(ctx context.Context, req economy.TransactionRequest, fn economy.TransactionCallback) (*economy.TransactionResult, error)
}

type HomePetReader interface {
	ListHomePets(ctx context.Context, characterID string) ([]HomePet, error)
}

func WithHomePetReader(r HomePetReader) ServiceOption {
	return func(s *Service) { s.petReader = r }
}

type ServiceOption func(*Service)

func WithVisitorLimiter(_ Limiter, _ time.Duration) ServiceOption {
	return func(s *Service) {}
}

func WithNowFunc(fn func() time.Time) ServiceOption {
	return func(s *Service) {
		s.nowFunc = fn
	}
}

func WithRNG(rng random.Generator) ServiceOption {
	return func(s *Service) {
		s.rng = rng
	}
}

func WithTimer(t TimerService) ServiceOption {
	return func(s *Service) {
		s.timer = t
	}
}

func WithJobStateRestorer(r JobStateRestorer) ServiceOption {
	return func(s *Service) {
		s.jobRestorer = r
	}
}

func WithCharacterUpdater(u CharacterUpdater) ServiceOption {
	return func(s *Service) {
		s.charUpdater = u
	}
}

func WithGuildPoints(g GuildPointsRegistrar) ServiceOption {
	return func(s *Service) {
		s.guildPoints = g
	}
}

func WithInventoryManager(i InventoryManager) ServiceOption {
	return func(s *Service) {
		s.invMgr = i
	}
}

func WithDepotManager(d DepotManager) ServiceOption {
	return func(s *Service) {
		s.depotMgr = d
	}
}

func WithItemCatalog(c ItemCatalog) ServiceOption {
	return func(s *Service) {
		s.catalog = c
	}
}

func WithFullnessResetter(f FullnessResetter) ServiceOption {
	return func(s *Service) {
		s.fullness = f
	}
}

func WithBlessingCleaner(b BlessingCleaner) ServiceOption {
	return func(s *Service) {
		s.chapel = b
	}
}

func WithAlchemyCompleter(a AlchemyCompleter) ServiceOption {
	return func(s *Service) {
		s.alchemy = a
	}
}

func WithCostumeResetter(c CostumeResetter) ServiceOption {
	return func(s *Service) {
		s.costume = c
	}
}

func WithOnlineCounter(c OnlineCounter) ServiceOption {
	return func(s *Service) {
		s.onlineCounter = c
	}
}

func WithBaseSleepDuration(d time.Duration) ServiceOption {
	return func(s *Service) {
		s.baseSleepDuration = d
	}
}

// WithWakeClaimTTL configures the safety TTL and renewal interval for Wake exclusive claims.
func WithWakeClaimTTL(ttl, renewInterval time.Duration) ServiceOption {
	return func(s *Service) {
		s.wakeClaimTTL = ttl
		s.wakeClaimRenewInterval = renewInterval
	}
}

// WithTransactionRunner sets the transaction runner for atomic operations.
func WithTransactionRunner(runner TransactionRunner) ServiceOption {
	return func(s *Service) {
		s.runner = runner
	}
}

// WithEconomy sets the economy service as the transaction runner.
func WithEconomy(eco *economy.Service) ServiceOption {
	return func(s *Service) {
		s.runner = eco
	}
}

// SetFullnessResetter registers a cross-domain fullness reset hook.
func (s *Service) SetFullnessResetter(f FullnessResetter) { s.fullness = f }

// SetBlessingCleaner registers a cross-domain chapel blessing cleaner hook.
func (s *Service) SetBlessingCleaner(b BlessingCleaner) { s.chapel = b }

// SetAlchemyCompleter registers a cross-domain alchemy synthesis completer hook.
func (s *Service) SetAlchemyCompleter(a AlchemyCompleter) { s.alchemy = a }

// SetCostumeResetter registers a cross-domain costume reset hook.
func (s *Service) SetCostumeResetter(c CostumeResetter) { s.costume = c }

// SetJobStateRestorer registers a cross-domain job state restorer hook.
func (s *Service) SetJobStateRestorer(r JobStateRestorer) { s.jobRestorer = r }
