package alchemy

import (
	"context"
	"errors"
	"fmt"
	"testing"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
)

func TestLearnRecipe_BaseIngredientOnly(t *testing.T) {
	ctx := context.Background()
	charRepo := newMemoryCharRepo()
	depotRepo := newMemoryDepotRepo()
	alchemyRepo := newMemoryAlchemyRepo()

	char, _ := corecharacter.New("Alchem")
	charRepo.characters[char.ID] = char

	itemA, _ := coreitem.NewDefinition("item-A", "BaseHerb", 10)
	itemB, _ := coreitem.NewDefinition("item-B", "BombStone", 20)
	itemC, _ := coreitem.NewDefinition("item-C", "ResultOne", 50)
	itemD, _ := coreitem.NewDefinition("item-D", "ResultTwo", 60)
	items, _ := coreitem.NewCatalog([]coreitem.Definition{itemA, itemB, itemC, itemD})

	// Recipe 1: Base is item-A, Secondary is item-B
	r1, _ := NewRecipe("rec-1", "Herb + Stone", "item-C", 1, []Ingredient{{"item-A", 1}, {"item-B", 1}})
	// Recipe 2: Base is item-B, Secondary is item-A
	r2, _ := NewRecipe("rec-2", "Stone + Herb", "item-D", 1, []Ingredient{{"item-B", 1}, {"item-A", 1}})
	recipes, _ := NewRecipeCatalog([]Recipe{r1, r2})

	svc, err := NewService(charRepo, depotRepo, alchemyRepo, recipes, items)
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}

	// Learning with allowedBases = ["BaseHerb"] MUST only match rec-1
	learned, err := svc.LearnRecipe(ctx, char.ID, []string{"BaseHerb"})
	if err != nil {
		t.Fatalf("LearnRecipe failed: %v", err)
	}
	if learned.ID != "rec-1" {
		t.Fatalf("expected rec-1, got %s", learned.ID)
	}

	// Second attempt with "BaseHerb" must return ErrNoRecipesToLearn because rec-2 has item-B as base
	_, err = svc.LearnRecipe(ctx, char.ID, []string{"BaseHerb"})
	if !errors.Is(err, ErrNoRecipesToLearn) {
		t.Fatalf("expected ErrNoRecipesToLearn, got %v", err)
	}
}

func TestLearnRecipe_GodRecipeLeakagePrevented(t *testing.T) {
	ctx := context.Background()
	charRepo := newMemoryCharRepo()
	depotRepo := newMemoryDepotRepo()
	alchemyRepo := newMemoryAlchemyRepo()

	char, _ := corecharacter.New("Alchem")
	charRepo.characters[char.ID] = char

	recipes, err := InitialRecipeCatalog()
	if err != nil {
		t.Fatalf("InitialRecipeCatalog failed: %v", err)
	}
	items, err := coreitem.InitialCatalog()
	if err != nil {
		t.Fatalf("coreitem.InitialCatalog failed: %v", err)
	}

	svc, err := NewService(charRepo, depotRepo, alchemyRepo, recipes, items)
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}

	// Advanced scroll pool includes "幸せの種" (item-022), but NOT "幸せの帽子" (item-111) or "幸せのくつ" (item-105).
	// recipe-009 (幸せのくつの合成): base is item-111 (幸せの帽子), secondary is item-022 (幸せの種).
	// recipe-011 (幸せの帽子の合成): base is item-105 (幸せのくつ), secondary is item-022 (幸せの種).
	// Both are God-only recipes marked with '#' in legacy CGI.
	// With the base-only restriction, neither recipe must ever be learned via advancedRecipePool.
	advancedRecipePool := []string{
		"item-006", "item-011", "item-022", "item-021", "item-073", "item-078", "item-081", "item-082",
		"item-106", "item-026", "item-119", "item-115", "item-114", "weapon-15", "weapon-46", "weapon-19",
		"weapon-21", "weapon-13", "item-090", "weapon-34", "weapon-57", "weapon-65", "weapon-20", "weapon-48",
		"armor-19", "armor-14", "armor-43", "armor-21", "armor-08", "armor-38",
	}

	learnedMap := make(map[string]bool)
	learnedCount := 0
	for {
		r, err := svc.LearnRecipe(ctx, char.ID, advancedRecipePool)
		if errors.Is(err, ErrNoRecipesToLearn) {
			break
		}
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if r.ID == "recipe-009" || r.ID == "recipe-011" {
			t.Fatalf("God recipe leaked into advancedRecipePool: %s (%s)", r.ID, r.Name)
		}
		baseDefID := r.Ingredients[0].DefinitionID
		baseDef, _ := items.FindByID(baseDefID)
		match := false
		for _, b := range advancedRecipePool {
			if b == baseDefID || b == baseDef.Name {
				match = true
				break
			}
		}
		if !match {
			t.Fatalf("learned recipe %s (%s) whose base %s (%s) is not in pool", r.ID, r.Name, baseDefID, baseDef.Name)
		}
		learnedMap[r.ID] = true
		learnedCount++
	}

	if learnedCount != 33 {
		t.Fatalf("expected exactly 33 recipes learned from advancedRecipePool, got %d", learnedCount)
	}

	// Verify that the previously-missing clean-room renamed base recipes are all learned
	expectedAdv := []string{"recipe-006", "recipe-046", "recipe-051", "recipe-055", "recipe-097", "recipe-098"}
	for _, id := range expectedAdv {
		if !learnedMap[id] {
			t.Errorf("expected previously missing recipe %s to be learned from advanced pool", id)
		}
	}

	// Now use God recipe scroll (empty pool) - remaining recipes (114 - 33 = 81) should be learnable!
	godLearnedCount := 0
	for {
		r, err := svc.LearnRecipe(ctx, char.ID, nil)
		if errors.Is(err, ErrNoRecipesToLearn) {
			break
		}
		if err != nil {
			t.Fatalf("God scroll failed: %v", err)
		}
		learnedMap[r.ID] = true
		godLearnedCount++
	}
	if godLearnedCount != 81 {
		t.Fatalf("expected 81 remaining recipes learned from God scroll, got %d", godLearnedCount)
	}
	if len(learnedMap) != 114 {
		t.Fatalf("expected all 114 recipes discovered, got %d", len(learnedMap))
	}
}

