package gemstore_test

import (
	"context"
	"testing"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/depot"
	"github.com/witchcraze/party2re/internal/gemstore"
)

// Recipes reference materials by display name, so a catalog rename must not leave a recipe unmatchable.
func TestRecipeReferencesResolveToOneDefinition(t *testing.T) {
	catalog, err := gemstore.DefaultCatalog()
	if err != nil {
		t.Fatal(err)
	}
	items, err := coreitem.DefaultCatalog()
	if err != nil {
		t.Fatal(err)
	}
	itemsByName := map[string]int{}
	for _, d := range items.Definitions() {
		itemsByName[d.Name]++
	}
	recipes := catalog.AllRecipes()
	if len(recipes) != 56 {
		t.Fatalf("recipes = %d, want 56", len(recipes))
	}
	for _, r := range recipes {
		if _, ok := catalog.FindGemByName(r.ResultName); !ok {
			t.Errorf("%s: result %q is not a gem", r.ID, r.ResultName)
		}
		for _, m := range []string{r.Material1, r.Material2} {
			_, isGem := catalog.FindGemByName(m)
			n := itemsByName[m]
			if isGem {
				n++
			}
			if n != 1 {
				t.Errorf("%s: material %q resolves to %d definitions, want 1", r.ID, m, n)
			}
		}
	}
}

func TestSynthesizeGem_ItemMaterials(t *testing.T) {
	catalog, err := gemstore.DefaultCatalog()
	if err != nil {
		t.Fatal(err)
	}
	items, err := coreitem.DefaultCatalog()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, recipe, gem, item, want string
	}{
		{"renamed item", "recipe_burst_sky", "gem_sky_atk_2", "item-059", "gem_burst_sky"},
		{"unchanged item", "recipe_dance_combo", "gem_combo_1", "item-008", "gem_dance_combo"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			char := corecharacter.Character{ID: "char_" + tc.recipe, PlayerID: "p1", Name: "Hero", JobLevel: 10}
			charRepo := newMockCharacterRepo()
			charRepo.characters[char.ID] = char
			invRepo, depotRepo, boxRepo := newMockInventoryRepo(), newMockDepotRepo(), newMockGemBoxRepo()

			gem, _ := coreitem.NewInstance(tc.gem, 1)
			if err := boxRepo.Save(ctx, gemstore.GemBox{CharacterID: char.ID, Capacity: 5, Items: []coreitem.Instance{gem}}); err != nil {
				t.Fatal(err)
			}
			mat, _ := coreitem.NewInstance(tc.item, 1)
			dep, _ := depot.NewDepot(char.ID)
			dep.Items = append(dep.Items, mat)
			if err := depotRepo.Save(ctx, dep); err != nil {
				t.Fatal(err)
			}

			svc, err := gemstore.NewService(catalog, charRepo, invRepo,
				gemstore.WithDepotRepository(depotRepo),
				gemstore.WithGemBoxRepository(boxRepo),
				gemstore.WithItemDefinitionProvider(items),
			)
			if err != nil {
				t.Fatal(err)
			}
			res, err := svc.SynthesizeGem(ctx, char.ID, tc.recipe)
			if err != nil {
				t.Fatalf("SynthesizeGem: %v", err)
			}
			box, _ := boxRepo.FindByCharacterID(ctx, char.ID)
			inv, _ := invRepo.FindByCharacterID(ctx, char.ID)
			dep, _ = depotRepo.FindByCharacterID(ctx, char.ID)
			if res.CreatedGem.ID != tc.want || len(box.Items) != 1 || box.Items[0].DefinitionID != tc.want ||
				len(inv.Items) != 0 || len(dep.Items) != 0 {
				t.Fatalf("want only %s left, got box=%+v inv=%+v depot=%+v", tc.want, box.Items, inv.Items, dep.Items)
			}
		})
	}
}
