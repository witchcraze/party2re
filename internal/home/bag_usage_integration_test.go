package home_test

import (
	"context"
	"errors"
	"os"
	"sync/atomic"
	"testing"
	"time"

	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/depot"
	"github.com/witchcraze/party2re/internal/economy"
	"github.com/witchcraze/party2re/internal/home"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/inventory"
	"github.com/witchcraze/party2re/internal/testutil"
)

// constRNG always returns min(v, n-1); safe for concurrent use.
type constRNG struct{ v int }

func (r constRNG) Intn(n int) int   { return min(r.v, n-1) }
func (r constRNG) IntN(n int) int   { return r.Intn(n) }
func (r constRNG) Float64() float64 { return 0 }

type bagFixture struct {
	charID    string
	bagID     string
	depotRepo *database.DepotRepository
	invRepo   *database.InventoryRepository
	newSvc    func(home.DepotManager, int) *home.Service
}

// newBagFixture stores one bag in source and fills the Depot with filler
// slots relative to capacity (delta −1, 0 or +1).
func newBagFixture(t *testing.T, name, bagDef, source string, delta int) bagFixture {
	t.Helper()
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}
	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()
	char, err := database.CreateTestCharacter(ctx, db, name)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		for _, q := range []string{
			"DELETE FROM inventory_items WHERE character_id = ?",
			"DELETE FROM depot_items WHERE character_id = ?",
			"DELETE FROM character_depots WHERE character_id = ?",
			"DELETE FROM characters WHERE id = ?",
		} {
			_, _ = db.ExecContext(context.Background(), q, char.ID)
		}
		_, _ = db.ExecContext(context.Background(), "DELETE FROM players WHERE id = ?", char.PlayerID)
	})

	f := bagFixture{charID: char.ID, bagID: id.New()}
	charRepo, err := database.NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	if f.invRepo, err = database.NewInventoryRepository(db); err != nil {
		t.Fatal(err)
	}
	if f.depotRepo, err = database.NewDepotRepository(db); err != nil {
		t.Fatal(err)
	}
	bag := coreitem.Instance{ID: f.bagID, DefinitionID: bagDef, Quantity: 1}
	inv, err := coreinventory.New(char.ID)
	if err != nil {
		t.Fatal(err)
	}
	dp, err := depot.NewDepot(char.ID)
	if err != nil {
		t.Fatal(err)
	}
	if source == "inventory" {
		if err := inv.Add(bag); err != nil {
			t.Fatal(err)
		}
	} else {
		dp.Items = append(dp.Items, bag)
	}
	for len(dp.Items) < dp.Capacity+delta {
		dp.Items = append(dp.Items, coreitem.Instance{ID: id.New(), DefinitionID: "item-018", Quantity: 1})
	}
	if err := f.invRepo.Save(ctx, inv); err != nil {
		t.Fatal(err)
	}
	if err := f.depotRepo.Save(ctx, dp); err != nil {
		t.Fatal(err)
	}

	eco, err := economy.NewService(charRepo, f.invRepo, economy.WithTransactionProvider(database.NewTransactionProvider(db)))
	if err != nil {
		t.Fatal(err)
	}
	invService, err := inventory.NewService(f.invRepo)
	if err != nil {
		t.Fatal(err)
	}
	homeRepo, err := database.NewHomeRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	cat, err := coreitem.InitialCatalog()
	if err != nil {
		t.Fatal(err)
	}
	f.newSvc = func(dm home.DepotManager, roll int) *home.Service {
		if dm == nil {
			dm = &testDepotManagerAdapter{repo: f.depotRepo}
		}
		svc, err := home.NewService(homeRepo, charRepo,
			home.WithEconomy(eco), home.WithInventoryManager(invService),
			home.WithDepotManager(dm), home.WithItemCatalog(cat), home.WithRNG(constRNG{roll}))
		if err != nil {
			t.Fatal(err)
		}
		return svc
	}
	return f
}

// state returns the Depot and the bag quantity left in either source.
func (f bagFixture) state(t *testing.T) (dp depot.Depot, bags int) {
	t.Helper()
	ctx := context.Background()
	dp, err := f.depotRepo.FindByCharacterID(ctx, f.charID)
	if err != nil {
		t.Fatal(err)
	}
	inv, err := f.invRepo.FindByCharacterID(ctx, f.charID)
	if err != nil {
		t.Fatal(err)
	}
	if it, ok := inv.Find(f.bagID); ok {
		bags += it.Quantity
	}
	for _, it := range dp.Items {
		if it.ID == f.bagID {
			bags += it.Quantity
		}
	}
	return dp, bags
}

