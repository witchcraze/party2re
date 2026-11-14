package depot_test

import (
	"context"
	"errors"
	"os"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/depot"
	"github.com/witchcraze/party2re/internal/testutil"
)

var (
	errSalePersistence = errors.New("sale character save failed")
	errSaleDepotSave   = errors.New("sale depot save failed")
)

type saleFailingCharacterSave struct{ *database.CharacterRepository }

func (saleFailingCharacterSave) Update(context.Context, corecharacter.Character) error {
	return errSalePersistence
}

type saleFailingDepotSave struct{ *database.DepotRepository }

func (saleFailingDepotSave) Save(context.Context, depot.Depot) error {
	return errSaleDepotSave
}

type saleDBCatalog struct{ failID string }

func (c saleDBCatalog) FindByID(id string) (item.Definition, error) {
	if id == c.failID {
		return item.Definition{}, errSalePersistence
	}
	return item.Definition{ID: id, Price: 101}, nil
}

type saleFixedPriceCatalog struct{ price int }

func (c saleFixedPriceCatalog) FindByID(id string) (item.Definition, error) {
	return item.Definition{ID: id, Price: c.price}, nil
}

func saleDBFixtureWithOptions(t *testing.T, failCharSave, failDepotSave bool, catalog depot.ItemDefinitionProvider) (*depot.Service, corecharacter.Character, *database.CharacterRepository, *database.DepotRepository, []item.Instance) {
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
	chars, err := database.NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	inv, err := database.NewInventoryRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	depots, err := database.NewDepotRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	actor, err := database.CreateTestCharacterWithFunds(context.Background(), db, "Depot sale", 1000)
	if err != nil {
		t.Fatal(err)
	}
	items := make([]item.Instance, 3)
	for i, defID := range []string{"first", "second", "survivor"} {
		items[i], err = item.NewInstance(defID, []int{2, 1, 4}[i])
		if err != nil {
			t.Fatal(err)
		}
	}
	items[0].EnhancementLevel = 6
	if _, err := database.CreateTestDepot(context.Background(), db, actor.ID, 0, items); err != nil {
		t.Fatal(err)
	}
	var storage depot.Repository = depots
	if failDepotSave {
		storage = saleFailingDepotSave{depots}
	}
	var characters depot.CharacterRepository = chars
	if failCharSave {
		characters = saleFailingCharacterSave{chars}
	}
	var opts []depot.Option
	if catalog != nil {
		opts = append(opts, depot.WithItemDefinitionProvider(catalog))
	}
	svc, err := depot.NewServiceWithTransaction(storage, characters, inv, depots, opts...)
	if err != nil {
		t.Fatal(err)
	}
	return svc, actor, chars, depots, items
}

func saleDBFixture(t *testing.T, failSave bool, failID string) (*depot.Service, corecharacter.Character, *database.CharacterRepository, *database.DepotRepository, []item.Instance) {
	t.Helper()
	return saleDBFixtureWithOptions(t, failSave, false, saleDBCatalog{failID})
}

func TestDepotSaleRollback_Integration(t *testing.T) {
	for _, tc := range []struct {
		name                        string
		single, failSave, failDepot bool
		failID                      string
		wantErr                     error
	}{
		{"single save failure", true, true, false, "", errSalePersistence},
		{"batch save failure", false, true, false, "", errSalePersistence},
		{"depot save failure", false, false, true, "", errSaleDepotSave},
		{"late catalog failure", false, false, false, "second", errSalePersistence},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, actor, chars, depots, items := saleDBFixtureWithOptions(t, tc.failSave, tc.failDepot, saleDBCatalog{tc.failID})
			ctx := context.Background()
			before, err := depots.FindByCharacterID(ctx, actor.ID)
			if err != nil {
				t.Fatal(err)
			}
			var earned int
			if tc.single {
				_, earned, err = svc.SellItem(ctx, actor.ID, items[0].ID)
			} else {
				_, earned, err = svc.SellItems(ctx, actor.ID, []string{items[0].ID, items[1].ID})
			}
			if !errors.Is(err, tc.wantErr) || earned != 0 {
				t.Fatalf("earned=%d error=%v, want %v", earned, err, tc.wantErr)
			}
			after, err := depots.FindByCharacterID(ctx, actor.ID)
			if err != nil {
				t.Fatal(err)
			}
			wallet, err := chars.FindByID(ctx, actor.ID)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(before, after) || wallet.Money != actor.Money {
				t.Fatalf("failed sale changed assets: before=%+v after=%+v money=%d", before, after, wallet.Money)
			}
		})
	}
}

