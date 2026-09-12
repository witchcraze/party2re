package database

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/alchemy"
)

func TestAlchemyRepositoryNilDB(t *testing.T) {
	if _, err := NewAlchemyRepository(nil); err == nil {
		t.Fatal("NewAlchemyRepository(nil) expected error, got nil")
	}
}

func TestAlchemyRepositoryLifecycle(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()
	alcRepo, err := NewAlchemyRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	char, err := CreateTestCharacter(ctx, db, "Alchemy Repo Test")
	if err != nil {
		t.Fatal(err)
	}

	// 1. Initially empty state
	initialState, err := alcRepo.GetSynthesisState(ctx, char.ID)
	if err != nil {
		t.Fatalf("GetSynthesisState failed: %v", err)
	}
	if initialState.State != alchemy.StateNone || initialState.RecipeID != "" {
		t.Errorf("expected StateNone, got %+v", initialState)
	}

	// 2. Save ongoing synthesis state
	now := time.Now().UTC().Truncate(time.Second)
	matures := now.Add(12 * time.Hour)
	savedState := alchemy.Synthesis{
		CharacterID: char.ID,
		RecipeID:    "recipe-001",
		State:       alchemy.StateOngoing,
		StartedAt:   &now,
		MaturesAt:   &matures,
		TotalCrafts: 0,
		CompAlc:     false,
	}
	if err := alcRepo.SaveSynthesisState(ctx, savedState); err != nil {
		t.Fatalf("SaveSynthesisState failed: %v", err)
	}

	fetchedState, err := alcRepo.GetSynthesisState(ctx, char.ID)
	if err != nil {
		t.Fatalf("GetSynthesisState failed: %v", err)
	}
	if fetchedState.State != alchemy.StateOngoing || fetchedState.RecipeID != "recipe-001" {
		t.Errorf("expected StateOngoing with recipe-001, got %+v", fetchedState)
	}

	// 3. Complete ongoing synthesis (e.g. home sleep hook)
	if err := alcRepo.CompleteOngoingSynthesis(ctx, char.ID); err != nil {
		t.Fatalf("CompleteOngoingSynthesis failed: %v", err)
	}
	completedState, err := alcRepo.GetSynthesisStateForUpdate(ctx, char.ID)
	if err != nil {
		t.Fatalf("GetSynthesisStateForUpdate failed: %v", err)
	}
	if completedState.State != alchemy.StateCompleted {
		t.Errorf("expected StateCompleted, got %v", completedState.State)
	}

	// 4. Recipes discovery and crafting
	recipes, err := alcRepo.GetDiscoveredRecipes(ctx, char.ID)
	if err != nil {
		t.Fatalf("GetDiscoveredRecipes failed: %v", err)
	}
	if len(recipes) != 0 {
		t.Errorf("expected 0 discovered recipes, got %d", len(recipes))
	}

	if err := alcRepo.SaveDiscoveredRecipe(ctx, char.ID, "recipe-001", false); err != nil {
		t.Fatalf("SaveDiscoveredRecipe failed: %v", err)
	}
	if err := alcRepo.SaveDiscoveredRecipe(ctx, char.ID, "recipe-002", false); err != nil {
		t.Fatalf("SaveDiscoveredRecipe failed: %v", err)
	}

	recipes, err = alcRepo.GetDiscoveredRecipes(ctx, char.ID)
	if err != nil {
		t.Fatalf("GetDiscoveredRecipes failed: %v", err)
	}
	if len(recipes) != 2 {
		t.Fatalf("expected 2 discovered recipes, got %d", len(recipes))
	}

	count, err := alcRepo.CountCraftedRecipes(ctx, char.ID)
	if err != nil {
		t.Fatalf("CountCraftedRecipes failed: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 crafted recipes, got %d", count)
	}

	if err := alcRepo.MarkRecipeCrafted(ctx, char.ID, "recipe-001"); err != nil {
		t.Fatalf("MarkRecipeCrafted failed: %v", err)
	}
	count, err = alcRepo.CountCraftedRecipes(ctx, char.ID)
	if err != nil {
		t.Fatalf("CountCraftedRecipes failed: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 crafted recipe, got %d", count)
	}

	// 5. Title award
	if err := alcRepo.SetCompAlcTitle(ctx, char.ID); err != nil {
		t.Fatalf("SetCompAlcTitle failed: %v", err)
	}
	finalState, err := alcRepo.GetSynthesisState(ctx, char.ID)
	if err != nil {
		t.Fatalf("GetSynthesisState failed: %v", err)
	}
	if !finalState.CompAlc {
		t.Errorf("expected CompAlc == true, got false")
	}
}
