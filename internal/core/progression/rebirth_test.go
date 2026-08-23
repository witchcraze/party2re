package progression_test

import (
	"errors"
	"testing"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/progression"
)

func TestRebirthSuccess(t *testing.T) {
	char, err := corecharacter.New("Level 99 Legend")
	if err != nil {
		t.Fatal(err)
	}
	char.Level = 99
	char.Experience = 1000000

	if err := progression.Rebirth(&char); err != nil {
		t.Fatalf("Rebirth error: %v", err)
	}

	if char.Level != 1 {
		t.Errorf("level after rebirth = %d, want 1", char.Level)
	}
	if char.Experience != 0 {
		t.Errorf("experience after rebirth = %d, want 0", char.Experience)
	}
	if char.RebirthCount != 1 {
		t.Errorf("rebirth count = %d, want 1", char.RebirthCount)
	}

	// Verify stats with 1st rebirth bonus (+5)
	expectedHP := progression.BaseInitialHP + 5*2
	if char.Stats.MaxHP != expectedHP || char.Stats.HP != expectedHP {
		t.Errorf("stats HP = %d, want %d", char.Stats.HP, expectedHP)
	}
	expectedStat := progression.BaseInitialStat + 5
	if char.Stats.Attack != expectedStat || char.Stats.Defense != expectedStat || char.Stats.Agility != expectedStat {
		t.Errorf("stats Attack = %d, want %d", char.Stats.Attack, expectedStat)
	}
}

func TestRebirthRejectsUnderleveled(t *testing.T) {
	char, err := corecharacter.New("Novice")
	if err != nil {
		t.Fatal(err)
	}
	char.Level = 98

	err = progression.Rebirth(&char)
	if !errors.Is(err, progression.ErrRebirthNotEligible) {
		t.Errorf("expected ErrRebirthNotEligible, got %v", err)
	}
}

func TestRebirthNilCharacter(t *testing.T) {
	err := progression.Rebirth(nil)
	if !errors.Is(err, progression.ErrNilCharacter) {
		t.Errorf("expected ErrNilCharacter, got %v", err)
	}
}
