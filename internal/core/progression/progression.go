package progression

import (
	"errors"

	"github.com/witchcraze/party2re/internal/core/character"
)

const (
	MaxLevel = 99

	experienceMultiplier = 10
)

var (
	ErrNilCharacter          = errors.New("character is nil")
	ErrInvalidExperience     = errors.New("experience cannot be negative")
	ErrInvalidCharacterLevel = errors.New("character level is invalid")
)

// ExperienceForNextLevel returns the cumulative experience required to advance
// from the supplied level.
func ExperienceForNextLevel(level int) (int, error) {
	if level < character.InitialLevel || level >= MaxLevel {
		return 0, ErrInvalidCharacterLevel
	}
	return level * level * experienceMultiplier, nil
}

// ApplyExperience awards cumulative experience and applies every earned level.
func ApplyExperience(value *character.Character, amount int) (int, error) {
	if value == nil {
		return 0, ErrNilCharacter
	}
	if amount < 0 {
		return 0, ErrInvalidExperience
	}
	if value.Level < character.InitialLevel || value.Level > MaxLevel {
		return 0, ErrInvalidCharacterLevel
	}

	value.Experience += amount
	levelsGained := 0
	for value.Level < MaxLevel {
		threshold, err := ExperienceForNextLevel(value.Level)
		if err != nil {
			return levelsGained, err
		}
		if value.Experience < threshold {
			break
		}
		value.Level++
		levelsGained++
	}
	return levelsGained, nil
}
