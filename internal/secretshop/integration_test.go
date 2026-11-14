package secretshop_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/secretshop"
	"github.com/witchcraze/party2re/internal/testutil"
)

func TestPurchaseItem_HelperExclusionAndConcurrentRouting_Integration(t *testing.T) {
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
	chars, err := database.NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	players, err := database.NewPlayerRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	invs, err := database.NewInventoryRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	depots, err := database.NewDepotRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	char, err := database.CreateTestCharacterWithFunds(ctx, db, "", 50000)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := chars.Delete(ctx, char.ID); err != nil {
			t.Error(err)
			return
		}
		if err := players.Delete(ctx, char.PlayerID); err != nil {
			t.Error(err)
		}
	})
	char.JobLevel = 7
	if err := chars.Update(ctx, char); err != nil {
		t.Fatal(err)
	}
	filter := &mockHelperFilter{activeHelperItemIDs: []string{"item-010"}}
	catalog, err := secretshop.LoadDefaultCatalog()
	if err != nil {
		t.Fatal(err)
	}
	svc, err := secretshop.NewService(chars, invs, catalog,
		secretshop.WithHelperFilter(filter), secretshop.WithDepotRepository(depots),
		secretshop.WithTransactionProvider(database.NewTransactionProvider(db)))
	if err != nil {
		t.Fatal(err)
	}
	assertAssets := func(gold, inventory, depot int) {
		t.Helper()
		got, err := chars.FindByID(ctx, char.ID)
		if err != nil {
			t.Fatal(err)
		}
		inv, err := invs.FindByCharacterID(ctx, char.ID)
		if err != nil {
			t.Fatal(err)
		}
		dep, err := depots.FindByCharacterID(ctx, char.ID)
		if err != nil {
			t.Fatal(err)
		}
		quantity := 0
		for _, it := range inv.Items {
			if it.DefinitionID == "item-010" {
				quantity += it.Quantity
			}
		}
		if got.Money != gold || quantity != inventory || dep.Quantity("item-010") != depot {
			t.Fatalf("assets = %d/%d/%d, want %d/%d/%d", got.Money, quantity, dep.Quantity("item-010"), gold, inventory, depot)
		}
	}
	result := testutil.RunConcurrentStressTest(t, testutil.GetStressConfig(), func(_, _ int) error {
		_, err := svc.PurchaseItem(ctx, char.ID, "secret_item_herbal_root", 1)
		if !errors.Is(err, secretshop.ErrItemUnavailableInHelperQuest) {
			return fmt.Errorf("excluded purchase: %v", err)
		}
		return nil
	})
	if result.Failures != 0 {
		t.Fatalf("excluded purchases: %+v", result)
	}
	assertAssets(50000, 0, 0)
	filter.activeHelperItemIDs = nil
	for _, err := range testutil.RunRace(
		func() error { _, err := svc.PurchaseItem(ctx, char.ID, "secret_item_herbal_root", 1); return err },
		func() error { _, err := svc.PurchaseItem(ctx, char.ID, "secret_item_herbal_root", 1); return err },
	) {
		if err != nil {
			t.Fatal(err)
		}
	}
	assertAssets(50000-2*750, 1, 1)
}
