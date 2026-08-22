package skill

import (
	"errors"
	"strings"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

var (
	ErrInvalidDefinition = errors.New("skill definition is invalid")
	ErrUnavailable       = errors.New("skill is unavailable")
	ErrInsufficientMP    = errors.New("insufficient MP")
)

type Definition struct {
	ID             string
	Name           string
	RequiredJobIDs []string
	RequiredLevel  int
	MPCost         int
	Effect         corebattle.Effect
}

func NewDefinition(id, name string, jobs []string, level, mpCost int, effect corebattle.Effect) (Definition, error) {
	if strings.TrimSpace(id) == "" || strings.TrimSpace(name) == "" || level < 1 ||
		mpCost < 0 || strings.TrimSpace(effect.Kind) == "" || effect.Power < 0 {
		return Definition{}, ErrInvalidDefinition
	}
	return Definition{
		ID: id, Name: name, RequiredJobIDs: append([]string(nil), jobs...),
		RequiredLevel: level, MPCost: mpCost, Effect: effect,
	}, nil
}

type UseRequest struct {
	Character      *corecharacter.Character
	HasItem        func(definitionID string) bool
	RequiredItemID string
}

func (d Definition) CanUse(request UseRequest) error {
	if request.Character == nil {
		return ErrUnavailable
	}
	if request.Character.Level < d.RequiredLevel || request.Character.Stats.MP < d.MPCost {
		if request.Character.Stats.MP < d.MPCost {
			return ErrInsufficientMP
		}
		return ErrUnavailable
	}
	if len(d.RequiredJobIDs) > 0 && !contains(d.RequiredJobIDs, request.Character.JobID) {
		return ErrUnavailable
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

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
