package battle

import (
	"context"
	"errors"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreequipment "github.com/witchcraze/party2re/internal/core/equipment"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	"github.com/witchcraze/party2re/internal/core/job"
	"github.com/witchcraze/party2re/internal/core/skill"
	"github.com/witchcraze/party2re/internal/custom_skill"
	"github.com/witchcraze/party2re/internal/depot"
	"github.com/witchcraze/party2re/internal/economy"
)

var (
	ErrCharacterNotFound     = errors.New("character not found")
	ErrInvalidRequest        = errors.New("battle request is invalid")
	ErrNilRequest            = errors.New("request is nil")
	ErrInventoryNotFound     = errors.New("inventory not found")
	ErrEquipmentNotFound     = errors.New("equipment not found")
	ErrInsufficientResources = errors.New("insufficient character resources")
)

// CharacterRepository defines persistence operations on Character (Rank 2).
type CharacterRepository interface {
	FindByID(ctx context.Context, id string) (corecharacter.Character, error)
	FindByIDForUpdate(ctx context.Context, id string) (corecharacter.Character, error)
	Update(ctx context.Context, character corecharacter.Character) error
}

// InventoryRepository defines persistence operations on Inventory (Rank 3).
type InventoryRepository interface {
	FindByCharacterID(ctx context.Context, characterID string) (coreinventory.Inventory, error)
	FindByCharacterIDForUpdate(ctx context.Context, characterID string) (coreinventory.Inventory, error)
	Save(ctx context.Context, inventory coreinventory.Inventory) error
}

// EquipmentRepository defines persistence operations on Equipment (Rank 3).
type EquipmentRepository interface {
	FindByCharacterID(ctx context.Context, characterID string) (coreequipment.Equipment, error)
	Save(ctx context.Context, value coreequipment.Equipment) error
}

// DepotRepository defines persistence operations on Depot (Rank 5).
type DepotRepository interface {
	FindByCharacterID(ctx context.Context, characterID string) (depot.Depot, error)
	FindByCharacterIDForUpdate(ctx context.Context, characterID string) (depot.Depot, error)
	Save(ctx context.Context, value depot.Depot) error
}

// CustomSkillRepository defines access to custom skill definitions.
type CustomSkillRepository interface {
	FindCustomSkill(ctx context.Context, characterID string) (*custom_skill.CustomSkill, error)
}

// SkillProvider defines access to job skills.
type SkillProvider interface {
	SkillsForJob(jobID string) []skill.Definition
}

// TransactionProvider executes a callback within an ambient or new transaction.
type TransactionProvider interface {
	RunInTx(ctx context.Context, fn func(ctx context.Context) error) error
}

// BattleEngine defines the core party battle resolver.
type BattleEngine interface {
	ResolvePartyBattle(req corebattle.PartyBattleRequest) (corebattle.PartyBattleResult, error)
}

// Service orchestrates battle participation mapping and post-battle state application.
type Service struct {
	charRepo      CharacterRepository
	invRepo       InventoryRepository
	equipRepo     EquipmentRepository
	depotRepo     DepotRepository
	jobProvider   job.DefinitionProvider
	skillProvider SkillProvider
	customSkills  CustomSkillRepository
	txRunner      economy.TransactionRunner
	txProvider    TransactionProvider
	engine        BattleEngine
	rng           corecharacter.RandomSource
	maxInvCap     int
}

// Option configures Service dependencies.
type Option func(*Service)

// WithCharacterRepository sets the CharacterRepository.
func WithCharacterRepository(repo CharacterRepository) Option {
	return func(s *Service) {
		s.charRepo = repo
	}
}

// WithInventoryRepository sets the InventoryRepository.
func WithInventoryRepository(repo InventoryRepository) Option {
	return func(s *Service) {
		s.invRepo = repo
	}
}

// WithEquipmentRepository sets the EquipmentRepository.
func WithEquipmentRepository(repo EquipmentRepository) Option {
	return func(s *Service) {
		s.equipRepo = repo
	}
}

// WithDepotRepository sets the DepotRepository.
func WithDepotRepository(repo DepotRepository) Option {
	return func(s *Service) {
		s.depotRepo = repo
	}
}

// WithJobDefinitionProvider sets the job definition provider.
func WithJobDefinitionProvider(provider job.DefinitionProvider) Option {
	return func(s *Service) {
		s.jobProvider = provider
	}
}

// WithSkillProvider sets the job skill provider.
func WithSkillProvider(provider SkillProvider) Option {
	return func(s *Service) {
		s.skillProvider = provider
	}
}

// WithCustomSkillRepository sets the custom skill repository.
func WithCustomSkillRepository(repo CustomSkillRepository) Option {
	return func(s *Service) {
		s.customSkills = repo
	}
}

// WithTransactionRunner sets the economy.TransactionRunner for single-character transactions.
func WithTransactionRunner(runner economy.TransactionRunner) Option {
	return func(s *Service) {
		s.txRunner = runner
	}
}

// WithTransactionProvider sets the TransactionProvider for multi-aggregate / party transactions.
func WithTransactionProvider(provider TransactionProvider) Option {
	return func(s *Service) {
		s.txProvider = provider
	}
}

// WithBattleEngine sets the underlying core BattleEngine.
func WithBattleEngine(engine BattleEngine) Option {
	return func(s *Service) {
		s.engine = engine
	}
}

// WithRandomSource sets the random number generator.
func WithRandomSource(rng corecharacter.RandomSource) Option {
	return func(s *Service) {
		s.rng = rng
	}
}

// WithMaxInventoryCapacity overrides the default inventory capacity threshold before depot transfer occurs.
func WithMaxInventoryCapacity(cap int) Option {
	return func(s *Service) {
		if cap > 0 {
			s.maxInvCap = cap
		}
	}
}

// NewService creates a new battle adapter Service.
func NewService(opts ...Option) *Service {
	s := &Service{
		engine: corebattle.Engine{},
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *Service) runInTx(ctx context.Context, fn func(txCtx context.Context) error) error {
	if s.txProvider != nil {
		return s.txProvider.RunInTx(ctx, fn)
	}
	return fn(ctx)
}
