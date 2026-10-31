package skill

import (
	"errors"
	"slices"
	"strings"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

var (
	ErrInvalidDefinition  = errors.New("skill definition is invalid")
	ErrUnavailable        = errors.New("skill is unavailable")
	ErrInsufficientMP     = errors.New("insufficient MP")
	ErrDefinitionNotFound = errors.New("skill definition not found")
)

type Definition struct {
	ID             string
	Name           string
	RequiredJobIDs []string
	RequiredSP     int // Skill Point threshold at which this skill is automatically learned.
	MPCost         int
	Effect         corebattle.Effect
}

func NewDefinition(id, name string, jobs []string, requiredSP, mpCost int, effect corebattle.Effect) (Definition, error) {
	if strings.TrimSpace(id) == "" || strings.TrimSpace(name) == "" || requiredSP < 1 ||
		mpCost < 0 || strings.TrimSpace(effect.Kind) == "" || effect.Power < 0 {
		return Definition{}, ErrInvalidDefinition
	}
	return Definition{
		ID: id, Name: name, RequiredJobIDs: append([]string(nil), jobs...),
		RequiredSP: requiredSP, MPCost: mpCost, Effect: effect,
	}, nil
}

type UseRequest struct {
	Character      *corecharacter.Character
	HasItem        func(definitionID string) bool
	RequiredItemID string
}

// IsUnlocked returns true if the skill is unlocked for the character by current
// job SP or previous job SP. Previous-job SP is never substituted by current SP.
func (d Definition) IsUnlocked(char corecharacter.Character) bool {
	if len(d.RequiredJobIDs) == 0 {
		return char.SP >= d.RequiredSP
	}
	if char.JobID != "" && slices.Contains(d.RequiredJobIDs, char.JobID) && char.SP >= d.RequiredSP {
		return true
	}
	if char.OldJobID != "" && slices.Contains(d.RequiredJobIDs, char.OldJobID) && char.OldSP >= d.RequiredSP {
		return true
	}
	return false
}

func (d Definition) CanUse(request UseRequest) error {
	if request.Character == nil {
		return ErrUnavailable
	}
	if !d.IsUnlocked(*request.Character) {
		return ErrUnavailable
	}
	if request.Character.Stats.MP < d.MPCost {
		return ErrInsufficientMP
	}
	if request.RequiredItemID != "" && (request.HasItem == nil || !request.HasItem(request.RequiredItemID)) {
		return ErrUnavailable
	}
	return nil
}

func (d Definition) Use(request UseRequest) (corebattle.Effect, error) {
	if err := d.CanUse(request); err != nil {
		return corebattle.Effect{}, err
	}
	request.Character.Stats.MP -= d.MPCost
	return d.Effect, nil
}
