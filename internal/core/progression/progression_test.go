package progression

import (
	"errors"
	"testing"

	"github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/job"
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

	levels, err := ApplyExperienceWithJob(&value, 10, definition, random)
	if err != nil {
		t.Fatal(err)
	}
	if levels != 1 || value.Level != 2 {
		t.Fatalf("level result = %d, character = %#v", levels, value)
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

	levels, err := ApplyExperienceWithJob(&value, 10, definition, &sequenceRandomSource{values: []int{0, 0, 0, 0, 0}})
	if err != nil {
		t.Fatal(err)
	}
	if levels != 1 || value.Stats.MaxHP != 31 {
		t.Fatalf("level result = %d, stats = %#v", levels, value.Stats)
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

	levels, err := ApplyExperienceWithJob(&value, 100, definition, random)
	if err != nil {
		t.Fatal(err)
	}
	if levels != 3 || value.Level != 4 {
		t.Fatalf("level result = %d, character = %#v", levels, value)
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
	if _, err := ApplyExperienceWithJob(&value, 10, job.Definition{ID: "broken", HPGrowth: -1}, &sequenceRandomSource{}); !errors.Is(err, ErrInvalidGrowth) {
		t.Fatalf("invalid growth error = %v, want %v", err, ErrInvalidGrowth)
	}
	definition, _ := job.NewDefinition("novice", "Novice", 1, 1, 1, 1, 1, 1, "")
	if _, err := ApplyExperienceWithJob(&value, 10, definition, nil); !errors.Is(err, ErrInvalidGrowth) {
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
	if _, err := ApplyExperienceWithJob(&value, 10, definition, errRandom); err == nil {
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
		if _, err := ApplyExperienceWithJob(&char, 10, def, random); !errors.Is(err, ErrInvalidGrowth) {
			t.Fatalf("ApplyExperienceWithJob(%#v) error = %v, want %v", def, err, ErrInvalidGrowth)
		}
	}
}

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
