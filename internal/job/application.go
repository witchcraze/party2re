package job

import (
	"context"
	"errors"
	"fmt"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreequipment "github.com/witchcraze/party2re/internal/core/equipment"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	corejob "github.com/witchcraze/party2re/internal/core/job"
	"github.com/witchcraze/party2re/internal/core/skill"
	"github.com/witchcraze/party2re/internal/economy"
)

type Repository interface {
	Save(ctx context.Context, value corejob.CharacterJob) error
	FindByCharacterID(ctx context.Context, characterID string) (corejob.CharacterJob, error)
}

type CharacterRepository interface {
	FindByID(ctx context.Context, id string) (corecharacter.Character, error)
	Update(ctx context.Context, value corecharacter.Character) error
}

type InventoryRepository interface {
	FindByCharacterID(ctx context.Context, characterID string) (coreinventory.Inventory, error)
	Save(ctx context.Context, value coreinventory.Inventory) error
}

type EquipmentRepository interface {
	FindByCharacterID(ctx context.Context, characterID string) (coreequipment.Equipment, error)
	Save(ctx context.Context, value coreequipment.Equipment) error
}

type SkillProvider interface {
	SkillsForJob(jobID string) []skill.Definition
}

// NewsPublisher defines announcement publication contract.
type NewsPublisher interface {
	PublishNews(ctx context.Context, category, title, content, author string, publishedAt time.Time) error
}

type NewsPublisherFunc func(ctx context.Context, category, title, content, author string, publishedAt time.Time) error

func (f NewsPublisherFunc) PublishNews(ctx context.Context, category, title, content, author string, publishedAt time.Time) error {
	return f(ctx, category, title, content, author, publishedAt)
}

// GuildPointAwarder defines optional guild point award operations (job_change.cgi:195).
type GuildPointAwarder interface {
	AddGuildPoints(ctx context.Context, characterID string, points int) error
}

// CostumeResetter resets rented costume state upon job change (goods.cgi:28).
type CostumeResetter interface {
	ResetCostume(ctx context.Context, characterID string) error
}

// JobChangeTracker tracks job change events for weekly leaderboards (week_ranking.cgi).
type JobChangeTracker interface {
	RecordJobChange(ctx context.Context, characterID string) error
}

// LegendInductor defines permanent Hall of Fame induction contract (legend.cgi).
type LegendInductor interface {
	RecordLegend(ctx context.Context, category, characterID string) error
}

type LegendInductorFunc func(ctx context.Context, category, characterID string) error

func (f LegendInductorFunc) RecordLegend(ctx context.Context, category, characterID string) error {
	return f(ctx, category, characterID)
}

type Service struct {
	repository     Repository
	catalog        *corejob.Catalog
	characters     CharacterRepository
	inventories    InventoryRepository
	economy        *economy.Service
	skills         SkillProvider
	news           NewsPublisher
	futureMemories FutureMemoryRepository
	guildPoints    GuildPointAwarder
	costume        CostumeResetter
	jobTracker     JobChangeTracker
	equipment      EquipmentRepository
	legend         LegendInductor
}

type Option func(*Service)

func WithCatalog(catalog *corejob.Catalog) Option {
	return func(s *Service) {
		s.catalog = catalog
	}
}

func WithJobChangeTracker(tracker JobChangeTracker) Option {
	return func(s *Service) {
		s.jobTracker = tracker
	}
}

func WithCharacterRepository(characters CharacterRepository) Option {
	return func(s *Service) {
		s.characters = characters
	}
}

func WithInventoryRepository(inventories InventoryRepository) Option {
	return func(s *Service) {
		s.inventories = inventories
	}
}

func WithEquipmentRepository(equipment EquipmentRepository) Option {
	return func(s *Service) {
		s.equipment = equipment
	}
}

func WithEconomy(value *economy.Service) Option {
	return func(s *Service) {
		s.economy = value
	}
}

func WithSkillProvider(provider SkillProvider) Option {
	return func(s *Service) {
		s.skills = provider
	}
}

func WithNewsPublisher(news NewsPublisher) Option {
	return func(s *Service) {
		s.news = news
	}
}

// WithGuildPointAwarder sets the optional guild point awarder (job_change.cgi:195).
func WithGuildPointAwarder(gpa GuildPointAwarder) Option {
	return func(s *Service) {
		s.guildPoints = gpa
	}
}

// WithCostumeResetter sets the optional costume rental resetter on job change (goods.cgi:28).
func WithCostumeResetter(c CostumeResetter) Option {
	return func(s *Service) {
		s.costume = c
	}
}

// WithLegendInductor sets the optional Hall of Fame legend inductor (legend.cgi).
func WithLegendInductor(inductor LegendInductor) Option {
	return func(s *Service) {
		s.legend = inductor
	}
}

