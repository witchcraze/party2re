package customskill_test

import (
	"context"
	"testing"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/customskill"
	"github.com/witchcraze/party2re/internal/gemstore"
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

type synthesisGemBox struct {
	box gemstore.GemBox
}

func (r *synthesisGemBox) FindByCharacterIDForUpdate(context.Context, string) (gemstore.GemBox, error) {
	return r.box, nil
}
func (r *synthesisGemBox) Save(_ context.Context, value gemstore.GemBox) error {
	r.box = value
	return nil
}

func countGems(items []coreitem.Instance, definitionID string) int {
	count := 0
	for _, inst := range items {
		if inst.DefinitionID == definitionID {
			count++
		}
	}
	return count
}

func TestSetCustomSkillSynthesizesGemsAndReturnsPreviousSelection(t *testing.T) {
	repo := &synthesisRepo{}
	var items []coreitem.Instance
	for _, id := range []string{"ruby", "diamond"} {
		instance, err := coreitem.NewInstance(id, 1)
		if err != nil {
			t.Fatal(err)
		}
		items = append(items, instance)
	}
	boxRepo := &synthesisGemBox{
		box: gemstore.GemBox{
			CharacterID: "char-1",
			Capacity:    10,
			Items:       items,
		},
	}
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
	}, boxRepo, nil)

	first, err := service.SetCustomSkill(context.Background(), "char-1", "炎の舞", "いくぞ", [3]string{"ruby", "", ""})
	if err != nil {
		t.Fatal(err)
	}
	if first.CMP != 4 || countGems(boxRepo.box.Items, "ruby") != 0 {
		t.Fatalf("unexpected first synthesis: %+v, gembox=%+v", first, boxRepo.box)
	}

	second, err := service.SetCustomSkill(context.Background(), "char-1", "星の雨", "", [3]string{"diamond", "", ""})
	if err != nil {
		t.Fatal(err)
	}
	if second.CMP != 8 || countGems(boxRepo.box.Items, "ruby") != 1 || countGems(boxRepo.box.Items, "diamond") != 0 {
		t.Fatalf("previous gem was not swapped: %+v, gembox=%+v", second, boxRepo.box)
	}
}

func TestSetCustomSkillRejectsInvalidNameAndLimits(t *testing.T) {
	repo := &synthesisRepo{}
	var items []coreitem.Instance
	for _, id := range []string{"a", "a", "c", "c"} {
		instance, _ := coreitem.NewInstance(id, 1)
		items = append(items, instance)
	}
	boxRepo := &synthesisGemBox{
		box: gemstore.GemBox{
			CharacterID: "char-1",
			Capacity:    10,
			Items:       items,
		},
	}
	chars := &synthesisCharacters{characters: map[string]corecharacter.Character{
		"char-1": {ID: "char-1", Stats: corecharacter.Stats{MaxMP: 5}},
	}}
	service, _ := customskill.NewService(repo, chars)
	service.ConfigureGemSynthesis(synthesisGems{
		"a": {ID: "a", SlotCost: 2, MPCost: 3},
		"b": {ID: "b", SlotCost: 2, MPCost: 3},
		"c": {ID: "c", SlotCost: 1, MPCost: 3},
	}, boxRepo, nil)

	if _, err := service.SetCustomSkill(context.Background(), "char-1", "こうげき", "", [3]string{}); err != customskill.ErrInvalidSkillName {
		t.Fatalf("expected reserved name rejection, got %v", err)
	}
	if _, err := service.SetCustomSkill(context.Background(), "char-1", "valid", "", [3]string{"a", "a", ""}); err != customskill.ErrTooManyGemSlots {
		t.Fatalf("expected slot rejection, got %v", err)
	}
	if _, err := service.SetCustomSkill(context.Background(), "char-1", "valid", "", [3]string{"b", "", ""}); err != customskill.ErrGemNotOwned {
		t.Fatalf("expected gem not owned rejection, got %v", err)
	}
}
