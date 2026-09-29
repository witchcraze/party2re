package home

import (
	"context"
	"errors"
	"testing"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
)

type stubRecipeLearner struct {
	learnFn func(ctx context.Context, characterID string, allowedBases []string) (LearnedRecipeInfo, error)
}

func (l *stubRecipeLearner) LearnRecipe(ctx context.Context, characterID string, allowedBases []string) (LearnedRecipeInfo, error) {
	if l.learnFn != nil {
		return l.learnFn(ctx, characterID, allowedBases)
	}
	return LearnedRecipeInfo{}, errors.New("not implemented")
}

func TestUseHomeItem_RecipeScrolls(t *testing.T) {
	ctx := context.Background()
	char := corecharacter.Character{ID: "char-1", Name: "Alchemist"}

	cat, err := coreitem.DefaultCatalog()
	if err != nil {
		t.Fatalf("failed to load catalog: %v", err)
	}

	charReader := &mockCharReader{chars: map[string]corecharacter.Character{"char-1": char}}
	charUpdater := &mockCharUpdater{chars: map[string]corecharacter.Character{"char-1": char}}

	t.Run("basic recipe scroll item-127 success", func(t *testing.T) {
		inv := coreinventory.Inventory{
			CharacterID: "char-1",
			Items:       []coreitem.Instance{{ID: "inst-127", DefinitionID: "item-127", Quantity: 1}},
		}
		invMgr := &mockInventoryManager{invs: map[string]coreinventory.Inventory{"char-1": inv}}

		learner := &stubRecipeLearner{
			learnFn: func(ctx context.Context, characterID string, allowedBases []string) (LearnedRecipeInfo, error) {
				if len(allowedBases) != len(basicRecipeBases) {
					t.Errorf("expected basic pool of %d bases, got %d", len(basicRecipeBases), len(allowedBases))
				}
				return LearnedRecipeInfo{
					ID:              "recipe-001",
					Name:            "上薬草の合成",
					Ingredient1Name: "薬草",
					Ingredient2Name: "薬草",
				}, nil
			},
		}

		svc, _ := NewService(
			&mockHomeRepo{},
			charReader,
			WithInventoryManager(invMgr),
			WithItemCatalog(cat),
			WithCharacterUpdater(charUpdater),
		)
		svc.SetRecipeLearner(learner)

		res, err := svc.UseHomeItem(ctx, "char-1", "inst-127", "inventory")
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		if res.Action != "consumed" || !res.Consumed {
			t.Errorf("expected consumed result, got %+v", res)
		}
		expectedMsg := "Alchemist は錬金レシピを読んだ！【薬草 × 薬草 ＝ ？？？】の錬金方法を習得した！"
		if res.Message != expectedMsg {
			t.Errorf("expected message %q, got %q", expectedMsg, res.Message)
		}
	})

	t.Run("applied recipe scroll item-128 success", func(t *testing.T) {
		inv := coreinventory.Inventory{
			CharacterID: "char-1",
			Items:       []coreitem.Instance{{ID: "inst-128", DefinitionID: "item-128", Quantity: 1}},
		}
		invMgr := &mockInventoryManager{invs: map[string]coreinventory.Inventory{"char-1": inv}}

		learner := &stubRecipeLearner{
			learnFn: func(ctx context.Context, characterID string, allowedBases []string) (LearnedRecipeInfo, error) {
				if len(allowedBases) != len(appliedRecipeBases) {
					t.Errorf("expected applied pool of %d bases, got %d", len(appliedRecipeBases), len(allowedBases))
				}
				return LearnedRecipeInfo{
					ID:              "recipe-050",
					Name:            "世界樹のしずく",
					Ingredient1Name: "世界樹の葉",
					Ingredient2Name: "魔法の聖水",
				}, nil
			},
		}

		svc, _ := NewService(
			&mockHomeRepo{},
			charReader,
			WithInventoryManager(invMgr),
			WithItemCatalog(cat),
			WithCharacterUpdater(charUpdater),
		)
		svc.SetRecipeLearner(learner)

		res, err := svc.UseHomeItem(ctx, "char-1", "inst-128", "inventory")
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		expectedMsg := "Alchemist は錬金レシピを読んだ！【世界樹の葉 × 魔法の聖水 ＝ ？？？】の錬金方法を習得した！"
		if res.Message != expectedMsg {
			t.Errorf("expected message %q, got %q", expectedMsg, res.Message)
		}
	})

	t.Run("god recipe scroll item-129 success", func(t *testing.T) {
		inv := coreinventory.Inventory{
			CharacterID: "char-1",
			Items:       []coreitem.Instance{{ID: "inst-129", DefinitionID: "item-129", Quantity: 1}},
		}
		invMgr := &mockInventoryManager{invs: map[string]coreinventory.Inventory{"char-1": inv}}

		learner := &stubRecipeLearner{
			learnFn: func(ctx context.Context, characterID string, allowedBases []string) (LearnedRecipeInfo, error) {
				if allowedBases != nil {
					t.Errorf("expected nil allowedBases for god recipe, got %v", allowedBases)
				}
				return LearnedRecipeInfo{
					ID:              "recipe-100",
					Name:            "賢者の石",
					Ingredient1Name: "オリハルコン",
					Ingredient2Name: "太陽の鏡",
				}, nil
			},
		}

		svc, _ := NewService(
			&mockHomeRepo{},
			charReader,
			WithInventoryManager(invMgr),
			WithItemCatalog(cat),
			WithCharacterUpdater(charUpdater),
		)
		svc.SetRecipeLearner(learner)

		res, err := svc.UseHomeItem(ctx, "char-1", "inst-129", "inventory")
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		expectedMsg := "Alchemist は錬金レシピを読んだ！【オリハルコン × 太陽の鏡 ＝ ？？？】の錬金方法を習得した！"
		if res.Message != expectedMsg {
			t.Errorf("expected message %q, got %q", expectedMsg, res.Message)
		}
	})

	t.Run("recipe pool exhausted consumes item with exhaustion message", func(t *testing.T) {
		inv := coreinventory.Inventory{
			CharacterID: "char-1",
			Items:       []coreitem.Instance{{ID: "inst-127", DefinitionID: "item-127", Quantity: 1}},
		}
		invMgr := &mockInventoryManager{invs: map[string]coreinventory.Inventory{"char-1": inv}}

		learner := &stubRecipeLearner{
			learnFn: func(ctx context.Context, characterID string, allowedBases []string) (LearnedRecipeInfo, error) {
				return LearnedRecipeInfo{}, ErrNoRecipesToLearn
			},
		}

		svc, _ := NewService(
			&mockHomeRepo{},
			charReader,
			WithInventoryManager(invMgr),
			WithItemCatalog(cat),
			WithCharacterUpdater(charUpdater),
		)
		svc.SetRecipeLearner(learner)

		res, err := svc.UseHomeItem(ctx, "char-1", "inst-127", "inventory")
		if err != nil {
			t.Fatalf("expected success on exhausted scroll, got error: %v", err)
		}
		if res.Action != "consumed" || !res.Consumed {
			t.Errorf("expected item consumed even when pool exhausted, got %+v", res)
		}
		expectedMsg := "この錬金レシピからこれ以上習得できる錬金方法はないようだ…"
		if res.Message != expectedMsg {
			t.Errorf("expected exhaustion message %q, got %q", expectedMsg, res.Message)
		}
	})
}
