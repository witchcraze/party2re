package main

import (
	"context"
	"errors"
	"testing"

	"github.com/witchcraze/party2re/internal/alchemy"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/medal"
)

var (
	errAlchemySynthesisHookInjected = errors.New("synthesis hook failed after medal write")
	errAlchemyLegendInjected        = errors.New("legend induction failed after legend write")
)

func TestAlchemyProductionConnections(t *testing.T) {
	t.Run("synthesis medal hook", func(t *testing.T) {
		f := newHookWiringFixture(t, nil)
		ctx := context.Background()
		repo, err := database.NewAlchemyRepository(f.db)
		if err != nil {
			t.Fatal(err)
		}
		if err := repo.SaveSynthesisState(ctx, alchemy.Synthesis{CharacterID: f.char.ID, RecipeID: "recipe-001", State: alchemy.StateCompleted}); err != nil {
			t.Fatal(err)
		}
		if _, err := f.econ.alchemy.Claim(ctx, f.char.ID); err != nil {
			t.Fatal(err)
		}
		var progress int
		if err := f.db.QueryRowContext(ctx, "SELECT current_progress FROM character_achievements WHERE character_id = ? AND achievement_id = 'alchemy_novice'", f.char.ID).Scan(&progress); err != nil {
			t.Fatal(err)
		}
		if progress != 1 {
			t.Fatalf("production medal connection progress = %d", progress)
		}
	})
	t.Run("legend inductor", func(t *testing.T) {
		f := newHookWiringFixture(t, nil)
		ctx := context.Background()
		repo, err := database.NewAlchemyRepository(f.db)
		if err != nil {
			t.Fatal(err)
		}
		recipes, err := alchemy.InitialRecipeCatalog()
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range recipes.All() {
			if err := repo.SaveDiscoveredRecipe(ctx, f.char.ID, r.ID, true); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := f.econ.alchemy.ViewRecipes(ctx, f.char.ID); err != nil {
			t.Fatal(err)
		}
		var count int
		if err := f.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM legend_records WHERE character_id = ? AND category = 'comp_alc'", f.char.ID).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("production legend connection count = %d", count)
		}
	})
}

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
	f := newHookWiringFixture(t, nil)
	ctx := context.Background()
	db, char, core, econ, soc, misc, cmbt := f.db, f.char, f.core, f.econ, f.soc, f.misc, f.cmbt

	repo, err := database.NewAlchemyRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.SaveSynthesisState(ctx, alchemy.Synthesis{CharacterID: char.ID, RecipeID: "recipe-001", State: alchemy.StateCompleted}); err != nil {
		t.Fatal(err)
	}

	// Inject writeThenFailSynthesisHook wrapping real medal progress
	prodHook := func(ctx context.Context, characterID string, recipeID string) error {
		return misc.medal.RecordProgress(ctx, characterID, medal.MetricAlchemyCrafts, 1)
	}
	econ.alchemy.SetSynthesisHook(writeThenFailSynthesisHook{real: prodHook}.Hook)

	// Claim must fail with injected error
	_, err = econ.alchemy.Claim(ctx, char.ID)
	if !errors.Is(err, errAlchemySynthesisHookInjected) {
		t.Fatalf("expected errAlchemySynthesisHookInjected, got: %v", err)
	}

	// Verify rollback:
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

	// Restore production hook and retry Claim
	if err := wireHooks(core, cmbt, econ, misc, soc); err != nil {
		t.Fatal(err)
	}
	_, err = econ.alchemy.Claim(ctx, char.ID)
	if err != nil {
		t.Fatalf("retry Claim failed: %v", err)
	}

	// Verify committed state after retry:
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
	f := newHookWiringFixture(t, nil)
	ctx := context.Background()
	db, char, core, econ, soc, misc, cmbt := f.db, f.char, f.core, f.econ, f.soc, f.misc, f.cmbt

	recipeCatalog, err := alchemy.InitialRecipeCatalog()
	if err != nil {
		t.Fatal(err)
	}
	allRecipes := recipeCatalog.All()

	alcRepo, err := database.NewAlchemyRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	if err := alcRepo.SaveSynthesisState(ctx, alchemy.Synthesis{CharacterID: char.ID, State: alchemy.StateNone}); err != nil {
		t.Fatal(err)
	}

	for _, r := range allRecipes {
		if err := alcRepo.SaveDiscoveredRecipe(ctx, char.ID, r.ID, true); err != nil {
			t.Fatal(err)
		}
	}

	// Inject writeThenFailAlchemyLegend
	realLegendAdapter := legendInductorAdapter{ranking: soc.ranking}
	econ.alchemy.SetLegendInductor(writeThenFailAlchemyLegend{real: realLegendAdapter})

	_, err = econ.alchemy.ViewRecipes(ctx, char.ID)
	if !errors.Is(err, errAlchemyLegendInjected) {
		t.Fatalf("expected errAlchemyLegendInjected, got: %v", err)
	}

	// Verify complete rollback:
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

	// Restore production legend inductor and retry the view
	if err := wireHooks(core, cmbt, econ, misc, soc); err != nil {
		t.Fatal(err)
	}
	viewRes, err := econ.alchemy.ViewRecipes(ctx, char.ID)
	if err != nil {
		t.Fatalf("retry ViewRecipes failed: %v", err)
	}
	if !viewRes.CompAlcAwarded {
		t.Fatal("expected CompAlcAwarded = true on the retried complete view")
	}

	// Verify committed state after retry:
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
	assertTrajectoryKind(t, db, char.ID, corecharacter.MemoryCompletion)
}
