package job

import (
	"context"
	"errors"
	"fmt"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
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

type Service struct {
	repository     Repository
	catalog        *corejob.Catalog
	characters     CharacterRepository
	inventories    InventoryRepository
	economy        *economy.Service
	skills         SkillProvider
	news           NewsPublisher
	futureMemories FutureMemoryRepository
}

type Option func(*Service)

func WithCatalog(catalog *corejob.Catalog) Option {
	return func(s *Service) {
		s.catalog = catalog
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

func (s *Service) ChangeJob(ctx context.Context, characterID string, targetJobID string) (corecharacter.Character, corejob.CharacterJob, error) {
	if s.characters == nil {
		return corecharacter.Character{}, corejob.CharacterJob{}, errors.New("character repository not configured")
	}
	char, err := s.characters.FindByID(ctx, characterID)
	if err != nil {
		return corecharacter.Character{}, corejob.CharacterJob{}, err
	}
	targetDef, err := s.GetDefinition(targetJobID)
	if err != nil {
		return corecharacter.Character{}, corejob.CharacterJob{}, err
	}
	if char.JobMemory != nil {
		return corecharacter.Character{}, corejob.CharacterJob{}, corejob.ErrJobUnavailable
	}
	state, err := s.loadState(ctx, char)
	if err != nil {
		return corecharacter.Character{}, corejob.CharacterJob{}, err
	}
	if err := state.ChangeTo(targetDef, char.Level, char.Gender); err != nil {
		return corecharacter.Character{}, corejob.CharacterJob{}, err
	}
	if currentDef, err := s.GetDefinition(char.JobID); err == nil {
		state.RecordMastery(char.JobID, char.SP, s.masterySP(currentDef))
	}
	s.checkAndNotifyCompletion(ctx, char.Name, &state)
	targetSPValue := targetSP(state, char, targetJobID)
	requiredItem := targetDef.RequiredItem()
	needItem := requiredItem != "" && targetJobID != char.JobID && targetJobID != char.OldJobID &&
		!state.IsMastered(targetJobID) &&
		!(targetJobID == "job-33" && state.IsMastered("job-08")) &&
		!(targetJobID == "job-46" && state.IsMastered("job-08"))

	if s.economy != nil {
		req := economy.TransactionRequest{CharacterID: characterID, LockInventory: needItem}
		if needItem {
			req.Cost.ItemDefinitionID = requiredItem
			req.Cost.ItemDefinitionQty = 1
		}
		var updated corecharacter.Character
		var updatedState corejob.CharacterJob
		_, err := s.economy.ExecuteTransaction(ctx, req, func(tc *economy.TxContext) error {
			currentState, err := s.loadState(tc.Context, tc.Character)
			if err != nil {
				return err
			}
			currentJobID := currentState.CurrentJobID
			if currentDef, err := s.GetDefinition(currentJobID); err == nil {
				currentState.RecordMastery(currentJobID, tc.Character.SP, s.masterySP(currentDef))
			}
			s.checkAndNotifyCompletion(tc.Context, tc.Character.Name, &currentState)
			if err := currentState.ChangeTo(targetDef, tc.Character.Level, tc.Character.Gender); err != nil {
				return err
			}
			targetSPValue := targetSP(currentState, tc.Character, targetJobID)
			if err := tc.Character.ApplyJobChange(targetJobID, targetSPValue); err != nil {
				return err
			}
			if err := s.repository.Save(tc.Context, currentState); err != nil {
				return err
			}
			updated, updatedState = tc.Character, currentState
			return nil
		})
		if err != nil {
			if errors.Is(err, economy.ErrItemNotFound) || errors.Is(err, economy.ErrInsufficientItemQuantity) {
				return corecharacter.Character{}, corejob.CharacterJob{}, ErrRequiredItem
			}
			return corecharacter.Character{}, corejob.CharacterJob{}, err
		}
		return updated, updatedState, nil
	}

	if needItem {
		if s.inventories == nil {
			return corecharacter.Character{}, corejob.CharacterJob{}, ErrRequiredItem
		}
		inventory, err := s.inventories.FindByCharacterID(ctx, characterID)
		if err != nil || inventory.Quantity(requiredItem) < 1 {
			return corecharacter.Character{}, corejob.CharacterJob{}, ErrRequiredItem
		}
		for _, instance := range inventory.Items {
			if instance.DefinitionID == requiredItem {
				if err := inventory.Consume(instance.ID, 1); err != nil {
					return corecharacter.Character{}, corejob.CharacterJob{}, err
				}
				break
			}
		}
		if err := s.inventories.Save(ctx, inventory); err != nil {
			return corecharacter.Character{}, corejob.CharacterJob{}, err
		}
	}
	if err := char.ApplyJobChange(targetJobID, targetSPValue); err != nil {
		return corecharacter.Character{}, corejob.CharacterJob{}, err
	}
	if err := s.characters.Update(ctx, char); err != nil {
		return corecharacter.Character{}, corejob.CharacterJob{}, err
	}
	if err := s.repository.Save(ctx, state); err != nil {
		return corecharacter.Character{}, corejob.CharacterJob{}, err
	}
	return char, state, nil
}

func (s *Service) Change(ctx context.Context, characterID string, target corejob.Definition, level int, gender string) (corejob.CharacterJob, error) {
	state, err := s.repository.FindByCharacterID(ctx, characterID)
	if err != nil {
		state, err = corejob.NewCharacterJob(characterID, "starter")
		if err != nil {
			return corejob.CharacterJob{}, err
		}
	}
	if err := state.ChangeTo(target, level, gender); err != nil {
		return corejob.CharacterJob{}, err
	}
	if err := s.repository.Save(ctx, state); err != nil {
		return corejob.CharacterJob{}, err
	}
	return state, nil
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
				fmt.Sprintf("<span class=\"comp\">%sが全ての職業をマスターしました！</span>", displayName),
				"@システム",
				time.Now().UTC(),
			)
		}
	}
}

