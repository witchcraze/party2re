package gemstore

import (
	"strings"
	"testing"

	coreitem "github.com/witchcraze/party2re/internal/core/item"
)

func TestLoadRecipesRejectsUnresolvedIDs(t *testing.T) {
	for _, field := range []string{"result", "material 1", "material 2"} {
		t.Run(field, func(t *testing.T) {
			c, err := DefaultCatalog()
			if err != nil {
				t.Fatal(err)
			}
			r := c.AllRecipes()[0]
			switch field {
			case "result":
				r.ResultID = "item-008" // A valid item is not a valid result gem.
			case "material 1":
				r.Material1 = "missing-gem"
			case "material 2":
				r.Material2 = "item-missing"
			}
			items, err := coreitem.DefaultCatalog()
			if err != nil {
				t.Fatal(err)
			}
			if err := c.loadRecipes([]Recipe{r}, items); err == nil || !strings.Contains(err.Error(), r.ID) {
				t.Fatalf("invalid %s accepted or lacks recipe context: %v", field, err)
			}
		})
	}
}

func TestLoadRecipesUsesCurrentNames(t *testing.T) {
	c, err := DefaultCatalog()
	if err != nil {
		t.Fatal(err)
	}
	r, _ := c.FindRecipeByID("recipe_dance_combo")
	gem := c.gemsByID[r.Material1]
	gem.Name = "renamed gem"
	c.gemsByID[gem.ID] = gem
	items, err := coreitem.NewCatalog([]coreitem.Definition{{ID: "item-008", Name: "renamed item", Slot: coreitem.SlotNone}})
	if err != nil {
		t.Fatal(err)
	}
	if err := c.loadRecipes([]Recipe{r}, items); err != nil {
		t.Fatal(err)
	}
	got := c.AllRecipes()[0]
	if got.Material1Name != "renamed gem" || got.Material2Name != "renamed item" || got.ResultName != "連舞の宝珠" {
		t.Fatalf("names did not follow definitions: %+v", got)
	}
}
