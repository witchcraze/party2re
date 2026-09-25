package alchemy

import (
	"context"
	"testing"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/depot"
)

func TestInitialRecipeCatalogValid(t *testing.T) {
	recipes, err := InitialRecipeCatalog()
	if err != nil {
		t.Fatalf("InitialRecipeCatalog error: %v", err)
	}

	items, err := item.InitialCatalog()
	if err != nil {
		t.Fatalf("item.InitialCatalog error: %v", err)
	}

	all := recipes.All()
	if len(all) != 114 {
		t.Fatalf("expected 114 canonical recipes, got %d", len(all))
	}

	seenIDs := make(map[string]bool)
	for _, r := range all {
		if seenIDs[r.ID] {
			t.Errorf("duplicate recipe ID: %s", r.ID)
		}
		seenIDs[r.ID] = true

		if r.Name == "" {
			t.Errorf("recipe %s has empty name", r.ID)
		}
		if r.ResultQuantity <= 0 {
			t.Errorf("recipe %s result quantity <= 0", r.ID)
		}

		// Verify result item exists in Item catalog
		if _, err := items.FindByID(r.ResultItemDefinitionID); err != nil {
			t.Errorf("recipe %s result item %s does not exist in item catalog: %v", r.ID, r.ResultItemDefinitionID, err)
		}

		// Verify all ingredients exist in Item catalog
		if len(r.Ingredients) == 0 {
			t.Errorf("recipe %s has no ingredients", r.ID)
		}
		for _, ing := range r.Ingredients {
			if ing.Quantity <= 0 {
				t.Errorf("recipe %s ingredient %s quantity <= 0", r.ID, ing.DefinitionID)
			}
			if _, err := items.FindByID(ing.DefinitionID); err != nil {
				t.Errorf("recipe %s ingredient %s does not exist in item catalog: %v", r.ID, ing.DefinitionID, err)
			}
		}
	}

	// Verify recipe-113: 魔獣の皮 + 幸せの種 -> 福袋
	r113, err := recipes.FindByID("recipe-113")
	if err != nil {
		t.Fatalf("recipe-113 not found: %v", err)
	}
	if r113.ResultItemDefinitionID != "item-125" {
		t.Errorf("recipe-113 result item = %s, want item-125 (福袋)", r113.ResultItemDefinitionID)
	}
	if len(r113.Ingredients) != 2 || r113.Ingredients[0].DefinitionID != "item-131" || r113.Ingredients[1].DefinitionID != "item-022" {
		t.Errorf("recipe-113 unexpected ingredients: %+v", r113.Ingredients)
	}

	// Verify recipe-114: 祈りの指輪 + 金塊 -> 金の指輪
	r114, err := recipes.FindByID("recipe-114")
	if err != nil {
		t.Fatalf("recipe-114 not found: %v", err)
	}
	if r114.ResultItemDefinitionID != "item-118" {
		t.Errorf("recipe-114 result item = %s, want item-118 (金の指輪)", r114.ResultItemDefinitionID)
	}
	if len(r114.Ingredients) != 2 || r114.Ingredients[0].DefinitionID != "item-012" || r114.Ingredients[1].DefinitionID != "item-134" {
		t.Errorf("recipe-114 unexpected ingredients: %+v", r114.Ingredients)
	}
}

func TestRecipeCatalogOperations(t *testing.T) {
	r1, err := NewRecipe("rec-1", "Test 1", "item-002", 1, []Ingredient{{"item-001", 2}})
	if err != nil {
		t.Fatal(err)
	}
	r2, err := NewRecipe("rec-2", "Test 2", "item-003", 1, []Ingredient{{"item-002", 2}})
	if err != nil {
		t.Fatal(err)
	}

	catalog, err := NewRecipeCatalog([]Recipe{r1, r2})
	if err != nil {
		t.Fatal(err)
	}

	found, err := catalog.FindByID("rec-1")
	if err != nil {
		t.Fatalf("FindByID error: %v", err)
	}
	if found.Name != "Test 1" {
		t.Errorf("recipe name = %s, want 'Test 1'", found.Name)
	}

	_, err = catalog.FindByID("nonexistent")
	if err != ErrRecipeNotFound {
		t.Errorf("expected ErrRecipeNotFound, got %v", err)
	}
}

