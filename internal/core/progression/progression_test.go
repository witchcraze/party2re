package progression

import (
	"errors"
	"testing"

	"github.com/witchcraze/party2re/internal/core/character"
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
