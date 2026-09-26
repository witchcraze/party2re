package customskill_test

import (
	"context"
	"errors"
	"testing"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/customskill"
	"github.com/witchcraze/party2re/internal/gemstore"
)

func TestSetCustomSkillAllowsCMPExceedingMaxMPWhenSlotsValid(t *testing.T) {
	repo := &synthesisRepo{}
	var items []coreitem.Instance
	for _, id := range []string{"heavy_gem", "heavy_gem"} {
		instance, err := coreitem.NewInstance(id, 1)
		if err != nil {
			t.Fatal(err)
		}
		items = append(items, instance)
	}
	boxRepo := &synthesisGemBox{
		box: gemstore.GemBox{
			CharacterID: "char-low-mp",
			Capacity:    10,
			Items:       items,
		},
	}
	// Character has MaxMP = 5, but CMP total will be 6 + 6 = 12
	chars := &synthesisCharacters{characters: map[string]corecharacter.Character{
		"char-low-mp": {ID: "char-low-mp", Stats: corecharacter.Stats{MaxMP: 5}},
	}}
	service, err := customskill.NewService(repo, chars)
	if err != nil {
		t.Fatal(err)
	}
	service.ConfigureGemSynthesis(synthesisGems{
		"heavy_gem": {ID: "heavy_gem", SlotCost: 1, MPCost: 6},
	}, boxRepo, nil)

	// In legacy specs, CMP is an independent growth stat; setting a skill whose CMP exceeds battle MaxMP is valid.
	skill, err := service.SetCustomSkill(context.Background(), "char-low-mp", "大魔法", "くらえ", [3]string{"heavy_gem", "heavy_gem", ""})
	if err != nil {
		t.Fatalf("expected synthesis to succeed even when CMP > MaxMP, got error: %v", err)
	}
	if skill.CMP != 12 {
		t.Fatalf("expected CMP 12, got %d", skill.CMP)
	}
}

func TestSetCustomSkillRejectsWhenGemBoxCapacityExceededOnReturn(t *testing.T) {
	repo := &synthesisRepo{}
	// Initially, previous skill has 2 gems equipped
	repo.skill = &customskill.CustomSkill{
		CharacterID: "char-cap",
		Name:        "旧スキル",
		CMP:         4,
		Gems:        [3]string{"g1", "g2", ""},
	}

	// GemBox already has 5 items and capacity is 5 (full)
	var items []coreitem.Instance
	for i := 0; i < 5; i++ {
		inst, _ := coreitem.NewInstance("dummy", 1)
		items = append(items, inst)
	}
	boxRepo := &synthesisGemBox{
		box: gemstore.GemBox{
			CharacterID: "char-cap",
			Capacity:    5,
			Items:       items,
		},
	}
	chars := &synthesisCharacters{characters: map[string]corecharacter.Character{
		"char-cap": {ID: "char-cap", JobLevel: 0}, // MinGemBoxCapacity = 5
	}}
	service, err := customskill.NewService(repo, chars)
	if err != nil {
		t.Fatal(err)
	}
	service.ConfigureGemSynthesis(synthesisGems{
		"g1": {ID: "g1", SlotCost: 1, MPCost: 2},
		"g2": {ID: "g2", SlotCost: 1, MPCost: 2},
	}, boxRepo, nil)

	// Unequipping both gems (new skill has 0 gems) would return 2 gems into a box that is already 5/5
	_, err = service.SetCustomSkill(context.Background(), "char-cap", "空スキル", "", [3]string{"", "", ""})
	if !errors.Is(err, gemstore.ErrGemBoxFull) {
		t.Fatalf("expected ErrGemBoxFull when returning gems to full box, got %v", err)
	}
}