var ErrRequiredItem = errors.New("required job-change item is missing")

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

func (s *Service) ExchangeJob(ctx context.Context, characterID, targetJobID, targetOldJobID string) (corecharacter.Character, corejob.CharacterJob, error) {
	if s.characters == nil {
		return corecharacter.Character{}, corejob.CharacterJob{}, errors.New("character repository not configured")
	}
	char, err := s.characters.FindByID(ctx, characterID)
	if err != nil {
		return corecharacter.Character{}, corejob.CharacterJob{}, err
	}
	state, err := s.loadState(ctx, char)
	if err != nil {
		return corecharacter.Character{}, corejob.CharacterJob{}, err
	}
	if char.JobMemory != nil {
		memory := *char.JobMemory
		if err := char.ApplyJobMemory(memory.JobID, memory.SP, memory.OldJobID, memory.OldSP); err != nil {
			return corecharacter.Character{}, corejob.CharacterJob{}, err
		}
		char.JobMemory = nil
		if err := s.characters.Update(ctx, char); err != nil {
			return corecharacter.Character{}, corejob.CharacterJob{}, err
		}
		return char, state, nil
	}
	if targetJobID == "" || targetOldJobID == "" || targetJobID == targetOldJobID ||
		!state.IsMastered(targetJobID) || !state.IsMastered(targetOldJobID) {
		return corecharacter.Character{}, corejob.CharacterJob{}, corejob.ErrJobUnavailable
	}
	targetSPValue, ok := state.MasteredSP(targetJobID)
	if !ok {
		return corecharacter.Character{}, corejob.CharacterJob{}, corejob.ErrJobUnavailable
	}
	targetOldSPValue, ok := state.MasteredSP(targetOldJobID)
	if !ok {
		return corecharacter.Character{}, corejob.CharacterJob{}, corejob.ErrJobUnavailable
	}
	if s.economy != nil {
		req := economy.TransactionRequest{CharacterID: characterID, LockInventory: true,
			Cost: economy.ResourceCost{ItemDefinitionID: "item-168", ItemDefinitionQty: 1}}
		var updated corecharacter.Character
		_, err := s.economy.ExecuteTransaction(ctx, req, func(tc *economy.TxContext) error {
			tc.Character.JobMemory = &corecharacter.JobMemory{JobID: tc.Character.JobID, SP: tc.Character.SP, OldJobID: tc.Character.OldJobID, OldSP: tc.Character.OldSP}
			if err := tc.Character.ApplyJobMemory(targetJobID, targetSPValue, targetOldJobID, targetOldSPValue); err != nil {
				return err
			}
			if err := s.repository.Save(tc.Context, state); err != nil {
				return err
			}
			updated = tc.Character
			return nil
		})
		if err != nil {
			if errors.Is(err, economy.ErrItemNotFound) || errors.Is(err, economy.ErrInsufficientItemQuantity) {
				return corecharacter.Character{}, corejob.CharacterJob{}, ErrRequiredItem
			}
			return corecharacter.Character{}, corejob.CharacterJob{}, err
		}
		return updated, state, nil
	}
	if s.inventories == nil {
		return corecharacter.Character{}, corejob.CharacterJob{}, ErrRequiredItem
	}
	inventory, err := s.inventories.FindByCharacterID(ctx, characterID)
	if err != nil || inventory.Quantity("item-168") < 1 {
		return corecharacter.Character{}, corejob.CharacterJob{}, ErrRequiredItem
	}
	for _, instance := range inventory.Items {
		if instance.DefinitionID == "item-168" {
			if err := inventory.Consume(instance.ID, 1); err != nil {
				return corecharacter.Character{}, corejob.CharacterJob{}, err
			}
			break
		}
	}
	memory := &corecharacter.JobMemory{JobID: char.JobID, SP: char.SP, OldJobID: char.OldJobID, OldSP: char.OldSP}
	if err := char.ApplyJobMemory(targetJobID, targetSPValue, targetOldJobID, targetOldSPValue); err != nil {
		return corecharacter.Character{}, corejob.CharacterJob{}, err
	}
	char.JobMemory = memory
	if err := s.characters.Update(ctx, char); err != nil {
		return corecharacter.Character{}, corejob.CharacterJob{}, err
	}
	if err := s.repository.Save(ctx, state); err != nil {
		return corecharacter.Character{}, corejob.CharacterJob{}, fmt.Errorf("save job memory: %w", err)
	}
	if err := s.inventories.Save(ctx, inventory); err != nil {
		return corecharacter.Character{}, corejob.CharacterJob{}, err
	}
	return char, state, nil
}
