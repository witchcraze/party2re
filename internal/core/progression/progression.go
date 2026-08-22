package progression

import (
	"errors"

	"github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/job"
)

const (
	MaxLevel = 99

	experienceMultiplier = 10
)

var (
	ErrNilCharacter          = errors.New("character is nil")
	ErrInvalidExperience     = errors.New("experience cannot be negative")
	ErrInvalidCharacterLevel = errors.New("character level is invalid")
	ErrInvalidGrowth         = errors.New("job growth is invalid")
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
	return ApplyExperienceWithJob(value, amount, job.Definition{}, zeroRandomSource{})
}

func ApplyExperienceWithProvider(value *character.Character, amount int, provider job.DefinitionProvider, random character.RandomSource) (int, error) {
	if value == nil {
		return 0, ErrNilCharacter
	}
	if provider == nil {
		return 0, ErrInvalidGrowth
	}
	definition, err := provider.FindByID(value.JobID)
	if err != nil {
		return 0, err
	}
	return ApplyExperienceWithJob(value, amount, definition, random)
}

// ApplyExperienceWithJob awards experience and applies job growth for each
// earned level. Current HP and MP are intentionally not restored.
func ApplyExperienceWithJob(value *character.Character, amount int, definition job.Definition, random character.RandomSource) (int, error) {
	if value == nil {
		return 0, ErrNilCharacter
	}
	if amount < 0 {
		return 0, ErrInvalidExperience
	}
	if value.Level < character.InitialLevel || value.Level > MaxLevel {
		return 0, ErrInvalidCharacterLevel
	}
	if definition.ID != "" && random == nil {
		return 0, ErrInvalidGrowth
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
		if definition.ID != "" {
			if err := applyGrowth(&value.Stats, definition, random); err != nil {
				return levelsGained, err
			}
		}
		levelsGained++
	}
	return levelsGained, nil
}

func applyGrowth(stats *character.Stats, definition job.Definition, random character.RandomSource) error {
	growths := []int{
		definition.HPGrowth,
		definition.MPGrowth,
		definition.AttackGrowth,
		definition.DefenseGrowth,
		definition.AgilityGrowth,
	}
	for _, growth := range growths {
		if growth < 0 {
			return ErrInvalidGrowth
		}
	}

	hp, err := growthValue(definition.HPGrowth, random)
	if err != nil {
		return err
	}
	hp++
	mp, err := growthValue(definition.MPGrowth, random)
	if err != nil {
		return err
	}
	attack, err := growthValue(definition.AttackGrowth, random)
	if err != nil {
		return err
	}
	defense, err := growthValue(definition.DefenseGrowth, random)
	if err != nil {
		return err
	}
	agility, err := growthValue(definition.AgilityGrowth, random)
	if err != nil {
		return err
	}
	stats.MaxHP += hp
	stats.MaxMP += mp
	stats.Attack += attack
	stats.Defense += defense
	stats.Agility += agility
	return nil
}

func growthValue(max int, random character.RandomSource) (int, error) {
	value, err := random.Intn(max + 1)
	if err != nil {
		return 0, err
	}
	return value, nil
}

type zeroRandomSource struct{}

func (zeroRandomSource) Intn(int) (int, error) {
	return 0, nil
}