func TestLearnRecipe_BasicRecipePool(t *testing.T) {
	ctx := context.Background()
	charRepo := newMemoryCharRepo()
	depotRepo := newMemoryDepotRepo()
	alchemyRepo := newMemoryAlchemyRepo()

	char, _ := corecharacter.New("Alchem")
	charRepo.characters[char.ID] = char

	recipes, err := InitialRecipeCatalog()
	if err != nil {
		t.Fatalf("InitialRecipeCatalog failed: %v", err)
	}
	items, err := coreitem.InitialCatalog()
	if err != nil {
		t.Fatalf("coreitem.InitialCatalog failed: %v", err)
	}

	svc, err := NewService(charRepo, depotRepo, alchemyRepo, recipes, items)
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}

	basicRecipePool := []string{
		"item-001", "item-002", "item-003", "item-007", "item-072", "item-079", "item-024", "item-025",
		"item-036", "item-131", "item-012", "item-118", "weapon-01", "weapon-03", "weapon-06", "weapon-43",
		"weapon-14", "weapon-45", "weapon-18", "weapon-08", "weapon-64", "weapon-09", "weapon-11", "weapon-47",
		"weapon-07", "armor-01", "armor-02", "armor-04", "armor-07", "armor-11", "armor-16", "armor-13",
		"armor-17", "armor-18",
	}

	learnedMap := make(map[string]bool)
	learnedCount := 0
	for {
		r, err := svc.LearnRecipe(ctx, char.ID, basicRecipePool)
		if errors.Is(err, ErrNoRecipesToLearn) {
			break
		}
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		baseDefID := r.Ingredients[0].DefinitionID
		baseDef, _ := items.FindByID(baseDefID)
		match := false
		for _, b := range basicRecipePool {
			if b == baseDefID || b == baseDef.Name {
				match = true
				break
			}
		}
		if !match {
			t.Fatalf("learned recipe %s (%s) whose base %s (%s) is not in basicRecipePool", r.ID, r.Name, baseDefID, baseDef.Name)
		}
		learnedMap[r.ID] = true
		learnedCount++
	}

	if learnedCount != 52 {
		t.Fatalf("expected exactly 52 recipes learned from basicRecipePool, got %d", learnedCount)
	}

	// Verify that the previously-missing clean-room renamed base recipes are all learned
	expectedBasic := []string{"recipe-040", "recipe-041", "recipe-057", "recipe-096", "recipe-101"}
	for _, id := range expectedBasic {
		if !learnedMap[id] {
			t.Errorf("expected previously missing recipe %s to be learned from basic pool", id)
		}
	}
}

type scriptedRNG struct {
	intnCalls []int
	intnRets  []int
	idx       int
}

func (s *scriptedRNG) Intn(n int) int {
	s.intnCalls = append(s.intnCalls, n)
	if s.idx < len(s.intnRets) {
		ret := s.intnRets[s.idx]
		s.idx++
		return ret
	}
	return 0
}

func (s *scriptedRNG) IntN(n int) int {
	return s.Intn(n)
}

func (s *scriptedRNG) Float64() float64 {
	return 0.0
}

