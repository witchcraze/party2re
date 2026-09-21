package customskill_test

import (
	"context"
	"testing"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/customskill"
)

type synthesisRepo struct {
	skill *customskill.CustomSkill
}

func (r *synthesisRepo) SaveCustomSkill(_ context.Context, skill customskill.CustomSkill) error {
	r.skill = &skill
	return nil
}
func (r *synthesisRepo) FindCustomSkill(context.Context, string) (*customskill.CustomSkill, error) {
	return r.skill, nil
}

type synthesisCharacters struct {
	characters map[string]corecharacter.Character
}

func (r *synthesisCharacters) FindByID(_ context.Context, id string) (corecharacter.Character, error) {
	character, ok := r.characters[id]
	if !ok {
		return corecharacter.Character{}, customskill.ErrCharacterNotFound
	}
	return character, nil
}

type synthesisGems map[string]customskill.GemDefinition

func (g synthesisGems) FindGemByID(id string) (customskill.GemDefinition, bool) {
	value, ok := g[id]
	return value, ok
}

type synthesisInventory struct{ value coreinventory.Inventory }

func (r *synthesisInventory) FindByCharacterIDForUpdate(context.Context, string) (coreinventory.Inventory, error) {
	return r.value, nil
}
func (r *synthesisInventory) Save(_ context.Context, value coreinventory.Inventory) error {
	r.value = value
	return nil
}

func TestSetCustomSkillSynthesizesGemsAndReturnsPreviousSelection(t *testing.T) {
	repo := &synthesisRepo{}
	inventory, err := coreinventory.New("char-1")
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"ruby", "diamond"} {
		instance, createErr := coreitem.NewInstance(id, 1)
		if createErr != nil {
			t.Fatal(createErr)
		}
		if err := inventory.Add(instance); err != nil {
			t.Fatal(err)
		}
	}
	invRepo := &synthesisInventory{value: inventory}
	chars := &synthesisCharacters{characters: map[string]corecharacter.Character{
		"char-1": {ID: "char-1", Stats: corecharacter.Stats{MaxMP: 20}},
	}}
	service, err := customskill.NewService(repo, chars)
	if err != nil {
		t.Fatal(err)
	}
	service.ConfigureGemSynthesis(synthesisGems{
		"ruby":    {ID: "ruby", SlotCost: 1, MPCost: 4},
		"diamond": {ID: "diamond", SlotCost: 2, MPCost: 8},
	}, invRepo, nil)

	first, err := service.SetCustomSkill(context.Background(), "char-1", "炎の舞", "いくぞ", [3]string{"ruby", "", ""})
	if err != nil {
		t.Fatal(err)
	}
	if first.CMP != 4 || invRepo.value.Quantity("ruby") != 0 {
		t.Fatalf("unexpected first synthesis: %+v, inventory=%+v", first, invRepo.value)
	}

	second, err := service.SetCustomSkill(context.Background(), "char-1", "星の雨", "", [3]string{"diamond", "", ""})
	if err != nil {
		t.Fatal(err)
	}
	if second.CMP != 8 || invRepo.value.Quantity("ruby") != 1 || invRepo.value.Quantity("diamond") != 0 {
		t.Fatalf("previous gem was not swapped: %+v, inventory=%+v", second, invRepo.value)
	}
}

func TestSetCustomSkillRejectsInvalidNameAndLimits(t *testing.T) {
	repo := &synthesisRepo{}
	inventory, _ := coreinventory.New("char-1")
	for _, id := range []string{"a", "a", "c", "c"} {
		instance, _ := coreitem.NewInstance(id, 1)
		_ = inventory.Add(instance)
	}
	invRepo := &synthesisInventory{value: inventory}
	chars := &synthesisCharacters{characters: map[string]corecharacter.Character{
		"char-1": {ID: "char-1", Stats: corecharacter.Stats{MaxMP: 5}},
	}}
	service, _ := customskill.NewService(repo, chars)
	service.ConfigureGemSynthesis(synthesisGems{
		"a": {ID: "a", SlotCost: 2, MPCost: 3},
		"b": {ID: "b", SlotCost: 2, MPCost: 3},
		"c": {ID: "c", SlotCost: 1, MPCost: 3},
	}, invRepo, nil)

	if _, err := service.SetCustomSkill(context.Background(), "char-1", "こうげき", "", [3]string{}); err != customskill.ErrInvalidSkillName {
		t.Fatalf("expected reserved name rejection, got %v", err)
	}
	if _, err := service.SetCustomSkill(context.Background(), "char-1", "valid", "", [3]string{"a", "a", ""}); err != customskill.ErrTooManyGemSlots {
		t.Fatalf("expected slot rejection, got %v", err)
	}
	if _, err := service.SetCustomSkill(context.Background(), "char-1", "valid", "", [3]string{"c", "c", ""}); err != customskill.ErrCMPTooHigh {
		t.Fatalf("expected CMP rejection, got %v", err)
	}
}