func NewService(repository Repository, opts ...Option) (*Service, error) {
	if repository == nil {
		return nil, errors.New("job repository is nil")
	}
	s := &Service{repository: repository}
	for _, opt := range opts {
		opt(s)
	}
	if s.catalog == nil {
		s.catalog, _ = corejob.InitialCatalog()
	}
	return s, nil
}

// SetCostumeResetter registers the costume reset hook called upon job change.
func (s *Service) SetCostumeResetter(c CostumeResetter) {
	s.costume = c
}

// SetJobChangeTracker registers the weekly job change tracker called upon job change.
func (s *Service) SetJobChangeTracker(tracker JobChangeTracker) {
	s.jobTracker = tracker
}

// SetLegendInductor registers the Hall of Fame legend inductor called upon all job mastery.
func (s *Service) SetLegendInductor(inductor LegendInductor) {
	s.legend = inductor
}

func (s *Service) ListDefinitions() []corejob.Definition {
	if s.catalog == nil {
		return nil
	}
	return s.catalog.Definitions()
}

func (s *Service) GetDefinition(id string) (corejob.Definition, error) {
	if s.catalog == nil {
		return corejob.Definition{}, corejob.ErrDefinitionNotFound
	}
	return s.catalog.FindByID(id)
}

func (s *Service) Master(ctx context.Context, characterID string, jobID string) (corejob.CharacterJob, error) {
	state, err := s.repository.FindByCharacterID(ctx, characterID)
	if err != nil {
		return corejob.CharacterJob{}, err
	}
	if s.characters != nil {
		charName := ""
		charSP := 0
		if char, err := s.characters.FindByID(ctx, characterID); err == nil {
			charName = char.Name
			charSP = char.SP
		}
		if def, err := s.GetDefinition(jobID); err == nil {
			if !state.RecordMastery(jobID, charSP, s.masterySP(def)) {
				state.Master(jobID)
			}
		} else {
			state.Master(jobID)
		}
		s.checkAndNotifyCompletion(ctx, charName, &state)
	} else {
		state.Master(jobID)
		s.checkAndNotifyCompletion(ctx, "", &state)
	}
	if err := s.repository.Save(ctx, state); err != nil {
		return corejob.CharacterJob{}, err
	}
	return state, nil
}

func (s *Service) CheckAndApplyMastery(ctx context.Context, characterID string, level int) (bool, error) {
	state, err := s.repository.FindByCharacterID(ctx, characterID)
	if err != nil {
		return false, err
	}
	sp := level
	charName := ""
	if s.characters != nil {
		char, err := s.characters.FindByID(ctx, characterID)
		if err != nil {
			return false, err
		}
		sp = char.SP
		charName = char.Name
	}
	required := s.masterySPForJob(state.CurrentJobID)
	if required <= 0 || !state.RecordMastery(state.CurrentJobID, sp, required) {
		return false, nil
	}
	s.checkAndNotifyCompletion(ctx, charName, &state)
	if err := s.repository.Save(ctx, state); err != nil {
		return false, err
	}
	return true, nil
}

func (s *Service) checkAndNotifyCompletion(ctx context.Context, charName string, state *corejob.CharacterJob) {
	if state == nil {
		return
	}
	if state.CheckAndSetAllJobsMastered() {
		if s.news != nil {
			displayName := charName
			if displayName == "" {
				displayName = state.CharacterID
			}
			_ = s.news.PublishNews(
				ctx,
				"job",
				"全ジョブコンプリート",
				fmt.Sprintf("%sが全ての職業をマスターしました！", displayName),
				"@システム",
				time.Now().UTC(),
			)
		}
		if s.legend != nil {
			_ = s.legend.RecordLegend(ctx, "comp_job", state.CharacterID)
		}
	}
}

func (s *Service) loadState(ctx context.Context, char corecharacter.Character) (corejob.CharacterJob, error) {
	state, err := s.repository.FindByCharacterID(ctx, char.ID)
	if err != nil {
		return corejob.NewCharacterJob(char.ID, char.JobID)
	}
	return state, nil
}

func targetSP(state corejob.CharacterJob, char corecharacter.Character, targetID string) int {
	if targetID == char.JobID {
		return char.SP
	}
	if targetID == char.OldJobID {
		return char.OldSP
	}
	if value, ok := state.MasteredSP(targetID); ok {
		return value
	}
	return 0
}

func (s *Service) masterySP(def corejob.Definition) int {
	if s.skills != nil {
		skills := s.skills.SkillsForJob(def.ID)
		if len(skills) > 0 {
			return skills[len(skills)-1].RequiredSP
		}
	}
	return def.MasteryThreshold()
}

func (s *Service) masterySPForJob(jobID string) int {
	def, err := s.GetDefinition(jobID)
	if err != nil {
		return 0
	}
	return s.masterySP(def)
}