func TestLearnRecipe_TwoStageRandomSelection(t *testing.T) {
	ctx := context.Background()
	charRepo := newMemoryCharRepo()
	depotRepo := newMemoryDepotRepo()
	alchemyRepo := newMemoryAlchemyRepo()

	char, _ := corecharacter.New("Alchem")
	charRepo.characters[char.ID] = char

	recipes, err := InitialRecipeCatalog()
	if err != nil {
		t.Fatalf("InitialRecipeCatalog failed: %v", err)
	}
	items, err := coreitem.InitialCatalog()
	if err != nil {
		t.Fatalf("coreitem.InitialCatalog failed: %v", err)
	}

	// Pool with two bases: item-118 (金の指輪: 7 recipes) and weapon-07 (おおきづち: 1 recipe).
	// Sorted base keys: ["item-118", "weapon-07"]
	// Index 0: item-118
	// Index 1: weapon-07
	pool := []string{"item-118", "weapon-07"}

	// Scripted sequence:
	// Call 1: Stage 1 base selection: Intn(2) -> return 0 (item-118)
	// Call 2: Stage 2 recipe selection: Intn(7) -> return 3 (recipe-027)
	// Call 3: Stage 1 base selection: Intn(2) -> return 1 (weapon-07)
	// Call 4: Stage 2 recipe selection: Intn(1) -> return 0 (recipe-071)
	// Now weapon-07 is completely discovered (0 left), so baseKeys becomes len 1: ["item-118"].
	// Call 5: Stage 1 base selection: Intn(1) -> return 0 (item-118)
	// Call 6: Stage 2 recipe selection: Intn(6) -> return 2 (3rd remaining recipe of item-118)
	rng := &scriptedRNG{
		intnRets: []int{0, 3, 1, 0, 0, 2},
	}

	svc, err := NewService(charRepo, depotRepo, alchemyRepo, recipes, items, WithRNG(rng))
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}

	r1, err := svc.LearnRecipe(ctx, char.ID, pool)
	if err != nil {
		t.Fatalf("LearnRecipe 1 failed: %v", err)
	}
	if r1.ID != "recipe-027" {
		t.Errorf("expected recipe-027 on call 1, got %s", r1.ID)
	}

	r2, err := svc.LearnRecipe(ctx, char.ID, pool)
	if err != nil {
		t.Fatalf("LearnRecipe 2 failed: %v", err)
	}
	if r2.ID != "recipe-071" {
		t.Errorf("expected recipe-071 on call 2, got %s", r2.ID)
	}

	r3, err := svc.LearnRecipe(ctx, char.ID, pool)
	if err != nil {
		t.Fatalf("LearnRecipe 3 failed: %v", err)
	}
	if r3.Ingredients[0].DefinitionID != "item-118" {
		t.Errorf("expected item-118 base on call 3, got %s", r3.Ingredients[0].DefinitionID)
	}

	expectedCalls := []int{2, 7, 2, 1, 1, 6}
	if len(rng.intnCalls) != len(expectedCalls) {
		t.Fatalf("expected %d Intn calls, got %d (%v)", len(expectedCalls), len(rng.intnCalls), rng.intnCalls)
	}
	for i, c := range expectedCalls {
		if rng.intnCalls[i] != c {
			t.Errorf("call %d expected argument Intn(%d), got Intn(%d)", i, c, rng.intnCalls[i])
		}
	}
}

func TestLearnRecipe_TwoStageRandomDistribution(t *testing.T) {
	ctx := context.Background()
	recipes, err := InitialRecipeCatalog()
	if err != nil {
		t.Fatalf("InitialRecipeCatalog failed: %v", err)
	}
	items, err := coreitem.InitialCatalog()
	if err != nil {
		t.Fatalf("coreitem.InitialCatalog failed: %v", err)
	}

	// Test distribution with 2 bases:
	// item-118 (金の指輪: 7 recipes) vs weapon-07 (おおきづち: 1 recipe)
	// Under 1-stage uniform recipe selection, weapon-07 is picked with probability 1/8 (12.5%).
	// Under 2-stage base-first selection, weapon-07 is picked with probability 1/2 (50%).
	pool := []string{"item-118", "weapon-07"}

	charRepo := newMemoryCharRepo()
	depotRepo := newMemoryDepotRepo()
	alchemyRepo := newMemoryAlchemyRepo()
	svc, err := NewService(charRepo, depotRepo, alchemyRepo, recipes, items)
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}

	weapon07Count := 0
	trials := 500
	for i := 0; i < trials; i++ {
		charID := fmt.Sprintf("char-%d", i)
		r, err := svc.LearnRecipe(ctx, charID, pool)
		if err != nil {
			t.Fatalf("trial %d failed: %v", i, err)
		}
		if r.Ingredients[0].DefinitionID == "weapon-07" {
			weapon07Count++
		}
	}

	// In 500 trials with p = 0.5, expected mean is 250, stddev ~11.2.
	// 40% to 60% (200 to 300) is well within normal bounds.
	// A 1-stage distribution would produce ~62 (12.5%), which would fail this check.
	ratio := float64(weapon07Count) / float64(trials)
	if ratio < 0.35 || ratio > 0.65 {
		t.Errorf("expected two-stage selection base frequency ~0.5, got %d/%d (%.2f)", weapon07Count, trials, ratio)
	}
}
