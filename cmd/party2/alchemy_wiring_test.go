package main

import (
	"context"
	"errors"
	"testing"

	"github.com/witchcraze/party2re/internal/alchemy"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/testutil"
)

var errAlchemyLegendInjected = errors.New("legend induction failed after legend write")

func TestAlchemyProductionConnections(t *testing.T) {
	t.Run("legend inductor", func(t *testing.T) {
		f := newHookWiringFixture(t, testutil.OpenValkey(t))
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
	f := newHookWiringFixture(t, testutil.OpenValkey(t))
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