func rewardsOf(dp depot.Depot, bagID string) []string {
	var out []string
	for _, it := range dp.Items {
		if it.ID != bagID && it.DefinitionID != "item-018" {
			out = append(out, it.DefinitionID)
		}
	}
	return out
}

func TestHomeRewardBag_Integration_SourcesAndCapacity(t *testing.T) {
	for _, tc := range []struct {
		bagDef string
		roll   int // constRNG: count roll and every slot roll
		want   []string
	}{
		{"item-125", 0, []string{"item-001"}},
		{"item-126", 99, []string{"item-136"}},
		{"item-199", 99, []string{"item-249", "item-249", "item-249", "item-249", "item-249"}},
	} {
		for _, source := range []string{"inventory", "depot"} {
			for _, delta := range []int{-1, 0, 1} {
				t.Run(tc.bagDef+"/"+source+"/"+[]string{"below", "at", "above"}[delta+1], func(t *testing.T) {
					f := newBagFixture(t, "BagCap", tc.bagDef, source, delta)
					before, _ := f.state(t)
					res, err := f.newSvc(nil, tc.roll).UseHomeItem(context.Background(), f.charID, f.bagID, source)
					if err != nil {
						t.Fatal(err)
					}
					if !res.Consumed {
						t.Fatalf("bag not consumed: %+v", res)
					}
					after, bags := f.state(t)
					got := rewardsOf(after, f.bagID)
					if bags != 0 || len(got) != len(tc.want) {
						t.Fatalf("bags=%d rewards=%v, want 0 and %v", bags, got, tc.want)
					}
					for i := range got {
						if got[i] != tc.want[i] {
							t.Fatalf("rewards=%v, want %v", got, tc.want)
						}
					}
					wantSlots := len(before.Items) + len(tc.want)
					if source == "depot" {
						wantSlots--
					}
					if len(after.Items) != wantSlots || after.Capacity != before.Capacity {
						t.Fatalf("slots=%d capacity=%d, want %d/%d", len(after.Items), after.Capacity, wantSlots, before.Capacity)
					}
				})
			}
		}
	}
}

// failAfterDelivery saves the rewards, then fails so the transaction must roll back.
type failAfterDelivery struct{ *testDepotManagerAdapter }

func (f failAfterDelivery) AppendDeliveries(ctx context.Context, characterID string, items []coreitem.Instance) error {
	if err := f.testDepotManagerAdapter.AppendDeliveries(ctx, characterID, items); err != nil {
		return err
	}
	return errors.New("injected failure after delivery")
}

func TestHomeRewardBag_Integration_FailureRollsBack(t *testing.T) {
	for _, source := range []string{"inventory", "depot"} {
		t.Run(source, func(t *testing.T) {
			f := newBagFixture(t, "BagFail", "item-199", source, 0)
			before, _ := f.state(t)
			failing := failAfterDelivery{&testDepotManagerAdapter{repo: f.depotRepo}}
			if _, err := f.newSvc(failing, 3).UseHomeItem(context.Background(), f.charID, f.bagID, source); err == nil {
				t.Fatal("expected injected failure")
			}
			after, bags := f.state(t)
			if bags != 1 || len(after.Items) != len(before.Items) {
				t.Fatalf("failure must keep the bag and add nothing: bags=%d slots=%d/%d", bags, len(after.Items), len(before.Items))
			}
		})
	}
}

func TestHomeRewardBag_Integration_ConcurrentDoubleUse(t *testing.T) {
	for _, source := range []string{"inventory", "depot"} {
		t.Run(source, func(t *testing.T) {
			f := newBagFixture(t, "BagCc", "item-199", source, 0)
			svc := f.newSvc(nil, 3)
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			var used, missing int32
			res := testutil.RunConcurrentStressTest(t, testutil.ConcurrencyStressConfig{Workers: 2, OpsPerWorker: 1}, func(_, _ int) error {
				_, err := svc.UseHomeItem(ctx, f.charID, f.bagID, source)
				switch {
				case err == nil:
					atomic.AddInt32(&used, 1)
				case errors.Is(err, home.ErrItemNotFound):
					atomic.AddInt32(&missing, 1)
				default:
					return err
				}
				return nil
			})
			if res.Failures+res.Deadlocks > 0 {
				t.Fatalf("failures=%d deadlocks=%d", res.Failures, res.Deadlocks)
			}
			after, bags := f.state(t)
			if used != 1 || missing != 1 || bags != 0 || len(rewardsOf(after, f.bagID)) != 5 {
				t.Fatalf("used=%d missing=%d bags=%d rewards=%d", used, missing, bags, len(rewardsOf(after, f.bagID)))
			}
		})
	}
}
