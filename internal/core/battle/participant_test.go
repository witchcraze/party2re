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

func TestMustNewParticipant(t *testing.T) {
	// Happy path
	p := battle.MustNewParticipant("valid-id", 100, 20, 10)
	if p.ID != "valid-id" || p.HP != 100 || p.Attack != 20 || p.Defense != 10 {
		t.Errorf("unexpected participant: %#v", p)
	}

	// Panic on invalid HP
	defer func() {
		r := recover()
		if r == nil {
			t.Error("expected panic for invalid participant HP <= 0, but did not panic")
		}
	}()
	battle.MustNewParticipant("invalid-id", 0, 20, 10)
}

func TestNewParticipantFromCharacterWithHP(t *testing.T) {
	// 1. currentHP > 0
	char1 := corecharacter.Character{
		ID:   "char-1",
		Name: "Hero",
		Stats: corecharacter.Stats{
			HP:      50,
			MaxHP:   100,
			Attack:  30,
			Defense: 15,
		},
	}
	p1 := battle.NewParticipantFromCharacterWithHP(char1, 40)
	if p1.HP != 40 {
		t.Errorf("expected HP=40 from override, got %d", p1.HP)
	}

	// 2. currentHP <= 0, char.Stats.HP > 0
	char2 := corecharacter.Character{
		ID:   "char-2",
		Name: "Hero2",
		Stats: corecharacter.Stats{
			HP:      60,
			MaxHP:   100,
			Attack:  30,
			Defense: 15,
		},
	}
	p2 := battle.NewParticipantFromCharacterWithHP(char2, 0)
	if p2.HP != 60 {
		t.Errorf("expected HP=60 from char.Stats.HP, got %d", p2.HP)
	}

	// 3. currentHP <= 0, char.Stats.HP <= 0, char.Stats.MaxHP > 0
	char3 := corecharacter.Character{
		ID:   "char-3",
		Name: "Hero3",
		Stats: corecharacter.Stats{
			HP:      0,
			MaxHP:   80,
			Attack:  30,
			Defense: 15,
		},
	}
	p3 := battle.NewParticipantFromCharacterWithHP(char3, -5)
	if p3.HP != 80 {
		t.Errorf("expected HP=80 from char.Stats.MaxHP, got %d", p3.HP)
	}
}

func TestParticipantBuilder_MethodsAndErrors(t *testing.T) {
	// WithName and WithStats
	b := battle.NewParticipantBuilder("hero-base").
		WithName("Sir Hero").
		WithStats(120, 45, 25)

	p, err := b.Build()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.ID != "hero-base" || p.Name != "Sir Hero" || p.HP != 120 || p.Attack != 45 || p.Defense != 25 {
		t.Errorf("unexpected participant: %#v", p)
	}

	// FromCharacter with HP <= 0 (fallback to MaxHP)
	charFallen := corecharacter.Character{
		ID:   "fallen",
		Name: "Ghost",
		Stats: corecharacter.Stats{
			HP:      0,
			MaxHP:   90,
			Attack:  10,
			Defense: 5,
		},
	}
	pFallen := battle.NewParticipantBuilder("").FromCharacter(charFallen).MustBuild()
	if pFallen.HP != 90 {
		t.Errorf("expected fallback to MaxHP 90, got %d", pFallen.HP)
	}

	// Build error: empty ID
	_, err = battle.NewParticipantBuilder("   ").WithStats(100, 10, 5).Build()
	if err == nil {
		t.Error("expected error for empty ID in builder")
	}

	// Build error: HP = 0
	_, err = battle.NewParticipantBuilder("valid").WithStats(0, 10, 5).Build()
	if err == nil {
		t.Error("expected error for HP=0 in builder")
	}

	// MustBuild panic on error
	defer func() {
		r := recover()
		if r == nil {
			t.Error("expected panic in MustBuild for invalid participant, but did not panic")
		}
	}()
	battle.NewParticipantBuilder("bad").WithStats(0, 10, 5).MustBuild()
}
