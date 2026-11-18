package progression

import (
	"errors"

	"github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/job"
	"github.com/witchcraze/party2re/internal/core/random"
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

// ClampLevel lowers a level above the character's current cap without changing experience or SP.
func ClampLevel(value *character.Character) {
	if value != nil {
		value.Level = min(value.Level, MaxLevelForCharacter(value))
	}
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

// ApplyHappySeed sets the character's experience to the threshold required for the next level,
// matching the legacy Party2 CGI formula: $m{exp} = $m{lv} * $m{lv} * 10.
func ApplyHappySeed(value *character.Character) error {
	if value == nil {
		return ErrNilCharacter
	}
	value.Experience = value.Level * value.Level * experienceMultiplier
	return nil
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
func ApplyExperienceWithJobFull(value *character.Character, amount int, definition job.Definition, rng character.RandomSource, opts ApplyExperienceOptions) (LevelUpResult, error) {
	var roll growthRoll
	if rng != nil {
		roll = func(bound float64) (int, error) { return rng.Intn(int(bound)) }
	}
	return applyExperience(value, amount, definition, roll, opts, nil)
}

// ApplyStockLevels advances using saved EXP and returns the uncapped growth
// at each level for the active battle projection. A skipped stock callback
// can still reach the ordinary single-level tail.
func ApplyStockLevels(value *character.Character, definition job.Definition, rng random.Generator, held string, single bool) ([]character.Stats, error) {
	if value == nil {
		return nil, ErrNilCharacter
	}
	if definition.ID == "" || rng == nil {
		return nil, ErrInvalidGrowth
	}
	next := *value
	stock := stockLevels{single: single}
	opts := ApplyExperienceOptions{HasSkillOrb: held == "item-157", StatOrbItems: map[string]bool{held: true}}
	roll := func(bound float64) (int, error) { return int(rng.Float64() * bound), nil }
	if _, err := applyExperience(&next, 0, definition, roll, opts, &stock); err != nil {
		return nil, err
	}
	if len(stock.growth) > 0 {
		// The character write caps once against the final level (system.cgi:49–67).
		next.Stats.Clamp(next.OverLevel, next.Level)
		*value = next
	}
	return stock.growth, nil
}

type growthRoll func(bound float64) (int, error)

type stockLevels struct {
	single bool
	growth []character.Stats
}

func applyExperience(value *character.Character, amount int, definition job.Definition, roll growthRoll, opts ApplyExperienceOptions, stock *stockLevels) (LevelUpResult, error) {
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
	if definition.ID != "" && roll == nil {
		return LevelUpResult{}, ErrInvalidGrowth
	}

	value.Experience += amount
	startingSP := value.SP
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
			bonus, err := roll(4)
			if err != nil {
				return result, err
			}
			if bonus < 1 {
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
			spGrowth := float64(int(float64(value.SP) * .02))
			if stock != nil {
				// The fractional job-49 growth is read at CGI data load (party.cgi:12–13).
				spGrowth = float64(startingSP) * .02
			}
			growth, err := rollGrowth(definition, spGrowth, roll, opts.StatOrbItems)
			if err != nil {
				return result, err
			}
			value.Stats.MaxHP += growth.MaxHP
			value.Stats.MaxMP += growth.MaxMP
			value.Stats.Attack += growth.Attack
			value.Stats.Defense += growth.Defense
			value.Stats.Agility += growth.Agility
			if stock == nil {
				value.Stats.Clamp(value.OverLevel, value.Level)
			} else {
				stock.growth = append(stock.growth, growth)
			}
		}
		result.LevelsGained++
		if stock != nil && stock.single {
			break
		}
	}
	return result, nil
}

func rollGrowth(definition job.Definition, spGrowth float64, roll growthRoll, statOrbItems map[string]bool) (character.Stats, error) {
	growths := [5]float64{
		float64(definition.HPGrowth), float64(definition.MPGrowth), float64(definition.AttackGrowth),
		float64(definition.DefenseGrowth), float64(definition.AgilityGrowth),
	}
	if definition.ID == "job-49" {
		growths = [5]float64{spGrowth, spGrowth, spGrowth, spGrowth, spGrowth}
	}
	for _, growth := range growths {
		if growth < 0 {
			return character.Stats{}, ErrInvalidGrowth
		}
	}
	orbs := [5]string{ItemLifeStatOrb, ItemMagicStatOrb, ItemPowerStatOrb, ItemDefenseStatOrb, ItemAgilityStatOrb}
	var gains [5]int
	for i, maxGrowth := range growths {
		gain, err := growthValue(maxGrowth, roll, statOrbItems[orbs[i]])
		if err != nil {
			return character.Stats{}, err
		}
		gains[i] = gain
	}
	return character.Stats{MaxHP: gains[0] + 1, MaxMP: gains[1], Attack: gains[2], Defense: gains[3], Agility: gains[4]}, nil
}

// growthValue truncates the draw using the job's growth bound.
// If the result exceeds growthCap (9), it is re-rolled as rand(1, 9)
// to match the original CGI formula: $v > 9 → int(rand(9)+1).
func growthValue(max float64, roll growthRoll, statOrb bool) (int, error) {
	bound := max + 1
	if statOrb {
		bound++
	}
	value, err := roll(bound)
	if err != nil {
		return 0, err
	}
	if value > growthCap {
		// Re-roll: rand(9)+1 → [1, 9]
		value, err = roll(growthCap)
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
