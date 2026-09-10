package progression

import (
	"errors"
	"testing"

	"github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/job"
	"github.com/witchcraze/party2re/internal/core/skill"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func TestExperienceForNextLevelUsesCumulativeSquareThreshold(t *testing.T) {
	tests := []struct {
		level int
		want  int
	}{
		{level: 1, want: 10},
		{level: 2, want: 40},
		{level: 10, want: 1000},
	}
	for _, test := range tests {
		got, err := ExperienceForNextLevel(test.level)
		if err != nil {
			t.Fatalf("ExperienceForNextLevel(%d) error = %v", test.level, err)
		}
		if got != test.want {
			t.Errorf("ExperienceForNextLevel(%d) = %d, want %d", test.level, got, test.want)
		}
	}
}

func TestApplyExperienceLevelsUpAtThreshold(t *testing.T) {
	value, err := character.New("Alice")
	if err != nil {
		t.Fatal(err)
	}

	levels, err := ApplyExperience(&value, 9)
	if err != nil {
		t.Fatal(err)
	}
	if levels != 0 || value.Level != 1 || value.Experience != 9 {
		t.Fatalf("below threshold = levels %d, character %#v", levels, value)
	}

	levels, err = ApplyExperience(&value, 1)
	if err != nil {
		t.Fatal(err)
	}
	if levels != 1 || value.Level != 2 || value.Experience != 10 {
		t.Fatalf("at threshold = levels %d, character %#v", levels, value)
	}
}

func TestApplyExperienceCanGainMultipleLevels(t *testing.T) {
	value, err := character.New("Alice")
	if err != nil {
		t.Fatal(err)
	}

	levels, err := ApplyExperience(&value, 100)
	if err != nil {
		t.Fatal(err)
	}
	if levels != 3 || value.Level != 4 || value.Experience != 100 {
		t.Fatalf("multiple levels = levels %d, character %#v", levels, value)
	}
}

func TestApplyExperienceRejectsInvalidAmountsAndLevels(t *testing.T) {
	value, err := character.New("Alice")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ApplyExperience(&value, -1); !errors.Is(err, ErrInvalidExperience) {
		t.Fatalf("negative amount error = %v, want %v", err, ErrInvalidExperience)
	}
	if _, err := ApplyExperience(nil, 1); !errors.Is(err, ErrNilCharacter) {
		t.Fatalf("nil character error = %v, want %v", err, ErrNilCharacter)
	}
	value.Level = MaxLevel + 1
	if _, err := ApplyExperience(&value, 1); !errors.Is(err, ErrInvalidCharacterLevel) {
		t.Fatalf("invalid level error = %v, want %v", err, ErrInvalidCharacterLevel)
	}
}

func TestApplyExperienceStopsAtMaximumLevel(t *testing.T) {
	value, err := character.New("Alice")
	if err != nil {
		t.Fatal(err)
	}
	value.Level = MaxLevel

	levels, err := ApplyExperience(&value, 1000000)
	if err != nil {
		t.Fatal(err)
	}
	if levels != 0 || value.Level != MaxLevel || value.Experience != 1000000 {
		t.Fatalf("maximum level = levels %d, character %#v", levels, value)
	}
}

func TestApplyExperienceWithJobAppliesRandomGrowthAndDoesNotRestoreCurrentResources(t *testing.T) {
	value, err := character.New("Alice")
	if err != nil {
		t.Fatal(err)
	}
	value.Stats = character.Stats{MaxHP: 30, HP: 12, MaxMP: 8, MP: 3, Attack: 6, Defense: 6, Agility: 6}
	definition, err := job.NewDefinition("vanguard", "Vanguard", 6, 1, 3, 5, 2, 1, "")
	if err != nil {
		t.Fatal(err)
	}
	random := &sequenceRandomSource{values: []int{6, 1, 3, 0, 2}}

	result, err := ApplyExperienceWithJob(&value, 10, definition, random, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.LevelsGained != 1 || value.Level != 2 {
		t.Fatalf("level result = %d, character = %#v", result.LevelsGained, value)
	}
	want := character.Stats{MaxHP: 37, HP: 12, MaxMP: 9, MP: 3, Attack: 9, Defense: 6, Agility: 8}
	if value.Stats != want {
		t.Fatalf("stats = %#v, want %#v", value.Stats, want)
	}
}

func TestApplyExperienceWithJobGivesHPMinimumGrowth(t *testing.T) {
	value, err := character.New("Alice")
	if err != nil {
		t.Fatal(err)
	}
	value.Stats = character.Stats{MaxHP: 30, HP: 30, MaxMP: 6, MP: 6, Attack: 6, Defense: 6, Agility: 6}
	definition, err := job.NewDefinition("novice", "Novice", 0, 0, 0, 0, 0, 1, "")
	if err != nil {
		t.Fatal(err)
	}

	result, err := ApplyExperienceWithJob(&value, 10, definition, &sequenceRandomSource{values: []int{0, 0, 0, 0, 0}}, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.LevelsGained != 1 || value.Stats.MaxHP != 31 {
		t.Fatalf("level result = %d, stats = %#v", result.LevelsGained, value.Stats)
	}
}

func TestApplyExperienceWithJobAppliesGrowthForMultipleLevels(t *testing.T) {
	value, err := character.New("Alice")
	if err != nil {
		t.Fatal(err)
	}
	value.Stats = character.Stats{MaxHP: 30, HP: 30, MaxMP: 6, MP: 6, Attack: 6, Defense: 6, Agility: 6}
	definition, err := job.NewDefinition("vanguard", "Vanguard", 1, 1, 1, 1, 1, 1, "")
	if err != nil {
		t.Fatal(err)
	}
	random := &sequenceRandomSource{values: []int{
		1, 1, 1, 1, 1,
		0, 0, 0, 0, 0,
		1, 1, 1, 1, 1,
	}}

	result, err := ApplyExperienceWithJob(&value, 100, definition, random, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.LevelsGained != 3 || value.Level != 4 {
		t.Fatalf("level result = %d, character = %#v", result.LevelsGained, value)
	}
	if value.Stats.MaxHP != 35 || value.Stats.MaxMP != 8 ||
		value.Stats.Attack != 8 || value.Stats.Defense != 8 || value.Stats.Agility != 8 {
		t.Fatalf("stats = %#v", value.Stats)
	}
}

func TestApplyExperienceWithJobRejectsInvalidGrowthAndRandomSource(t *testing.T) {
	value, err := character.New("Alice")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ApplyExperienceWithJob(&value, 10, job.Definition{ID: "broken", HPGrowth: -1}, &sequenceRandomSource{}, false, nil); !errors.Is(err, ErrInvalidGrowth) {
		t.Fatalf("invalid growth error = %v, want %v", err, ErrInvalidGrowth)
	}
	definition, _ := job.NewDefinition("novice", "Novice", 1, 1, 1, 1, 1, 1, "")
	if _, err := ApplyExperienceWithJob(&value, 10, definition, nil, false, nil); !errors.Is(err, ErrInvalidGrowth) {
		t.Fatalf("nil random error = %v, want %v", err, ErrInvalidGrowth)
	}
}

func TestApplyExperienceWithProviderUsesCharacterJobID(t *testing.T) {
	value, err := character.New("Alice")
	if err != nil {
		t.Fatal(err)
	}
	value.JobID = "job-01"
	value.Stats = character.Stats{MaxHP: 30, HP: 30, MaxMP: 6, MP: 6, Attack: 6, Defense: 6, Agility: 6}
	catalog, err := job.InitialCatalog()
	if err != nil {
		t.Fatal(err)
	}
	random := &sequenceRandomSource{values: []int{0, 0, 0, 0, 0}}

	levels, err := ApplyExperienceWithProvider(&value, 10, catalog, random)
	if err != nil {
		t.Fatal(err)
	}
	if levels != 1 || value.Stats.MaxHP != 31 || value.Stats.MaxMP != 6 {
		t.Fatalf("levels = %d, stats = %#v", levels, value.Stats)
	}
}

func TestExperienceForNextLevelBoundary(t *testing.T) {
	invalidLevels := []int{-5, 0, MaxLevel, MaxLevel + 1}
	for _, lvl := range invalidLevels {
		if _, err := ExperienceForNextLevel(lvl); !errors.Is(err, ErrInvalidCharacterLevel) {
			t.Fatalf("ExperienceForNextLevel(%d) error = %v, want %v", lvl, err, ErrInvalidCharacterLevel)
		}
	}
}

func TestApplyExperienceWithProviderErrors(t *testing.T) {
	value, err := character.New("Alice")
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := job.InitialCatalog()
	if err != nil {
		t.Fatal(err)
	}
	random := &sequenceRandomSource{}

	if _, err := ApplyExperienceWithProvider(nil, 10, catalog, random); !errors.Is(err, ErrNilCharacter) {
		t.Fatalf("ApplyExperienceWithProvider(nil character) error = %v, want %v", err, ErrNilCharacter)
	}
	if _, err := ApplyExperienceWithProvider(&value, 10, nil, random); !errors.Is(err, ErrInvalidGrowth) {
		t.Fatalf("ApplyExperienceWithProvider(nil provider) error = %v, want %v", err, ErrInvalidGrowth)
	}

	value.JobID = "unknown-job"
	if _, err := ApplyExperienceWithProvider(&value, 10, catalog, random); err == nil {
		t.Fatal("ApplyExperienceWithProvider(unknown job) expected error, got nil")
	}
}

func TestApplyExperienceWithJobRandomError(t *testing.T) {
	value, err := character.New("Alice")
	if err != nil {
		t.Fatal(err)
	}
	definition, err := job.NewDefinition("vanguard", "Vanguard", 1, 1, 1, 1, 1, 1, "")
	if err != nil {
		t.Fatal(err)
	}
	errRandom := errRandomSource{}
	if _, err := ApplyExperienceWithJob(&value, 10, definition, errRandom, false, nil); err == nil {
		t.Fatal("ApplyExperienceWithJob(errRandom) expected error, got nil")
	}
}

func TestApplyExperienceWithJobNegativeGrowthFields(t *testing.T) {
	random := &sequenceRandomSource{values: []int{0}}

	definitions := []job.Definition{
		{ID: "bad1", HPGrowth: -1},
		{ID: "bad2", MPGrowth: -1},
		{ID: "bad3", AttackGrowth: -1},
		{ID: "bad4", DefenseGrowth: -1},
		{ID: "bad5", AgilityGrowth: -1},
	}
	for _, def := range definitions {
		char, err := character.New("Alice")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := ApplyExperienceWithJob(&char, 10, def, random, false, nil); !errors.Is(err, ErrInvalidGrowth) {
			t.Fatalf("ApplyExperienceWithJob(%#v) error = %v, want %v", def, err, ErrInvalidGrowth)
		}
	}
}

func TestApplyExperience_OverLevel_GrowthTo150(t *testing.T) {
	char, err := character.New("Legend")
	if err != nil {
		t.Fatal(err)
	}
	char.Level = 99
	char.Experience = 99 * 99 * 10
	char.OverLevel = true

	def := job.Definition{
		ID:            "hero",
		Name:          "Hero",
		HPGrowth:      5,
		MPGrowth:      3,
		AttackGrowth:  2,
		DefenseGrowth: 2,
		AgilityGrowth: 1,
	}
	random := &sequenceRandomSource{values: []int{0, 0, 0, 0, 0, 0, 0, 0, 0, 0}}

	// Gain experience to reach Lv100
	needed100, err := ExperienceForNextLevelWithMax(99, OverMaxLevel)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result, err := ApplyExperienceWithJob(&char, needed100-char.Experience, def, random, false, nil)
	if err != nil {
		t.Fatalf("ApplyExperienceWithJob failed: %v", err)
	}
	if result.LevelsGained != 1 || char.Level != 100 {
		t.Fatalf("expected level 100 with 1 level gained, got level %d with %d gained", char.Level, result.LevelsGained)
	}
}

// ---------------------------------------------------------------------------
// SP gain tests (旧CGI仕様)
// ---------------------------------------------------------------------------

func TestApplyExperienceIncrementsSPOnEachLevelUp(t *testing.T) {
	char, err := character.New("Bob")
	if err != nil {
		t.Fatal(err)
	}
	// Gain enough XP for 3 level-ups (Lv1→4 needs XP ≥ 100)
	result, err := ApplyExperienceWithJob(&char, 100, job.Definition{}, zeroRandomSource{}, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.LevelsGained != 3 {
		t.Fatalf("expected 3 levels gained, got %d", result.LevelsGained)
	}
	if char.SP != 3 {
		t.Fatalf("SP after 3 level-ups = %d, want 3", char.SP)
	}
}

func TestApplyExperienceSPBasedSkillLearning(t *testing.T) {
	char, err := character.New("Bob")
	if err != nil {
		t.Fatal(err)
	}
	// Define a skill that is learned when SP == 2
	fireball, err := skill.NewDefinition("fireball", "Fireball", nil, 2, 5,
		corebattle.Effect{Kind: "damage", Power: 30})
	if err != nil {
		t.Fatal(err)
	}
	jobSkills := []skill.Definition{fireball}

	// Gain enough XP to level up to Lv3 (SP will become 2 at Lv3)
	result, err := ApplyExperienceWithJob(&char, 100, job.Definition{}, zeroRandomSource{}, false, jobSkills)
	if err != nil {
		t.Fatal(err)
	}
	if result.LevelsGained != 3 {
		t.Fatalf("expected 3 levels gained, got %d", result.LevelsGained)
	}
	if char.SP != 3 {
		t.Fatalf("SP = %d, want 3", char.SP)
	}
	// Fireball should have been learned exactly once (when SP == 2)
	if len(result.NewlyLearnedSkillIDs) != 1 || result.NewlyLearnedSkillIDs[0] != "fireball" {
		t.Fatalf("newly learned skills = %v, want [fireball]", result.NewlyLearnedSkillIDs)
	}
}

func TestApplyExperienceSkillOrbGivesExtraSPWithProbability(t *testing.T) {
	char, err := character.New("Bob")
	if err != nil {
		t.Fatal(err)
	}
	// Simulate a single level-up; item 157 orb roll = 0 (< 1) → grants extra SP.
	// Sequence: roll for HasSkillOrb (0 = triggers), then stat growth rolls (5 zeros).
	random := &sequenceRandomSource{values: []int{0}}

	result, err := ApplyExperienceWithJob(&char, 10, job.Definition{}, random, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.LevelsGained != 1 {
		t.Fatalf("expected 1 level, got %d", result.LevelsGained)
	}
	// SP = 1 (base) + 1 (skill orb bonus) = 2
	if char.SP != 2 {
		t.Fatalf("SP after skill orb level-up = %d, want 2", char.SP)
	}
}

func TestApplyExperienceSkillOrbNoExtraSPWhenRollFails(t *testing.T) {
	char, err := character.New("Bob")
	if err != nil {
		t.Fatal(err)
	}
	// Roll = 1 (≥ 1) → no extra SP.
	random := &sequenceRandomSource{values: []int{1}}

	result, err := ApplyExperienceWithJob(&char, 10, job.Definition{}, random, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.LevelsGained != 1 {
		t.Fatalf("expected 1 level, got %d", result.LevelsGained)
	}
	if char.SP != 1 {
		t.Fatalf("SP after failed skill orb = %d, want 1", char.SP)
	}
}

// ---------------------------------------------------------------------------
// growthValue cap tests (旧CGI: $v > 9 → rand(9)+1)
// ---------------------------------------------------------------------------

func TestGrowthValueCapsBeyondNineWithReroll(t *testing.T) {
	// First Intn(max+1) returns 10 (> growthCap=9), then re-roll Intn(9) returns 4 → value = 5.
	random := &sequenceRandomSource{values: []int{10, 4}}
	value, err := growthValue(15, random)
	if err != nil {
		t.Fatal(err)
	}
	if value != 5 {
		t.Fatalf("growthValue cap re-roll = %d, want 5", value)
	}
}

func TestGrowthValueNoCapWhenBelowOrEqualNine(t *testing.T) {
	random := &sequenceRandomSource{values: []int{9}}
	value, err := growthValue(9, random)
	if err != nil {
		t.Fatal(err)
	}

	if value != 9 {
		t.Fatalf("growthValue(9) = %d, want 9", value)
	}
}

func TestApplyExperienceStatOrbsAddOneToMatchingGrowthRoll(t *testing.T) {
	char, _ := character.New("OrbBearer")
	char.Stats = character.Stats{}
	def := job.Definition{ID: "orb-job"}
	random := &sequenceRandomSource{values: []int{1, 0, 0, 0, 0}}

	_, err := ApplyExperienceWithJobFull(&char, 10, def, random, ApplyExperienceOptions{
		StatOrbItems: map[string]bool{ItemLifeStatOrb: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if char.Stats.MaxHP != 2 || char.Stats.MaxMP != 0 || char.Stats.Attack != 0 {
		t.Fatalf("stat orb growth = %#v, want HP 2 and other stats 0", char.Stats)
	}
}

func TestStatsClampUsesOverLevelDoubleCaps(t *testing.T) {
	stats := character.Stats{MaxHP: 3000, HP: 3000, MaxMP: 3000, MP: 3000, Attack: 700, Defense: 700, Agility: 700}
	stats.Clamp(false, 99)
	if stats.MaxHP != 999 || stats.Attack != 255 {
		t.Fatalf("standard clamp = %#v", stats)
	}

	stats = character.Stats{MaxHP: 3000, HP: 3000, MaxMP: 3000, MP: 3000, Attack: 700, Defense: 700, Agility: 700}
	stats.Clamp(true, 100)
	if stats.MaxHP != 1998 || stats.Attack != 510 {
		t.Fatalf("over-level clamp = %#v", stats)
	}
}

func TestApplyHappySeed(t *testing.T) {
	if err := ApplyHappySeed(nil); !errors.Is(err, ErrNilCharacter) {
		t.Fatalf("expected ErrNilCharacter, got %v", err)
	}

	char := &character.Character{
		Level:      5,
		Experience: 10,
	}
	if err := ApplyHappySeed(char); err != nil {
		t.Fatalf("ApplyHappySeed failed: %v", err)
	}
	// level 5 * 5 * 10 = 250
	if char.Experience != 250 {
		t.Errorf("expected 250 experience, got %d", char.Experience)
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

type sequenceRandomSource struct {
	values []int
	index  int
}

func (s *sequenceRandomSource) Intn(max int) (int, error) {
	value := s.values[s.index]
	s.index++
	return value, nil
}

type errRandomSource struct{}

func (errRandomSource) Intn(int) (int, error) {
	return 0, errors.New("random generator failed")
}
