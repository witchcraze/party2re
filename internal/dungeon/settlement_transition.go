package dungeon

import (
	"context"
	"errors"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

// freezeExpedition fences further combat before SQL settles this exact snapshot.
func (s *Service) freezeExpedition(ctx context.Context, exp *ActiveExpedition, status ExpeditionStatus) error {
	if exp.Status == status {
		return nil
	}
	if exp.Status != StatusExploring {
		return ErrExpeditionStateChanged
	}
	out, err := s.activeStore.Step(ctx, exp.CharacterID, StepParams{
		ExpectedExpeditionID: exp.ID,
		ExpectedRevision:     &exp.Revision,
		FinishStatus:         status,
		Combat:               true,
		NewFloor:             exp.CurrentFloor,
		NewX:                 exp.PosX,
		NewY:                 exp.PosY,
		Now:                  time.Now().UTC(),
	})
	if errors.Is(err, ErrExpeditionNotFound) {
		return ErrNoActiveExpedition
	}
	if errors.Is(err, ErrExpeditionNotActive) {
		current, readErr := s.activeStore.GetActiveExpedition(ctx, exp.CharacterID)
		if readErr != nil {
			return readErr
		}
		if current == nil {
			return ErrNoActiveExpedition
		}
		if current.ID != exp.ID || current.Status != status {
			return ErrExpeditionStateChanged
		}
		// Concurrent identical exits share the frozen snapshot, never the stale one.
		*exp = *current
		return nil
	}
	if err != nil {
		return err
	}
	*exp = out.Expedition
	return nil
}

// resumeSettlement retries the frozen outcome, including SQL failure or lost cleanup.
func (s *Service) resumeSettlement(ctx context.Context, exp *ActiveExpedition) (ExpeditionStepResult, error) {
	char, err := s.characterRepo.FindByID(ctx, exp.CharacterID)
	if err != nil {
		if errors.Is(err, corecharacter.ErrNotFound) {
			return ExpeditionStepResult{}, ErrCharacterNotFound
		}
		return ExpeditionStepResult{}, err
	}
	switch exp.Status {
	case StatusEscaped:
		return s.handleEscape(ctx, exp, &char, "探索を中断し、戦利品を持って無事に脱出した！")
	case StatusWipedOut:
		return s.handleWipeout(ctx, exp, &char, "探索に失敗した…")
	default:
		return ExpeditionStepResult{}, ErrNoActiveExpedition
	}
}
