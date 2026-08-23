package progression

import (
	"errors"

	"github.com/witchcraze/party2re/internal/core/character"
)

const (
	MinLevelForRebirth       = 99
	PermanentBonusPerRebirth = 5
	BaseInitialHP            = 30
	BaseInitialMP            = 10
	BaseInitialStat          = 10
)

var (
	ErrRebirthNotEligible = errors.New("character must be level 99 to undergo rebirth")
)

func Rebirth(char *character.Character) error {
	if char == nil {
		return ErrNilCharacter
	}
	if char.Level < MinLevelForRebirth {
		return ErrRebirthNotEligible
	}

	char.RebirthCount++
	char.Level = character.InitialLevel
	char.Experience = 0

	bonus := char.RebirthCount * PermanentBonusPerRebirth
	char.Stats = character.Stats{
		MaxHP:   BaseInitialHP + bonus*2,
		MaxMP:   BaseInitialMP + bonus,
		HP:      BaseInitialHP + bonus*2,
		MP:      BaseInitialMP + bonus,
		Attack:  BaseInitialStat + bonus,
		Defense: BaseInitialStat + bonus,
		Agility: BaseInitialStat + bonus,
	}

	return nil
}
