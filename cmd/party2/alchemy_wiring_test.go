package main

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/witchcraze/party2re/internal/alchemy"
	"github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/depot"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/logging"
	"github.com/witchcraze/party2re/internal/medal"
)

var (
	errAlchemySynthesisHookInjected = errors.New("synthesis hook failed after medal write")
	errAlchemyLegendInjected        = errors.New("legend induction failed after legend write")
)

type writeThenFailSynthesisHook struct {
	real alchemy.SynthesisHook
}

func (h writeThenFailSynthesisHook) Hook(ctx context.Context, characterID, recipeID string) error {
	if err := h.real(ctx, characterID, recipeID); err != nil {
		return err
	}
	return errAlchemySynthesisHookInjected
}

func TestAlchemyProductionWiring_SynthesisHookMedalRollbackAndRetry(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}
	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})

	ctx := context.Background()
	core, err := newCoreServices(db, nil)
	if err != nil {
		t.Fatal(err)
	}
	econ, err := newEconServices(db, core)
	if err != nil {
		t.Fatal(err)
	}
	soc, err := newSocServices(db, core, econ, nil, logging.Nop())
	if err != nil {
		t.Fatal(err)
	}
	misc, err := newMiscServices(db, core, soc, econ, nil)
	if err != nil {
		t.Fatal(err)
	}
	cmbt, err := newCmbtServices(db, core, soc, econ, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := wireHooks(core, cmbt, econ, misc, soc); err != nil {
		t.Fatal(err)
	}

	char, err := database.CreateTestCharacter(ctx, db, "Alch_"+id.New()[:6])
	if err != nil {
		t.Fatal(err)
	}

	// 1. Put ingredients for recipe-001 into depot
	dep, err := depot.NewDepot(char.ID)
	if err != nil {
		t.Fatal(err)
	}
	dep.Capacity = 10
	herbInst, err := item.NewInstance("item-001", 2)
	if err != nil {
		t.Fatal(err)
	}
	if err := dep.AddItem(herbInst); err != nil {
		t.Fatal(err)
	}
	if err := econ.depotRepo.Save(ctx, dep); err != nil {
		t.Fatal(err)
	}

	// 2. Unlock, synthesize and complete
	if err := econ.alchemy.UnlockRecipe(ctx, char.ID, "recipe-001"); err != nil {
		t.Fatalf("UnlockRecipe failed: %v", err)
	}
	if _, err := econ.alchemy.Synthesize(ctx, char.ID, "recipe-001"); err != nil {
		t.Fatalf("Synthesize failed: %v", err)
	}
	if err := econ.alchemy.CompleteOngoingSynthesis(ctx, char.ID); err != nil {
		t.Fatalf("CompleteOngoingSynthesis failed: %v", err)
	}

	// 3. Inject writeThenFailSynthesisHook wrapping real medal progress
	prodHook := func(ctx context.Context, characterID string, recipeID string) error {
		return misc.medal.RecordProgress(ctx, characterID, medal.MetricAlchemyCrafts, 1)
	}
	econ.alchemy.SetSynthesisHook(writeThenFailSynthesisHook{real: prodHook}.Hook)

	// 4. Claim must fail with injected error
	_, err = econ.alchemy.Claim(ctx, char.ID)
	if !errors.Is(err, errAlchemySynthesisHookInjected) {
		t.Fatalf("expected errAlchemySynthesisHookInjected, got: %v", err)
	}

	// 5. Verify rollback:
	// - character_achievements has 0 rows for character
	var achievementCount int
	err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM character_achievements WHERE character_id = ?", char.ID).Scan(&achievementCount)
	if err != nil {
		t.Fatal(err)
	}
	if achievementCount != 0 {
		t.Fatalf("character_achievements committed despite rollback: count = %d", achievementCount)
	}

	// - depot has 0 items
	depAfterRollback, err := econ.depotRepo.FindByCharacterID(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(depAfterRollback.Items) != 0 {
		t.Fatalf("depot item was committed despite rollback: %+v", depAfterRollback.Items)
	}

	// - synthesis state is still completed
	state, err := econ.alchemy.GetStatus(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}
	if state.State != alchemy.StateCompleted || state.RecipeID != "recipe-001" {
		t.Fatalf("synthesis state modified despite rollback: %+v", state)
	}

	// 6. Restore production hook and retry Claim
	econ.alchemy.SetSynthesisHook(prodHook)
	claimRes, err := econ.alchemy.Claim(ctx, char.ID)
	if err != nil {
		t.Fatalf("retry Claim failed: %v", err)
	}
	if claimRes.CreatedItem.DefinitionID != "item-002" || claimRes.TotalCrafts != 1 {
		t.Fatalf("unexpected claim result: %+v", claimRes)
	}

	// 7. Verify committed state after retry:
	// - character_achievements has progress = 1 recorded for each alchemy achievement
	var progressNovice, progressMaster int
	err = db.QueryRowContext(ctx, "SELECT current_progress FROM character_achievements WHERE character_id = ? AND achievement_id = 'alchemy_novice'", char.ID).Scan(&progressNovice)
	if err != nil {
		t.Fatal(err)
	}
	if progressNovice != 1 {
		t.Fatalf("expected alchemy_novice progress 1, got %d", progressNovice)
	}
	err = db.QueryRowContext(ctx, "SELECT current_progress FROM character_achievements WHERE character_id = ? AND achievement_id = 'alchemy_master'", char.ID).Scan(&progressMaster)
	if err != nil {
		t.Fatal(err)
	}
	if progressMaster != 1 {
		t.Fatalf("expected alchemy_master progress 1, got %d", progressMaster)
	}

	// - depot has 1 item
	depFinal, err := econ.depotRepo.FindByCharacterID(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(depFinal.Items) != 1 || depFinal.Items[0].DefinitionID != "item-002" {
		t.Fatalf("expected 1 item-002 in depot, got %+v", depFinal.Items)
	}
}

type writeThenFailAlchemyLegend struct {
	real legendInductorAdapter
}

func (l writeThenFailAlchemyLegend) RecordLegend(ctx context.Context, category, characterID string) error {
	if err := l.real.RecordLegend(ctx, category, characterID); err != nil {
		return err
	}
	return errAlchemyLegendInjected
}

func TestAlchemyProductionWiring_LegendRollbackAndRetry(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}
	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})

	ctx := context.Background()
	core, err := newCoreServices(db, nil)
	if err != nil {
		t.Fatal(err)
	}
	econ, err := newEconServices(db, core)
	if err != nil {
		t.Fatal(err)
	}
	soc, err := newSocServices(db, core, econ, nil, logging.Nop())
	if err != nil {
		t.Fatal(err)
	}
	misc, err := newMiscServices(db, core, soc, econ, nil)
	if err != nil {
		t.Fatal(err)
	}
	cmbt, err := newCmbtServices(db, core, soc, econ, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := wireHooks(core, cmbt, econ, misc, soc); err != nil {
		t.Fatal(err)
	}

	char, err := database.CreateTestCharacter(ctx, db, "LegAlc_"+id.New()[:6])
	if err != nil {
		t.Fatal(err)
	}

	recipeCatalog, err := alchemy.InitialRecipeCatalog()
	if err != nil {
		t.Fatal(err)
	}
	allRecipes := recipeCatalog.All()
	totalRecipes := len(allRecipes)
	if totalRecipes != 114 {
		t.Fatalf("expected 114 recipes, got %d", totalRecipes)
	}

	alcRepo, err := database.NewAlchemyRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	// 1. Mark first 113 recipes as crafted in SQL
	for i := 0; i < totalRecipes-1; i++ {
		r := allRecipes[i]
		if err := alcRepo.SaveDiscoveredRecipe(ctx, char.ID, r.ID, true); err != nil {
			t.Fatal(err)
		}
	}

	// 2. Put ingredients for 114th recipe into depot
	finalRecipe := allRecipes[totalRecipes-1]
	dep, err := depot.NewDepot(char.ID)
	if err != nil {
		t.Fatal(err)
	}
	dep.Capacity = 20
	for _, ing := range finalRecipe.Ingredients {
		inst, err := item.NewInstance(ing.DefinitionID, ing.Quantity)
		if err != nil {
			t.Fatal(err)
		}
		if err := dep.AddItem(inst); err != nil {
			t.Fatal(err)
		}
	}
	if err := econ.depotRepo.Save(ctx, dep); err != nil {
		t.Fatal(err)
	}

	// 3. Unlock, synthesize and complete final recipe
	if err := econ.alchemy.UnlockRecipe(ctx, char.ID, finalRecipe.ID); err != nil {
		t.Fatalf("UnlockRecipe failed: %v", err)
	}
	if _, err := econ.alchemy.Synthesize(ctx, char.ID, finalRecipe.ID); err != nil {
		t.Fatalf("Synthesize failed: %v", err)
	}
	if err := econ.alchemy.CompleteOngoingSynthesis(ctx, char.ID); err != nil {
		t.Fatalf("CompleteOngoingSynthesis failed: %v", err)
	}

	// 4. Inject writeThenFailAlchemyLegend
	realLegendAdapter := legendInductorAdapter{ranking: soc.ranking}
	econ.alchemy.SetLegendInductor(writeThenFailAlchemyLegend{real: realLegendAdapter})

	// 5. Delivery never awards; the first complete recipe view fails with the injected error
	if _, err := econ.alchemy.Claim(ctx, char.ID); err != nil {
		t.Fatalf("Claim failed: %v", err)
	}
	_, err = econ.alchemy.ViewRecipes(ctx, char.ID)
	if !errors.Is(err, errAlchemyLegendInjected) {
		t.Fatalf("expected errAlchemyLegendInjected, got: %v", err)
	}

	// 6. Verify complete rollback:
	// - legend_records has 0 rows for character
	var legendCount int
	err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM legend_records WHERE character_id = ?", char.ID).Scan(&legendCount)
	if err != nil {
		t.Fatal(err)
	}
	if legendCount != 0 {
		t.Fatalf("legend_records committed despite rollback: count = %d", legendCount)
	}

	// - comp_alc flag is false in DB
	var compAlc bool
	err = db.QueryRowContext(ctx, "SELECT comp_alc FROM character_alchemy WHERE character_id = ?", char.ID).Scan(&compAlc)
	if err != nil {
		t.Fatal(err)
	}
	if compAlc {
		t.Fatal("character_alchemy.comp_alc committed as TRUE despite rollback")
	}

	// 7. Restore production legend inductor and retry the view
	econ.alchemy.SetLegendInductor(realLegendAdapter)
	viewRes, err := econ.alchemy.ViewRecipes(ctx, char.ID)
	if err != nil {
		t.Fatalf("retry ViewRecipes failed: %v", err)
	}
	if !viewRes.CompAlcAwarded {
		t.Fatal("expected CompAlcAwarded = true on the retried complete view")
	}

	// 8. Verify committed state after retry:
	// - legend_records has 1 row
	err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM legend_records WHERE character_id = ?", char.ID).Scan(&legendCount)
	if err != nil {
		t.Fatal(err)
	}
	if legendCount != 1 {
		t.Fatalf("expected 1 legend_records row, got %d", legendCount)
	}

	// - comp_alc is true
	err = db.QueryRowContext(ctx, "SELECT comp_alc FROM character_alchemy WHERE character_id = ?", char.ID).Scan(&compAlc)
	if err != nil {
		t.Fatal(err)
	}
	if !compAlc {
		t.Fatal("character_alchemy.comp_alc should be TRUE after retry")
	}
}
