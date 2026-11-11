package gemstore_test

import (
	"context"
	"errors"
	"testing"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/depot"
	"github.com/witchcraze/party2re/internal/gemstore"
)

type errorInjectingDepotRepo struct {
	*mockDepotRepo
	findErr error
}

func (r *errorInjectingDepotRepo) FindByCharacterID(ctx context.Context, characterID string) (depot.Depot, error) {
	if r.findErr != nil {
		return depot.Depot{}, r.findErr
	}
	return r.mockDepotRepo.FindByCharacterID(ctx, characterID)
}

func (r *errorInjectingDepotRepo) FindByCharacterIDForUpdate(ctx context.Context, characterID string) (depot.Depot, error) {
	if r.findErr != nil {
		return depot.Depot{}, r.findErr
	}
	if err := ctx.Err(); err != nil {
		return depot.Depot{}, err
	}
	return r.mockDepotRepo.FindByCharacterIDForUpdate(ctx, characterID)
}

func setupTestCatalogAndProviders(t *testing.T) (*gemstore.Catalog, *mockItemProvider) {
	t.Helper()
	catalog, err := gemstore.DefaultCatalog()
	if err != nil {
		t.Fatalf("failed to load default catalog: %v", err)
	}
	itemProvider := &mockItemProvider{
		items: map[string]coreitem.Definition{
			"unidentified_glowing_orb": {ID: "unidentified_glowing_orb", Name: "光る宝珠"},
			"iron_sword":               {ID: "iron_sword", Name: "鉄の剣"},
			"gem_atk_1":                {ID: "gem_atk_1", Name: "攻撃の宝珠Ⅰ"},
			"gem_atk_2":                {ID: "gem_atk_2", Name: "攻撃の宝珠Ⅱ"},
		},
	}
	return catalog, itemProvider
}

