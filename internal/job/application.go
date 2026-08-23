package job

import (
	"context"
	"errors"

	corejob "github.com/witchcraze/party2re/internal/core/job"
)

type Repository interface {
	Save(ctx context.Context, value corejob.CharacterJob) error
	FindByCharacterID(ctx context.Context, characterID string) (corejob.CharacterJob, error)
}

type Service struct {
	repository Repository
}

func NewService(repository Repository) (*Service, error) {
	if repository == nil {
		return nil, errors.New("job repository is nil")
	}
	return &Service{repository: repository}, nil
}

func (s *Service) Change(ctx context.Context, characterID string, target corejob.Definition, level int, gender string) (corejob.CharacterJob, error) {
	state, err := s.repository.FindByCharacterID(ctx, characterID)
	if err != nil {
		return corejob.CharacterJob{}, err
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
