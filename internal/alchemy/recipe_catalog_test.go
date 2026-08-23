package alchemy

import (
	"testing"

	"github.com/witchcraze/party2re/internal/core/item"
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
	if len(all) == 0 {
		t.Fatal("recipe catalog is empty")
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
		if r.GoldFee < 0 {
			t.Errorf("recipe %s gold fee < 0", r.ID)
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
}

func TestRecipeCatalogOperations(t *testing.T) {
	r1, err := NewRecipe("rec-1", "Test 1", "item-002", 1, []Ingredient{{"item-001", 2}}, 50)
	if err != nil {
		t.Fatal(err)
	}
	r2, err := NewRecipe("rec-2", "Test 2", "item-003", 1, []Ingredient{{"item-002", 2}}, 100)
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