func TestSynthesizeGem_DepotReadFailure_PropagationAndNoMutation(t *testing.T) {
	catalog, itemProvider := setupTestCatalogAndProviders(t)

	char := corecharacter.Character{ID: "char_synth_err", PlayerID: "p1", Name: "Hero", JobLevel: 10, Money: 10000}

	// 1. Temporary SQL Error: must propagate and not mutate state even if materials exist in inventory
	t.Run("transient SQL error aborts and prevents mutation", func(t *testing.T) {
		charRepo := newMockCharacterRepo()
		charRepo.characters[char.ID] = char

		invRepo := newMockInventoryRepo()
		inv, _ := coreinventory.New(char.ID)
		_ = inv.Add(coreitem.Instance{ID: "inv-mat-1", DefinitionID: "gem_atk_1", Quantity: 1})
		_ = inv.Add(coreitem.Instance{ID: "inv-mat-2", DefinitionID: "gem_atk_1", Quantity: 1})
		invRepo.inventories[char.ID] = inv

		baseDepotRepo := newMockDepotRepo()
		depotErr := errors.New("sql: connection refused")
		depotRepo := &errorInjectingDepotRepo{mockDepotRepo: baseDepotRepo, findErr: depotErr}

		boxRepo := newMockGemBoxRepo()

		svc, err := gemstore.NewService(
			catalog,
			charRepo,
			invRepo,
			gemstore.WithDepotRepository(depotRepo),
			gemstore.WithGemBoxRepository(boxRepo),
			gemstore.WithItemDefinitionProvider(itemProvider),
		)
		if err != nil {
			t.Fatalf("failed to create service: %v", err)
		}

		ctx := context.Background()
		_, err = svc.SynthesizeGem(ctx, char.ID, "recipe_atk_2")
		if err == nil {
			t.Fatalf("expected error from SynthesizeGem, got nil")
		}
		if !errors.Is(err, depotErr) {
			t.Fatalf("expected error %v, got %v", depotErr, err)
		}

		// Verify Inventory is unchanged
		savedInv, _ := invRepo.FindByCharacterID(ctx, char.ID)
		if len(savedInv.Items) != 2 {
			t.Fatalf("expected 2 items in inventory, got %d", len(savedInv.Items))
		}

		// Verify GemBox is unchanged
		box, _ := boxRepo.FindByCharacterID(ctx, char.ID)
		if len(box.Items) != 0 {
			t.Fatalf("expected 0 items in gem box, got %d", len(box.Items))
		}

		// Verify Depot was never saved
		savedDepot, _ := baseDepotRepo.FindByCharacterID(ctx, char.ID)
		if len(savedDepot.Items) != 0 {
			t.Fatalf("expected depot to remain empty, got %+v", savedDepot)
		}
	})

	// 2. Cancellation: must propagate and not mutate state
	t.Run("context cancellation aborts and prevents mutation", func(t *testing.T) {
		charRepo := newMockCharacterRepo()
		charRepo.characters[char.ID] = char

		invRepo := newMockInventoryRepo()
		inv, _ := coreinventory.New(char.ID)
		_ = inv.Add(coreitem.Instance{ID: "inv-mat-1", DefinitionID: "gem_atk_1", Quantity: 2})
		invRepo.inventories[char.ID] = inv

		baseDepotRepo := newMockDepotRepo()
		depotRepo := &errorInjectingDepotRepo{mockDepotRepo: baseDepotRepo}
		boxRepo := newMockGemBoxRepo()

		svc, err := gemstore.NewService(
			catalog,
			charRepo,
			invRepo,
			gemstore.WithDepotRepository(depotRepo),
			gemstore.WithGemBoxRepository(boxRepo),
			gemstore.WithItemDefinitionProvider(itemProvider),
		)
		if err != nil {
			t.Fatalf("failed to create service: %v", err)
		}

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // pre-cancel context

		_, err = svc.SynthesizeGem(ctx, char.ID, "recipe_atk_2")
		if err == nil {
			t.Fatalf("expected cancellation error from SynthesizeGem, got nil")
		}
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context.Canceled, got %v", err)
		}

		// Verify Inventory is unchanged
		savedInv, _ := invRepo.FindByCharacterID(context.Background(), char.ID)
		if len(savedInv.Items) != 1 || savedInv.Items[0].Quantity != 2 {
			t.Fatalf("expected 1 slot with quantity 2 in inventory, got %+v", savedInv.Items)
		}

		// Verify GemBox is unchanged
		box, _ := boxRepo.FindByCharacterID(context.Background(), char.ID)
		if len(box.Items) != 0 {
			t.Fatalf("expected 0 items in gem box, got %d", len(box.Items))
		}
	})

	// 4. Depot Not Found when item missing -> returns ErrItemNotOwned
	t.Run("uncreated depot ErrNotFound reports ErrItemNotOwned when item missing", func(t *testing.T) {
		charRepo := newMockCharacterRepo()
		charRepo.characters[char.ID] = char

		invRepo := newMockInventoryRepo()
		inv, _ := coreinventory.New(char.ID)
		invRepo.inventories[char.ID] = inv

		baseDepotRepo := newMockDepotRepo()
		depotRepo := &errorInjectingDepotRepo{mockDepotRepo: baseDepotRepo, findErr: depot.ErrNotFound}
		boxRepo := newMockGemBoxRepo()

		svc, err := gemstore.NewService(
			catalog,
			charRepo,
			invRepo,
			gemstore.WithDepotRepository(depotRepo),
			gemstore.WithGemBoxRepository(boxRepo),
			gemstore.WithItemDefinitionProvider(itemProvider),
		)
		if err != nil {
			t.Fatalf("failed to create service: %v", err)
		}

		ctx := context.Background()
		_, err = svc.AppraiseItem(ctx, char.ID, "nonexistent-item")
		if !errors.Is(err, gemstore.ErrItemNotOwned) {
			t.Fatalf("expected ErrItemNotOwned, got %v", err)
		}
	})

	// 5. Regular equipment appraisal aborts on SQL error
	t.Run("regular equipment appraisal aborts on transient SQL error", func(t *testing.T) {
		charRepo := newMockCharacterRepo()
		charRepo.characters[char.ID] = char

		invRepo := newMockInventoryRepo()
		inv, _ := coreinventory.New(char.ID)
		_ = inv.Add(coreitem.Instance{ID: "inv-sword-1", DefinitionID: "iron_sword", Quantity: 1})
		invRepo.inventories[char.ID] = inv

		baseDepotRepo := newMockDepotRepo()
		depotErr := errors.New("sql: statement timeout")
		depotRepo := &errorInjectingDepotRepo{mockDepotRepo: baseDepotRepo, findErr: depotErr}

		boxRepo := newMockGemBoxRepo()

		svc, err := gemstore.NewService(
			catalog,
			charRepo,
			invRepo,
			gemstore.WithDepotRepository(depotRepo),
			gemstore.WithGemBoxRepository(boxRepo),
			gemstore.WithItemDefinitionProvider(itemProvider),
		)
		if err != nil {
			t.Fatalf("failed to create service: %v", err)
		}

		ctx := context.Background()
		_, err = svc.AppraiseItem(ctx, char.ID, "inv-sword-1")
		if err == nil {
			t.Fatalf("expected error from AppraiseItem on regular sword with depot failure, got nil")
		}
		if !errors.Is(err, depotErr) {
			t.Fatalf("expected error %v, got %v", depotErr, err)
		}
	})

	// 6. Depot SQL error when item is in depot -> returns SQL error, not ErrItemNotOwned
	t.Run("transient SQL error when item only in depot returns SQL error not ErrItemNotOwned", func(t *testing.T) {
		charRepo := newMockCharacterRepo()
		charRepo.characters[char.ID] = char

		invRepo := newMockInventoryRepo()
		inv, _ := coreinventory.New(char.ID)
		invRepo.inventories[char.ID] = inv

		baseDepotRepo := newMockDepotRepo()
		dep, _ := depot.NewDepot(char.ID)
		_ = dep.AddItem(coreitem.Instance{ID: "depot-orb-1", DefinitionID: "unidentified_glowing_orb", Quantity: 1})
		baseDepotRepo.depots[char.ID] = dep

		depotErr := errors.New("sql: network packet dropped")
		depotRepo := &errorInjectingDepotRepo{mockDepotRepo: baseDepotRepo, findErr: depotErr}
		boxRepo := newMockGemBoxRepo()

		svc, err := gemstore.NewService(
			catalog,
			charRepo,
			invRepo,
			gemstore.WithDepotRepository(depotRepo),
			gemstore.WithGemBoxRepository(boxRepo),
			gemstore.WithItemDefinitionProvider(itemProvider),
			gemstore.WithRandomSource(fixedRandomSource{value: 0}),
		)
		if err != nil {
			t.Fatalf("failed to create service: %v", err)
		}

		ctx := context.Background()
		_, err = svc.AppraiseItem(ctx, char.ID, "depot-orb-1")
		if err == nil {
			t.Fatalf("expected error from AppraiseItem, got nil")
		}
		if !errors.Is(err, depotErr) {
			t.Fatalf("expected error %v, got %v", depotErr, err)
		}
		if errors.Is(err, gemstore.ErrItemNotOwned) {
			t.Fatalf("did not expect ErrItemNotOwned to mask depot error")
		}
	})
}
