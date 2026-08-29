package job

import (
	"context"
	"errors"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	corejob "github.com/witchcraze/party2re/internal/core/job"
)

type Repository interface {
	Save(ctx context.Context, value corejob.CharacterJob) error
	FindByCharacterID(ctx context.Context, characterID string) (corejob.CharacterJob, error)
}

type CharacterRepository interface {
	FindByID(ctx context.Context, id string) (corecharacter.Character, error)
	Update(ctx context.Context, value corecharacter.Character) error
}

type Service struct {
	repository Repository
	catalog    *corejob.Catalog
	characters CharacterRepository
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
	jobState, err := s.Change(ctx, characterID, targetDef, char.Level, char.Gender)
	if err != nil {
		return corecharacter.Character{}, corejob.CharacterJob{}, err
	}
	char.JobID = targetJobID
	if err := s.characters.Update(ctx, char); err != nil {
		return corecharacter.Character{}, corejob.CharacterJob{}, err
	}
	return char, jobState, nil
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
	state.Master(jobID)
	if err := s.repository.Save(ctx, state); err != nil {
		return corejob.CharacterJob{}, err
	}
	return state, nil
}

func (s *Service) CheckAndApplyMastery(ctx context.Context, characterID string, level int) (bool, error) {
	if level < 99 {
		return false, nil
	}
	state, err := s.repository.FindByCharacterID(ctx, characterID)
	if err != nil {
		return false, err
	}
	if state.IsMastered(state.CurrentJobID) {
		return false, nil
	}
	state.Master(state.CurrentJobID)
	if err := s.repository.Save(ctx, state); err != nil {
		return false, err
	}
	return true, nil
}
