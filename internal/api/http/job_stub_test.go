package http_test

import (
	"context"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	corejob "github.com/witchcraze/party2re/internal/core/job"
	jobapp "github.com/witchcraze/party2re/internal/job"
)

type stubJobService struct {
	listDefinitionsFn    func() []corejob.Definition
	changeJobFn          func(ctx context.Context, characterID, targetJobID string) (corecharacter.Character, corejob.CharacterJob, error)
	exchangeJobFn        func(ctx context.Context, characterID, targetJobID, targetOldJobID string) (corecharacter.Character, corejob.CharacterJob, error)
	recallFutureMemoryFn func(ctx context.Context, characterID, memoryID string) (corecharacter.Character, corejob.CharacterJob, error)
	getJobMasteryFn      func(ctx context.Context, characterID string) (jobapp.CharacterJobMastery, error)
}

func (s *stubJobService) ListDefinitions() []corejob.Definition {
	if s.listDefinitionsFn != nil {
		return s.listDefinitionsFn()
	}
	return nil
}

func (s *stubJobService) ChangeJob(ctx context.Context, characterID, targetJobID string) (corecharacter.Character, corejob.CharacterJob, error) {
	if s.changeJobFn != nil {
		return s.changeJobFn(ctx, characterID, targetJobID)
	}
	return corecharacter.Character{}, corejob.CharacterJob{}, nil
}

func (s *stubJobService) ExchangeJob(ctx context.Context, characterID, targetJobID, targetOldJobID string, itemDefinitionID ...string) (corecharacter.Character, corejob.CharacterJob, error) {
	if s.exchangeJobFn != nil {
		return s.exchangeJobFn(ctx, characterID, targetJobID, targetOldJobID)
	}
	return corecharacter.Character{}, corejob.CharacterJob{}, nil
}

func (s *stubJobService) RecallFutureMemory(ctx context.Context, characterID, memoryID string) (corecharacter.Character, corejob.CharacterJob, error) {
	if s.recallFutureMemoryFn != nil {
		return s.recallFutureMemoryFn(ctx, characterID, memoryID)
	}
	return corecharacter.Character{}, corejob.CharacterJob{}, nil
}

func (s *stubJobService) TempleStatus(context.Context, string) (jobapp.TempleStatus, error) {
	return jobapp.TempleStatus{}, nil
}

func (s *stubJobService) GetJobMastery(ctx context.Context, characterID string) (jobapp.CharacterJobMastery, error) {
	if s.getJobMasteryFn != nil {
		return s.getJobMasteryFn(ctx, characterID)
	}
	return jobapp.CharacterJobMastery{}, nil
}
