package depot_test

import (
	"context"
	"errors"
	"os"
	"reflect"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/depot"
	"github.com/witchcraze/party2re/internal/testutil"
)

var errSortPostSave = errors.New("simulated post-save failure")

type sortFailingPostSave struct{ *database.DepotRepository }

func (r sortFailingPostSave) Save(ctx context.Context, value depot.Depot) error {
	if err := r.DepotRepository.Save(ctx, value); err != nil {
		return err
	}
	return errSortPostSave
}

func sortDBFixture(t *testing.T, failSave bool) (*depot.Service, corecharacter.Character, *database.CharacterRepository, *database.DepotRepository, []item.Instance) {
	t.Helper()
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
	inventories, err := database.NewInventoryRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	depots, err := database.NewDepotRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	actor, err := database.CreateTestCharacterWithFunds(ctx, db, "Depot Sort", 1000)
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := item.DefaultCatalog()
	if err != nil {
		t.Fatal(err)
	}
	items := []item.Instance{
		{ID: actor.ID[:30] + "ff", DefinitionID: "weapon-01", Quantity: 1, EnhancementLevel: 6},
		{ID: actor.ID[:30] + "ee", DefinitionID: "weapon-02", Quantity: 1},
		{ID: actor.ID[:30] + "dd", DefinitionID: "armor-01", Quantity: 1, EnhancementLevel: 2},
		{ID: actor.ID[:30] + "cc", DefinitionID: "armor-02", Quantity: 1},
		{ID: actor.ID[:30] + "bb", DefinitionID: "item-001", Quantity: 8},
		// Shield from the item catalog: legacy stores it as kind 3, so it sorts after item-001.
		{ID: actor.ID[:30] + "aa", DefinitionID: "item-071", Quantity: 1},
	}
	unsorted := slices.Clone(items)
	slices.Reverse(unsorted)
	if err := depots.Save(ctx, depot.Depot{CharacterID: actor.ID, Capacity: 5, Items: unsorted}); err != nil {
		t.Fatal(err)
	}
	var storage depot.Repository = depots
	if failSave {
		storage = sortFailingPostSave{depots}
	}
	svc, err := depot.NewServiceWithTransaction(storage, chars, inventories, depots, depot.WithItemDefinitionProvider(catalog))
	if err != nil {
		t.Fatal(err)
	}
	return svc, actor, chars, depots, items
}

func TestDepotSortPersistenceAndReload_Integration(t *testing.T) {
	svc, actor, chars, depots, items := sortDBFixture(t, false)
	ctx := context.Background()

	got, err := svc.SortItems(ctx, actor.ID)
	if err != nil {
		t.Fatalf("SortItems error = %v", err)
	}
	if !reflect.DeepEqual(got.Items, items) {
		t.Fatalf("sorted items mismatch: got=%+v want=%+v", got.Items, items)
	}
	reloaded, err := depots.FindByCharacterID(ctx, actor.ID)
	if err != nil {
		t.Fatal(err)
	}
	wallet, err := chars.FindByID(ctx, actor.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(reloaded.Items, items) || wallet.Money != actor.Money {
		t.Fatalf("reloaded depot/money mismatch: items=%+v money=%d", reloaded.Items, wallet.Money)
	}
}

func TestDepotSortRollbackAndEmptyStorage_Integration(t *testing.T) {
	for _, tc := range []struct {
		name     string
		failSave bool
	}{
		{"post-save rollback", true},
		{"empty storage", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, actor, chars, depots, _ := sortDBFixture(t, tc.failSave)
			ctx := context.Background()
			before, err := depots.FindByCharacterID(ctx, actor.ID)
			if err != nil {
				t.Fatal(err)
			}
			if !tc.failSave {
				before.Items = []item.Instance{}
				if err := depots.Save(ctx, before); err != nil {
					t.Fatal(err)
				}
			}
			got, err := svc.SortItems(ctx, actor.ID)
			if tc.failSave {
				if !errors.Is(err, errSortPostSave) {
					t.Fatalf("expected %v, got %v", errSortPostSave, err)
				}
			} else {
				if err != nil || got.Items == nil || len(got.Items) != 0 {
					t.Fatalf("empty SortItems = %+v, err=%v", got.Items, err)
				}
			}
			after, err := depots.FindByCharacterID(ctx, actor.ID)
			if err != nil {
				t.Fatal(err)
			}
			wallet, err := chars.FindByID(ctx, actor.ID)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(after, before) || wallet.Money != actor.Money {
				t.Fatalf("sort rollback/empty changed assets: before=%+v after=%+v money=%d", before, after, wallet.Money)
			}
		})
	}
}

func TestDepotSortAndSaleConcurrency_Integration(t *testing.T) {
	svc, actor, chars, depots, items := sortDBFixture(t, false)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var sales atomic.Int64
	res := testutil.RunConcurrentStressTest(t, testutil.GetStressConfig(), func(worker, op int) error {
		if (worker+op)%2 == 0 {
			_, err := svc.SortItems(ctx, actor.ID)
			return err
		}
		_, earned, err := svc.SellItem(ctx, actor.ID, items[4].ID)
		if errors.Is(err, depot.ErrItemNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if earned != 120 {
			return errors.New("unexpected sale payout")
		}
		sales.Add(1)
		return nil
	})
	if res.Failures != 0 || res.Deadlocks != 0 || sales.Load() != 1 {
		t.Fatalf("results=%+v sales=%d", res, sales.Load())
	}
	storage, err := depots.FindByCharacterID(ctx, actor.ID)
	if err != nil {
		t.Fatal(err)
	}
	wallet, err := chars.FindByID(ctx, actor.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(storage.Items, slices.Delete(slices.Clone(items), 4, 5)) || wallet.Money != actor.Money+120 {
		t.Fatalf("assets/order changed: items=%+v money=%d", storage.Items, wallet.Money)
	}
}
