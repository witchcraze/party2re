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