func TestRestoredRecipes_SynthesisAndLearn(t *testing.T) {
	ctx := context.Background()
	recipes, err := InitialRecipeCatalog()
	if err != nil {
		t.Fatalf("InitialRecipeCatalog error: %v", err)
	}
	items, err := item.InitialCatalog()
	if err != nil {
		t.Fatalf("item.InitialCatalog error: %v", err)
	}

	charRepo := newMemoryCharRepo()
	depotRepo := newMemoryDepotRepo()
	alchemyRepo := newMemoryAlchemyRepo()

	char, _ := corecharacter.New("TestAlchemist")
	charRepo.characters[char.ID] = char

	now := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)
	svc, err := NewService(
		charRepo,
		depotRepo,
		alchemyRepo,
		recipes,
		items,
		WithNowFunc(func() time.Time { return now }),
	)
	if err != nil {
		t.Fatalf("NewService error: %v", err)
	}

	// 1. Learn recipe with 魔獣の皮 and 祈りの指輪 in pool
	_, err = svc.LearnRecipe(ctx, char.ID, []string{"魔獣の皮"})
	if err != nil {
		t.Fatalf("LearnRecipe with 魔獣の皮 failed: %v", err)
	}
	_, err = svc.LearnRecipe(ctx, char.ID, []string{"祈りの指輪"})
	if err != nil {
		t.Fatalf("LearnRecipe with 祈りの指輪 failed: %v", err)
	}

	// 2. Explicitly unlock recipe-113 and recipe-114
	if err := svc.UnlockRecipe(ctx, char.ID, "recipe-113"); err != nil {
		t.Fatalf("UnlockRecipe recipe-113 failed: %v", err)
	}
	if err := svc.UnlockRecipe(ctx, char.ID, "recipe-114"); err != nil {
		t.Fatalf("UnlockRecipe recipe-114 failed: %v", err)
	}

	// 3. Prepare Depot with ingredients for recipe-113 (item-131 + item-022) and recipe-114 (item-012 + item-134)
	dep, _ := depot.NewDepot(char.ID)
	dep.Capacity = 20
	pelt, _ := item.NewInstance("item-131", 1)
	seed, _ := item.NewInstance("item-022", 1)
	ring, _ := item.NewInstance("item-012", 1)
	gold, _ := item.NewInstance("item-134", 1)
	_ = dep.AddItem(pelt)
	_ = dep.AddItem(seed)
	_ = dep.AddItem(ring)
	_ = dep.AddItem(gold)
	_ = depotRepo.Save(ctx, dep)

	// 4. Synthesize recipe-113 -> claim -> produces item-125 (福袋)
	_, err = svc.Synthesize(ctx, char.ID, "recipe-113")
	if err != nil {
		t.Fatalf("Synthesize recipe-113 failed: %v", err)
	}
	if err := svc.CompleteOngoingSynthesis(ctx, char.ID); err != nil {
		t.Fatalf("CompleteOngoingSynthesis failed: %v", err)
	}
	claim113, err := svc.Claim(ctx, char.ID)
	if err != nil {
		t.Fatalf("Claim recipe-113 failed: %v", err)
	}
	if claim113.CreatedItem.DefinitionID != "item-125" {
		t.Errorf("expected result item-125 (福袋), got %s", claim113.CreatedItem.DefinitionID)
	}

	// 5. Synthesize recipe-114 -> claim -> produces item-118 (金の指輪)
	_, err = svc.Synthesize(ctx, char.ID, "recipe-114")
	if err != nil {
		t.Fatalf("Synthesize recipe-114 failed: %v", err)
	}
	if err := svc.CompleteOngoingSynthesis(ctx, char.ID); err != nil {
		t.Fatalf("CompleteOngoingSynthesis failed: %v", err)
	}
	claim114, err := svc.Claim(ctx, char.ID)
	if err != nil {
		t.Fatalf("Claim recipe-114 failed: %v", err)
	}
	if claim114.CreatedItem.DefinitionID != "item-118" {
		t.Errorf("expected result item-118 (金の指輪), got %s", claim114.CreatedItem.DefinitionID)
	}

	comp, err := svc.GetCompendium(ctx, char.ID)
	if err != nil {
		t.Fatalf("GetCompendium failed: %v", err)
	}
	if comp.TotalRecipes != 114 {
		t.Errorf("expected 114 total recipes in compendium, got %d", comp.TotalRecipes)
	}
}
