package progression

import (
	"errors"

	"github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/job"
	"github.com/witchcraze/party2re/internal/core/skill"
)

const (
	MaxLevel     = 99
	OverMaxLevel = 150

	experienceMultiplier = 10

	// growthCap is the maximum stat gain per level-up before the legacy
	// re-roll kicks in (original CGI: $v > 9 → int(rand(9)+1)).
	growthCap = 9

	ItemLifeStatOrb    = "item-152"
	ItemMagicStatOrb   = "item-153"
	ItemPowerStatOrb   = "item-154"
	ItemDefenseStatOrb = "item-155"
	ItemAgilityStatOrb = "item-156"
)

var (
	ErrNilCharacter          = errors.New("character is nil")
	ErrInvalidExperience     = errors.New("experience cannot be negative")
	ErrInvalidCharacterLevel = errors.New("character level is invalid")
	ErrInvalidGrowth         = errors.New("job growth is invalid")
)

// MaxLevelForCharacter returns the maximum level for a character, taking OverLevel limit break into account.
func MaxLevelForCharacter(char *character.Character) int {
	if char != nil && char.OverLevel {
		return OverMaxLevel
	}
	return MaxLevel
}

// ExperienceForNextLevel returns the cumulative experience required to advance
// from the supplied level.
func ExperienceForNextLevel(level int) (int, error) {
	if level < character.InitialLevel || level >= MaxLevel {
		return 0, ErrInvalidCharacterLevel
	}
	return level * level * experienceMultiplier, nil
}

// ExperienceForNextLevelWithMax returns the cumulative experience required to advance
// from the supplied level considering a custom maximum level.
func ExperienceForNextLevelWithMax(level int, maxLevel int) (int, error) {
	if level < character.InitialLevel || level >= maxLevel {
		return 0, ErrInvalidCharacterLevel
	}
	return level * level * experienceMultiplier, nil
}

// ApplyExperience awards cumulative experience and applies every earned level.
func ApplyExperience(value *character.Character, amount int) (int, error) {
	result, err := ApplyExperienceWithJob(value, amount, job.Definition{}, zeroRandomSource{}, false, nil)
	return result.LevelsGained, err
}

// LevelUpResult holds the outcome of a level-up event.
type LevelUpResult struct {
	// LevelsGained is the number of levels gained.
	LevelsGained int
	// NewlyLearnedSkillIDs contains the IDs of skills learned by SP threshold
	// during this experience application (SP == skill.RequiredSP).
	NewlyLearnedSkillIDs []string
}

// ApplyExperienceOptions configures optional level-up behaviour.
type ApplyExperienceOptions struct {
	// HasSkillOrb indicates the character carries item 157 (スキルの宝珠),
	// which grants a 25% chance of an additional SP on each level-up.
	HasSkillOrb bool
	// JobSkills is the ordered list of skill definitions for the character's
	// current job, used to detect SP-threshold skill learning.
	JobSkills []skill.Definition
	// StatOrbItems contains the stat orb item IDs currently carried by the character.
	StatOrbItems map[string]bool
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
	result, err := ApplyExperienceWithJob(value, amount, definition, random, false, nil)
	if err != nil {
		return 0, err
	}
	return result.LevelsGained, nil
}

// ApplyExperienceWithJob awards experience and applies job growth for each
// earned level. Current HP and MP are intentionally not restored.
//
// Deprecated: prefer ApplyExperienceWithJobFull for SP / skill-learning support.
func ApplyExperienceWithJob(value *character.Character, amount int, definition job.Definition, random character.RandomSource, hasSkillOrb bool, jobSkills []skill.Definition) (LevelUpResult, error) {
	return ApplyExperienceWithJobFull(value, amount, definition, random, ApplyExperienceOptions{
		HasSkillOrb: hasSkillOrb,
		JobSkills:   jobSkills,
	})
}

