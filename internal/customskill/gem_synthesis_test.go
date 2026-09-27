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

func TestSetCustomSkill_OverCapacityPostJobChangeAllowed(t *testing.T) {
	repo := &synthesisRepo{
		skill: &customskill.CustomSkill{
			CharacterID: "char-overcap",
			Name:        "旧スキル",
			CMP:         8,
			Gems:        [3]string{"gem-a", "gem-b", ""},
		},
	}
	// Player has 13 gems in GemBox + 2 equipped = 15 total gems (acquired when JobLevel was 20)
	var items []coreitem.Instance
	for i := 0; i < 12; i++ {
		inst, _ := coreitem.NewInstance("dummy", 1)
		items = append(items, inst)
	}
	gemC, _ := coreitem.NewInstance("gem-c", 1)
	items = append(items, gemC)

	boxRepo := &synthesisGemBox{
		box: gemstore.GemBox{
			CharacterID: "char-overcap",
			Capacity:    10, // Dynamic capacity reset to 10 due to JobLevel = 1
			Items:       items,
		},
	}
	// Character underwent job change: JobLevel reset to 1
	chars := &synthesisCharacters{characters: map[string]corecharacter.Character{
		"char-overcap": {ID: "char-overcap", JobLevel: 1, Stats: corecharacter.Stats{MaxMP: 50}},
	}}
	service, err := customskill.NewService(repo, chars)
	if err != nil {
		t.Fatal(err)
	}
	service.ConfigureGemSynthesis(synthesisGems{
		"gem-a": {ID: "gem-a", SlotCost: 1, MPCost: 4},
		"gem-b": {ID: "gem-b", SlotCost: 1, MPCost: 4},
		"gem-c": {ID: "gem-c", SlotCost: 1, MPCost: 5},
	}, boxRepo, nil)

	// Case 1: Swapping gem-b for gem-c (net count delta = 0), box count = 13 > capacity 10
	skill, err := service.SetCustomSkill(context.Background(), "char-overcap", "新スキル", "スワップ", [3]string{"gem-a", "gem-c", ""})
	if err != nil {
		t.Fatalf("expected gem swap to succeed over capacity, got %v", err)
	}
	if skill.CMP != 9 || skill.Gems != [3]string{"gem-a", "gem-c", ""} {
		t.Fatalf("unexpected skill after swap: %+v", skill)
	}

	// Case 2: Unsetting gem-c (net count delta = +1, returns to box), box count = 14 > capacity 10
	skill2, err := service.SetCustomSkill(context.Background(), "char-overcap", "単一スキル", "解除", [3]string{"gem-a", "", ""})
	if err != nil {
		t.Fatalf("expected unsetting gem to succeed over capacity, got %v", err)
	}
	if skill2.CMP != 4 || skill2.Gems != [3]string{"gem-a", "", ""} {
		t.Fatalf("unexpected skill after unsetting: %+v", skill2)
	}
	if len(boxRepo.box.Items) != 14 {
		t.Fatalf("expected 14 items in GemBox, got %d", len(boxRepo.box.Items))
	}

	// Case 3: Editing only name and comment without gem changes
	skill3, err := service.SetCustomSkill(context.Background(), "char-overcap", "改名スキル", "コメント変更", [3]string{"gem-a", "", ""})
	if err != nil {
		t.Fatalf("expected editing text to succeed over capacity, got %v", err)
	}
	if skill3.Name != "改名スキル" || skill3.Comment != "コメント変更" {
		t.Fatalf("unexpected text after edit: %+v", skill3)
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
