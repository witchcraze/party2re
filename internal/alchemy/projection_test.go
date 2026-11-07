package alchemy

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/depot"
)

// dragonLeaks identifies the unlearned combination in setupTestService.
var dragonLeaks = []string{"rec-dragon-grass", "item-042", "Dragon Grass", "item-041"}

func assertNoneIn(t *testing.T, label string, v any, forbidden []string) {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal %s: %v", label, err)
	}
	for _, f := range forbidden {
		if strings.Contains(string(b), f) {
			t.Errorf("%s leaks %q: %s", label, f, b)
		}
	}
}

func TestGetCompendium_UnlearnedCombinationsAreNotObserved(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	svc, _, _, _, char := setupTestService(t, &now)

	comp, err := svc.GetCompendium(ctx, char.ID)
	if err != nil {
		t.Fatalf("GetCompendium: %v", err)
	}
	if comp.TotalRecipes != 2 || comp.LearnedCount != 0 || comp.CraftedCount != 0 || comp.CompletionPercentage != 0 {
		t.Errorf("counts changed: %+v", comp)
	}
	if comp.Entries == nil || len(comp.Entries) != 0 {
		t.Fatalf("entries = %#v, want empty non-nil", comp.Entries)
	}
	b, _ := json.Marshal(comp)
	if !strings.Contains(string(b), `"entries":[]`) {
		t.Errorf("entries must serialize as [], got %s", b)
	}
}

func TestGetCompendium_LearnedUncraftedHidesProduct(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	svc, _, _, _, char := setupTestService(t, &now)
	if err := svc.UnlockRecipe(ctx, char.ID, "rec-super-herb"); err != nil {
		t.Fatal(err)
	}

	comp, err := svc.GetCompendium(ctx, char.ID)
	if err != nil {
		t.Fatalf("GetCompendium: %v", err)
	}
	if comp.TotalRecipes != 2 || comp.LearnedCount != 1 || comp.CraftedCount != 0 {
		t.Errorf("counts changed: %+v", comp)
	}
	if len(comp.Entries) != 1 {
		t.Fatalf("entries = %d, want 1 (learned only)", len(comp.Entries))
	}
	e := comp.Entries[0]
	if e.RecipeID != "rec-super-herb" || !e.IsLearned || e.IsCrafted {
		t.Errorf("unexpected entry %+v", e)
	}
	if len(e.Ingredients) != 1 || e.Ingredients[0].DefinitionID != "item-001" || e.Ingredients[0].Quantity != 2 {
		t.Errorf("ingredients not published: %+v", e.Ingredients)
	}
	assertNoneIn(t, "compendium", comp, append([]string{"Synthesize Super Herb", "item-002", "Super Herb"}, dragonLeaks...))
}

func TestSynthesize_FirstCraftHidesProductUntilClaim(t *testing.T) {
	ctx := context.Background()
	simTime := time.Date(2026, 9, 12, 20, 0, 0, 0, timer.JST)
	svc, _, depotRepo, _, char := setupTestService(t, &simTime)
	_ = svc.UnlockRecipe(ctx, char.ID, "rec-super-herb")
	dep, _ := depot.NewDepot(char.ID)
	dep.Capacity = 10
	h, _ := coreitem.NewInstance("item-001", 4)
	_ = dep.AddItem(h)
	_ = depotRepo.Save(ctx, dep)

	res, err := svc.Synthesize(ctx, char.ID, "rec-super-herb")
	if err != nil {
		t.Fatalf("Synthesize: %v", err)
	}
	if res.Recipe.ID != "rec-super-herb" || len(res.Recipe.Ingredients) != 1 {
		t.Errorf("combination must stay visible: %+v", res.Recipe)
	}
	assertNoneIn(t, "first synthesis result", res, []string{"Synthesize Super Herb", "item-002", "Super Herb", "result_quantity"})

	// Status and compendium while the first synthesis is in progress.
	st, _ := svc.GetStatus(ctx, char.ID)
	assertNoneIn(t, "status", st, []string{"Synthesize Super Herb", "item-002", "Super Herb"})
	comp, _ := svc.GetCompendium(ctx, char.ID)
	assertNoneIn(t, "compendium while brewing", comp, []string{"Synthesize Super Herb", "item-002", "Super Herb"})

	// Claim publishes the product.
	if err := svc.CompleteOngoingSynthesis(ctx, char.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Claim(ctx, char.ID); err != nil {
		t.Fatalf("Claim: %v", err)
	}
	comp, _ = svc.GetCompendium(ctx, char.ID)
	if comp.CraftedCount != 1 || len(comp.Entries) != 1 {
		t.Fatalf("after claim: %+v", comp)
	}
	e := comp.Entries[0]
	if !e.IsCrafted || e.Name != "Synthesize Super Herb" || e.ResultItemDefinitionID != "item-002" || e.ResultItemName != "Super Herb" {
		t.Errorf("crafted entry must be fully public: %+v", e)
	}

	// Re-synthesizing a crafted recipe reveals the product.
	res, err = svc.Synthesize(ctx, char.ID, "rec-super-herb")
	if err != nil {
		t.Fatalf("second Synthesize: %v", err)
	}
	if res.Recipe.Name != "Synthesize Super Herb" || res.Recipe.ResultItemDefinitionID != "item-002" || res.Recipe.ResultQuantity != 1 {
		t.Errorf("crafted recipe must be public: %+v", res.Recipe)
	}
}
