package dungeon

import (
	"context"
	"errors"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

func (s *Service) resolveMissingMemberMPs(
	ctx context.Context,
	exp *ActiveExpedition,
	leader *corecharacter.Character,
) (map[string][2]int, error) {
	resolved := make(map[string][2]int)
	for _, m := range exp.Members {
		if m.HasCurrentMP() {
			continue
		}
		var memberChar corecharacter.Character
		if leader != nil && m.CharacterID == leader.ID {
			memberChar = *leader
		} else {
			var err error
			memberChar, err = s.characterRepo.FindByID(ctx, m.CharacterID)
			if err != nil {
				if errors.Is(err, corecharacter.ErrNotFound) {
					return nil, ErrCharacterNotFound
				}
				return nil, err
			}
		}
		maxMP := memberChar.Stats.MaxMP
		if maxMP <= 0 {
			maxMP = memberChar.Stats.MP
		}
		currMP := memberChar.Stats.MP
		if currMP < 0 {
			currMP = 0
		} else if maxMP > 0 && currMP > maxMP {
			currMP = maxMP
		}
		resolved[m.CharacterID] = [2]int{currMP, maxMP}
	}
	return resolved, nil
}

func applyResolvedMemberMPs(exp *ActiveExpedition, resolved map[string][2]int) {
	for i := range exp.Members {
		if !exp.Members[i].HasCurrentMP() {
			if mp, ok := resolved[exp.Members[i].CharacterID]; ok {
				exp.Members[i].CurrentMP = mp[0]
				exp.Members[i].MaxMP = mp[1]
				exp.Members[i].mpKnown = true
			}
		}
	}
}

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
	case StatusCleared:
		dungeon, ok := s.dungeonMap[exp.DungeonID]
		if !ok {
			return ExpeditionStepResult{}, ErrDungeonNotFound
		}
		return s.handleDungeonClear(ctx, exp, &char, dungeon)
	default:
		return ExpeditionStepResult{}, ErrNoActiveExpedition
	}
}