func TestDepotSalePersistenceAndRejections_Integration(t *testing.T) {
	for _, tc := range []struct {
		name     string
		single   bool
		price    int
		provider bool
		mode     string
		wantEarn int
		wantErr  error
	}{
		{"single", true, 101, true, "", 100, nil},
		{"batch", false, 101, true, "", 150, nil},
		{"zero price", true, 0, true, "", 0, nil},
		{"batch zero price", false, 0, true, "", 0, nil},
		{"missing provider", true, 101, false, "", 0, nil},
		{"missing single", true, 101, true, "missing", 0, depot.ErrItemNotFound},
		{"missing batch", false, 101, true, "missing", 0, depot.ErrItemNotFound},
		{"duplicate batch", false, 101, true, "duplicate", 0, depot.ErrItemNotFound},
		{"blank single", true, 101, true, "empty", 0, depot.ErrInvalidItemInstanceID},
		{"empty batch", false, 101, true, "empty", 0, depot.ErrEmptyItemList},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var catalog depot.ItemDefinitionProvider
			if tc.provider {
				catalog = saleFixedPriceCatalog{price: tc.price}
			}
			svc, actor, chars, depots, items := saleDBFixtureWithOptions(t, false, false, catalog)
			ctx := context.Background()
			before, err := depots.FindByCharacterID(ctx, actor.ID)
			if err != nil {
				t.Fatal(err)
			}
			ids := []string{items[0].ID}
			if !tc.single {
				ids = append(ids, items[1].ID)
			}
			switch tc.mode {
			case "missing":
				ids[len(ids)-1] = "missing"
			case "duplicate":
				ids[1] = ids[0]
			case "empty":
				if tc.single {
					ids[0] = ""
				} else {
					ids = []string{}
				}
			}
			var gotDepot depot.Depot
			var earned int
			if tc.single {
				gotDepot, earned, err = svc.SellItem(ctx, actor.ID, ids[0])
			} else {
				gotDepot, earned, err = svc.SellItems(ctx, actor.ID, ids)
			}
			wantSuccess := tc.provider && tc.wantErr == nil
			if wantSuccess {
				if err != nil || earned != tc.wantEarn {
					t.Fatalf("sale failed: earned=%d err=%v", earned, err)
				}
			} else {
				if err == nil || earned != 0 || (tc.wantErr != nil && !errors.Is(err, tc.wantErr)) {
					t.Fatalf("unexpected rejection: earned=%d err=%v want=%v", earned, err, tc.wantErr)
				}
			}
			wallet, err := chars.FindByID(ctx, actor.ID)
			if err != nil {
				t.Fatal(err)
			}
			after, err := depots.FindByCharacterID(ctx, actor.ID)
			if err != nil {
				t.Fatal(err)
			}
			if !wantSuccess {
				if !reflect.DeepEqual(before, after) || wallet.Money != actor.Money {
					t.Fatalf("rejected sale changed assets: before=%+v after=%+v money=%d", before, after, wallet.Money)
				}
				return
			}
			gotDepot.ItemDefs = nil
			if wallet.Money != actor.Money+tc.wantEarn || !reflect.DeepEqual(gotDepot, after) {
				t.Fatalf("persisted sale mismatch: wallet=%d got=%+v after=%+v", wallet.Money, gotDepot, after)
			}
			remaining := map[string]item.Instance{}
			for _, stored := range after.Items {
				remaining[stored.ID] = stored
			}
			if len(remaining) != 3-len(ids) || !reflect.DeepEqual(remaining[items[2].ID], items[2]) {
				t.Fatalf("unselected item changed: %+v", after.Items)
			}
			for _, id := range ids {
				if _, exists := remaining[id]; exists {
					t.Fatalf("sold item %s remains", id)
				}
			}
			if tc.single && !reflect.DeepEqual(remaining[items[1].ID], items[1]) {
				t.Fatalf("single sale changed second item: %+v", remaining[items[1].ID])
			}
		})
	}
}

func TestDepotSaleConcurrency_Integration(t *testing.T) {
	svc, actor, chars, depots, items := saleDBFixture(t, false, "")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var paid atomic.Int64
	var successes atomic.Int64
	res := testutil.RunConcurrentStressTest(t, testutil.GetStressConfig(), func(worker, op int) error {
		var earned int
		var err error
		if (worker+op)%2 == 0 {
			_, earned, err = svc.SellItem(ctx, actor.ID, items[0].ID)
		} else {
			_, earned, err = svc.SellItems(ctx, actor.ID, []string{items[0].ID, items[1].ID})
		}
		if errors.Is(err, depot.ErrItemNotFound) {
			return nil
		}
		if err == nil {
			successes.Add(1)
			paid.Add(int64(earned))
		}
		return err
	})
	if res.Failures != 0 || successes.Load() != 1 {
		t.Fatalf("results=%+v sales=%d", res, successes.Load())
	}
	wallet, err := chars.FindByID(ctx, actor.ID)
	if err != nil {
		t.Fatal(err)
	}
	storage, err := depots.FindByCharacterID(ctx, actor.ID)
	if err != nil {
		t.Fatal(err)
	}
	remaining := map[string]item.Instance{}
	for _, stored := range storage.Items {
		remaining[stored.ID] = stored
	}
	if _, exists := remaining[items[0].ID]; exists {
		t.Fatal("sold first item remains")
	}
	if !reflect.DeepEqual(remaining[items[2].ID], items[2]) {
		t.Fatal("unselected item changed")
	}
	if paid.Load() == 100 {
		if len(remaining) != 2 || !reflect.DeepEqual(remaining[items[1].ID], items[1]) {
			t.Fatal("single sale changed second item")
		}
	} else if paid.Load() == 150 {
		if len(remaining) != 1 {
			t.Fatal("batch sale left selected items")
		}
	} else {
		t.Fatalf("invalid sale payment %d", paid.Load())
	}
	if wallet.Money != actor.Money+int(paid.Load()) {
		t.Fatalf("money=%d paid=%d", wallet.Money, paid.Load())
	}
}
