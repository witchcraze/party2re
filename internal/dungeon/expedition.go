package dungeon

import (
	"context"
	"errors"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

func (s *Service) GetActiveExpedition(ctx context.Context, characterID string) (*ActiveExpedition, error) {
	if characterID == "" {
		return nil, ErrCharacterNotFound
	}
	run, err := s.activeStore.GetActiveExpedition(ctx, characterID)
	if err != nil {
		return nil, err
	}
	if run != nil && !run.ContainsCharacter(characterID) {
		return nil, ErrForbidden
	}
	return run, nil
}

func (s *Service) StartExpedition(ctx context.Context, characterID, dungeonID string) (*ActiveExpedition, error) {
	var result *ActiveExpedition
	start := func(ctx context.Context) error {
		var err error
		result, err = s.startRoster(ctx, characterID, []string{characterID}, dungeonID, "")
		return err
	}
	var err error
	if s.admission != nil {
		err = s.admission.AdmitSolo(ctx, characterID, start)
	} else {
		err = start(ctx)
	}
	if err != nil && result != nil {
		return s.reconcileStart(ctx, result, err)
	}
	return result, err
}

func (s *Service) Move(ctx context.Context, characterID string, dir Direction) (ExpeditionStepResult, error) {
	if characterID == "" {
		return ExpeditionStepResult{}, ErrCharacterNotFound
	}

	exp, err := s.activeStore.GetActiveExpedition(ctx, characterID)
	if err != nil {
		return ExpeditionStepResult{}, err
	}
	if exp == nil || exp.Status != StatusExploring {
		return ExpeditionStepResult{}, ErrNoActiveExpedition
	}
	if !exp.ContainsCharacter(characterID) {
		return ExpeditionStepResult{}, ErrForbidden
	}
	// Legacy registers the directions for every member, but a fallen one can only Escape.
	if !exp.isAlive(characterID) {
		return ExpeditionStepResult{}, ErrActorFallen
	}

	if exp.Encounter.blocking() {
		return ExpeditionStepResult{}, ErrCombatActive
	}

	if _, ok := s.dungeonMap[exp.DungeonID]; !ok {
		return ExpeditionStepResult{}, ErrDungeonNotFound
	}

	actor, err := s.findCharacter(ctx, characterID)
	if err != nil {
		return ExpeditionStepResult{}, err
	}
	// The leader owns the run buffer, combat and the run-level mirrors.
	owner := actor
	if exp.CharacterID != characterID {
		if owner, err = s.findCharacter(ctx, exp.CharacterID); err != nil {
			return ExpeditionStepResult{}, err
		}
	}

	d, floor, err := s.activeFloor(exp)
	if err != nil {
		return ExpeditionStepResult{}, err
	}
	// The limit rejects before any state, RNG or wall event changes.
	if exp.Round >= exp.RoundLimit {
		return ExpeditionStepResult{}, ErrMovementExhausted
	}

	newX, newY := exp.PosX, exp.PosY
	switch dir {
	case DirectionNorth:
		newY--
	case DirectionSouth:
		newY++
	case DirectionEast:
		newX++
	case DirectionWest:
		newX--
	default:
		return ExpeditionStepResult{}, ErrInvalidDirection
	}

	if newX < 0 || newX >= floor.Width || newY < 0 || newY >= floor.Height {
		return ExpeditionStepResult{}, ErrImpassableWall
	}

	tileChar := floor.Grid[newY][newX]
	if tileChar == cellWall {
		return ExpeditionStepResult{}, ErrImpassableWall
	}
	return s.moveNative(ctx, exp, &owner, &actor, d, floor, newX, newY)
}

func (s *Service) findCharacter(ctx context.Context, id string) (corecharacter.Character, error) {
	char, err := s.characterRepo.FindByID(ctx, id)
	if errors.Is(err, corecharacter.ErrNotFound) {
		return char, ErrCharacterNotFound
	}
	return char, err
}
