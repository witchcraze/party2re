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

var errSalePersistence = errors.New("sale character save failed")

type saleFailingCharacterSave struct{ *database.CharacterRepository }

func (saleFailingCharacterSave) Update(context.Context, corecharacter.Character) error {
	return errSalePersistence
}

type saleDBCatalog struct{ failID string }

func (c saleDBCatalog) FindByID(id string) (item.Definition, error) {
	if id == c.failID {
		return item.Definition{}, errSalePersistence
	}
	return item.Definition{ID: id, Price: 101}, nil
}

func saleDBFixture(t *testing.T, failSave bool, failID string) (*depot.Service, corecharacter.Character, *database.CharacterRepository, *database.DepotRepository, []item.Instance) {
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
	var characters depot.CharacterRepository = chars
	if failSave {
		characters = saleFailingCharacterSave{chars}
	}
	svc, err := depot.NewServiceWithTransaction(depots, characters, inv, depots, depot.WithItemDefinitionProvider(saleDBCatalog{failID}))
	if err != nil {
		t.Fatal(err)
	}
	return svc, actor, chars, depots, items
}

func TestDepotSaleRollback_Integration(t *testing.T) {
	for _, tc := range []struct {
		name             string
		single, failSave bool
		failID           string
	}{
		{"single save failure", true, true, ""},
		{"batch save failure", false, true, ""},
		{"late catalog failure", false, false, "second"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, actor, chars, depots, items := saleDBFixture(t, tc.failSave, tc.failID)
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
			if !errors.Is(err, errSalePersistence) || earned != 0 {
				t.Fatalf("earned=%d error=%v", earned, err)
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
