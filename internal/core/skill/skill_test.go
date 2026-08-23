package skill

import (
	"errors"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

func TestSkillUseChecksConditionsConsumesMPAndReturnsBattleEffect(t *testing.T) {
	value, err := corecharacter.NewWithOptions("Alice", "vanguard", "unspecified", nil)
	if err != nil {
		t.Fatal(err)
	}
	value.Stats.MP = 5
	definition, err := NewDefinition("power-strike", "Power Strike", []string{"vanguard"}, 1, 3,
		corebattle.Effect{Kind: "damage", Power: 10})
	if err != nil {
		t.Fatal(err)
	}

	effect, err := definition.Use(UseRequest{Character: &value})
	if err != nil {
		t.Fatal(err)
	}
	if effect.Kind != "damage" || effect.Power != 10 || value.Stats.MP != 2 {
		t.Fatalf("Use() effect = %#v, character = %#v", effect, value)
	}
}

func TestSkillRejectsUnavailableConditionsAndInsufficientMP(t *testing.T) {
	value, _ := corecharacter.New("Alice")
	definition, _ := NewDefinition("locked", "Locked Skill", []string{"vanguard"}, 5, 2,
		corebattle.Effect{Kind: "damage", Power: 1})
	if err := definition.CanUse(UseRequest{Character: &value}); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("wrong job/level error = %v", err)
	}
	value.JobID = "vanguard"
	value.Level = 5
	value.Stats.MP = 1
	if err := definition.CanUse(UseRequest{Character: &value}); !errors.Is(err, ErrInsufficientMP) {
		t.Fatalf("insufficient MP error = %v", err)
	}
}

func TestSkillRequiresOwnedItemWhenConfigured(t *testing.T) {
	value, _ := corecharacter.New("Alice")
	definition, _ := NewDefinition("item-skill", "Item Skill", nil, 1, 0,
		corebattle.Effect{Kind: "heal", Power: 5})
	if err := definition.CanUse(UseRequest{
		Character:      &value,
		RequiredItemID: "focus-item",
		HasItem:        func(string) bool { return false },
	}); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("missing item error = %v", err)
	}
}

func TestNewDefinitionValidation(t *testing.T) {
	tests := []struct {
		id     string
		name   string
		level  int
		mpCost int
		effect corebattle.Effect
	}{
		{id: "", name: "Name", level: 1, mpCost: 1, effect: corebattle.Effect{Kind: "heal", Power: 1}},
		{id: "id", name: "", level: 1, mpCost: 1, effect: corebattle.Effect{Kind: "heal", Power: 1}},
		{id: "id", name: "Name", level: 0, mpCost: 1, effect: corebattle.Effect{Kind: "heal", Power: 1}},
		{id: "id", name: "Name", level: 1, mpCost: -1, effect: corebattle.Effect{Kind: "heal", Power: 1}},
		{id: "id", name: "Name", level: 1, mpCost: 1, effect: corebattle.Effect{Kind: "", Power: 1}},
		{id: "id", name: "Name", level: 1, mpCost: 1, effect: corebattle.Effect{Kind: "heal", Power: -1}},
	}
	for _, test := range tests {
		if _, err := NewDefinition(test.id, test.name, nil, test.level, test.mpCost, test.effect); !errors.Is(err, ErrInvalidDefinition) {
			t.Errorf("NewDefinition(%#v) error = %v, want %v", test, err, ErrInvalidDefinition)
		}
	}
}

func TestSkillCanUseAndUseNilCharacter(t *testing.T) {
	definition, err := NewDefinition("heal", "Heal", nil, 1, 1, corebattle.Effect{Kind: "heal", Power: 5})
	if err != nil {
		t.Fatal(err)
	}
	if err := definition.CanUse(UseRequest{Character: nil}); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("CanUse(nil character) error = %v, want %v", err, ErrUnavailable)
	}
	if _, err := definition.Use(UseRequest{Character: nil}); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Use(nil character) error = %v, want %v", err, ErrUnavailable)
	}
}

func TestSkillUseInsufficientMPLeavesMPUnchanged(t *testing.T) {
	value, err := corecharacter.New("Alice")
	if err != nil {
		t.Fatal(err)
	}
	value.Stats.MP = 2
	definition, err := NewDefinition("heavy-strike", "Heavy Strike", nil, 1, 5, corebattle.Effect{Kind: "damage", Power: 20})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := definition.Use(UseRequest{Character: &value}); !errors.Is(err, ErrInsufficientMP) {
		t.Fatalf("Use(insufficient MP) error = %v, want %v", err, ErrInsufficientMP)
	}
	if value.Stats.MP != 2 {
		t.Fatalf("MP modified on failed Use: %d, want 2", value.Stats.MP)
	}
}