// ApplyExperienceWithJobFull is the canonical implementation: awards experience,
// increments SP on each level-up (with 25% bonus for item 157), detects
// SP-threshold skill learning, and applies job stat growth.
func ApplyExperienceWithJobFull(value *character.Character, amount int, definition job.Definition, random character.RandomSource, opts ApplyExperienceOptions) (LevelUpResult, error) {
	if value == nil {
		return LevelUpResult{}, ErrNilCharacter
	}
	if amount < 0 {
		return LevelUpResult{}, ErrInvalidExperience
	}
	maxLvl := MaxLevelForCharacter(value)
	if value.Level < character.InitialLevel || value.Level > maxLvl {
		return LevelUpResult{}, ErrInvalidCharacterLevel
	}
	if definition.ID != "" && random == nil {
		return LevelUpResult{}, ErrInvalidGrowth
	}

	value.Experience += amount
	var result LevelUpResult
	for value.Level < maxLvl {
		threshold, err := ExperienceForNextLevelWithMax(value.Level, maxLvl)
		if err != nil {
			return result, err
		}
		if value.Experience < threshold {
			break
		}
		value.Level++

		// SP gain: +1 per level (旧CGI: ++$m{sp})
		value.SP++

		// スキルの宝珠 (item 157): 25% chance of +1 additional SP.
		if opts.HasSkillOrb {
			roll, err := random.Intn(4)
			if err != nil {
				return result, err
			}
			if roll < 1 {
				value.SP++
			}
		}

		// SP-based skill learning: trigger when SP == skill.RequiredSP (旧CGI: $skills[$i][0] eq $m{sp})
		for _, sk := range opts.JobSkills {
			if value.SP == sk.RequiredSP {
				result.NewlyLearnedSkillIDs = append(result.NewlyLearnedSkillIDs, sk.ID)
			}
		}

		if definition.ID != "" {
			if err := applyGrowth(&value.Stats, definition, random, opts.StatOrbItems, value.OverLevel, value.Level); err != nil {
				return result, err
			}
		}
		result.LevelsGained++
	}
	return result, nil
}

func applyGrowth(stats *character.Stats, definition job.Definition, random character.RandomSource, statOrbItems map[string]bool, overLevel bool, level int) error {
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

	hp, err := growthValue(definition.HPGrowth, random, statOrbItems[ItemLifeStatOrb])
	if err != nil {
		return err
	}
	hp++ // HP minimum +1 guaranteed (旧CGI: ++$v for hp)
	mp, err := growthValue(definition.MPGrowth, random, statOrbItems[ItemMagicStatOrb])
	if err != nil {
		return err
	}
	attack, err := growthValue(definition.AttackGrowth, random, statOrbItems[ItemPowerStatOrb])
	if err != nil {
		return err
	}
	defense, err := growthValue(definition.DefenseGrowth, random, statOrbItems[ItemDefenseStatOrb])
	if err != nil {
		return err
	}
	agility, err := growthValue(definition.AgilityGrowth, random, statOrbItems[ItemAgilityStatOrb])
	if err != nil {
		return err
	}
	stats.MaxHP += hp
	stats.MaxMP += mp
	stats.Attack += attack
	stats.Defense += defense
	stats.Agility += agility
	stats.Clamp(overLevel, level)
	return nil
}

// growthValue returns a random growth amount in [0, max].
// If the result exceeds growthCap (9), it is re-rolled as rand(1, 9)
// to match the original CGI formula: $v > 9 → int(rand(9)+1).
func growthValue(max int, random character.RandomSource, statOrb ...bool) (int, error) {
	bonus := 0
	if len(statOrb) > 0 && statOrb[0] {
		bonus = 1
	}
	value, err := random.Intn(max + 1 + bonus)
	if err != nil {
		return 0, err
	}
	if value > growthCap {
		// Re-roll: rand(9)+1 → [1, 9]
		value, err = random.Intn(growthCap)
		if err != nil {
			return 0, err
		}
		value++ // ensure minimum 1
	}
	return value, nil
}

type zeroRandomSource struct{}

func (zeroRandomSource) Intn(int) (int, error) {
	return 0, nil
}
