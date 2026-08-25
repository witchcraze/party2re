package battle_test

import (
	"testing"

	"github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

func TestNewParticipant(t *testing.T) {
	p, err := battle.NewParticipant("hero-1", 100, 50, 30)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.ID != "hero-1" || p.HP != 100 || p.Attack != 50 || p.Defense != 30 {
		t.Errorf("unexpected participant: %#v", p)
	}

	_, err = battle.NewParticipant("", 100, 50, 30)
	if err == nil {
		t.Error("expected error for empty ID")
	}

	_, err = battle.NewParticipant("hero-1", 0, 50, 30)
	if err == nil {
		t.Error("expected error for 0 HP")
	}
}

func TestNewParticipantFromCharacter(t *testing.T) {
	char := corecharacter.Character{
		ID:   "char-123",
		Name: "Alice",
		Stats: corecharacter.Stats{
			HP:      80,
			MaxHP:   100,
			Attack:  45,
			Defense: 25,
		},
	}

	p := battle.NewParticipantFromCharacter(char)
	if p.ID != "char-123" || p.HP != 80 || p.Attack != 45 || p.Defense != 25 {
		t.Errorf("unexpected participant from character: %#v", p)
	}

	// Fallback to MaxHP when HP <= 0
	char.Stats.HP = 0
	p2 := battle.NewParticipantFromCharacter(char)
	if p2.HP != 100 {
		t.Errorf("expected fallback to MaxHP (100), got %d", p2.HP)
	}
}

func TestParticipantBuilder(t *testing.T) {
	char := corecharacter.Character{
		ID:   "char-builder",
		Name: "Bob",
		Stats: corecharacter.Stats{
			HP:      50,
			MaxHP:   100,
			Attack:  30,
			Defense: 20,
		},
	}

	p := battle.NewParticipantBuilder("").
		FromCharacter(char).
		WithCurrentHP(75).
		MustBuild()

	if p.ID != "char-builder" || p.HP != 75 || p.Attack != 30 || p.Defense != 20 {
		t.Errorf("unexpected built participant: %#v", p)
	}
}
