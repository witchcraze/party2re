package casino_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"testing"

	"github.com/witchcraze/party2re/internal/casino"
	"github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/depot"
	"github.com/witchcraze/party2re/internal/testutil"
)

type prizeDepotFaultRepository struct {
	*database.DepotRepository
	afterSave func(context.Context, depot.Depot) error
}

func (r *prizeDepotFaultRepository) Save(ctx context.Context, dep depot.Depot) error {
	if err := r.DepotRepository.Save(ctx, dep); err != nil {
		return err
	}
	if r.afterSave != nil {
		return r.afterSave(ctx, dep)
	}
	return nil
}

type prizeCoinFaultRepository struct {
	*database.CasinoRepository
	afterDebit func(context.Context, casino.Account) error
}

func (r *prizeCoinFaultRepository) AdjustCoins(ctx context.Context, charID string, delta int64) (casino.Account, error) {
	acc, err := r.CasinoRepository.AdjustCoins(ctx, charID, delta)
	if err != nil {
		return casino.Account{}, err
	}
	if delta < 0 && r.afterDebit != nil {
		return casino.Account{}, r.afterDebit(ctx, acc)
	}
	return acc, nil
}

type prizeDatabaseTest struct {
	svc    *casino.Service
	depots *prizeDepotFaultRepository
	coins  *prizeCoinFaultRepository
	charID string
}

func newPrizeDatabaseTest(t *testing.T, coins int64, exDepot int, items []item.Instance) prizeDatabaseTest {
	t.Helper()
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}
	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	coinRepo, err := database.NewCasinoRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	depotRepo, err := database.NewDepotRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	charRepo, err := database.NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	f := prizeDatabaseTest{
		depots: &prizeDepotFaultRepository{DepotRepository: depotRepo},
		coins:  &prizeCoinFaultRepository{CasinoRepository: coinRepo},
	}
	f.svc, err = casino.NewService(f.coins,
		casino.WithDepotRepository(f.depots),
		casino.WithCharacterRepository(charRepo),
		casino.WithTransactionProvider(database.NewTransactionProvider(db)),
	)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	char, err := database.CreateTestCharacterWithFunds(ctx, db, "PrizeAssets", int(coins)*casino.GoldPerCoin)
	if err != nil {
		t.Fatal(err)
	}
	f.charID = char.ID
	if _, _, err := f.svc.ExchangeGoldToCoins(ctx, char.ID, coins); err != nil {
		t.Fatal(err)
	}
	if _, err := database.CreateTestDepot(ctx, db, char.ID, exDepot, items); err != nil {
		t.Fatal(err)
	}
	return f
}

func (f prizeDatabaseTest) readAssets(t *testing.T) (casino.Account, depot.Depot) {
	t.Helper()
	ctx := context.Background()
	acc, err := f.coins.GetAccount(ctx, f.charID)
	if err != nil {
		t.Fatal(err)
	}
	dep, err := f.depots.FindByCharacterID(ctx, f.charID)
	if err != nil {
		t.Fatal(err)
	}
	return acc, dep
}

func prizeExistingItems(t *testing.T) []item.Instance {
	t.Helper()
	var items []item.Instance
	for _, spec := range []struct {
		id       string
		quantity int
	}{{"weapon-31", 1}, {"armor-34", 1}, {"item-004", 7}} {
		inst, err := item.NewInstance(spec.id, spec.quantity)
		if err != nil {
			t.Fatal(err)
		}
		items = append(items, inst)
	}
	return items
}

func assertPrizeItems(t *testing.T, before, after depot.Depot, prize casino.Prize, purchased int) {
	t.Helper()
	existing := make(map[string]item.Instance)
	for _, inst := range before.Items {
		existing[inst.ID] = inst
	}
	seen := make(map[string]bool)
	newCount := 0
	for _, inst := range after.Items {
		if inst.ID == "" || seen[inst.ID] {
			t.Fatalf("empty or duplicate instance ID: %+v", inst)
		}
		seen[inst.ID] = true
		if old, ok := existing[inst.ID]; ok {
			if old.DefinitionID == prize.ItemID && prize.Category == 3 {
				old.Quantity += purchased
			}
			if inst != old {
				t.Errorf("existing instance changed: got %+v, want %+v", inst, old)
			}
			delete(existing, inst.ID)
		} else {
			newCount++
			if prize.Category == 3 || inst.DefinitionID != prize.ItemID || inst.Quantity != 1 || inst.EnhancementLevel != 0 {
				t.Errorf("unexpected new instance: %+v", inst)
			}
		}
	}
	if len(existing) != 0 {
		t.Errorf("existing instances lost: %+v", existing)
	}
	wantNew := purchased
	if prize.Category == 3 {
		wantNew = 0
	}
	if newCount != wantNew || after.Quantity(prize.ItemID) != before.Quantity(prize.ItemID)+purchased {
		t.Errorf("prize conservation failed: new instances=%d (want %d), before=%+v, after=%+v", newCount, wantNew, before.Items, after.Items)
	}
}

func TestExchangePrizeDatabaseCapacityRollback(t *testing.T) {
	for _, cost := range []int64{30000, 8000, 100} {
		t.Run(fmt.Sprint(cost), func(t *testing.T) {
			var items []item.Instance
			fill := depot.DefaultDepotCapacity - 1
			if cost == 100 {
				fill++ // A new stack needs a free slot; existing stacks can grow at capacity.
			}
			for i := 0; i < fill; i++ {
				inst, err := item.NewInstance("weapon-31", 1)
				if err != nil {
					t.Fatal(err)
				}
				items = append(items, inst)
			}
			f := newPrizeDatabaseTest(t, 100000, 0, items)
			beforeAcc, beforeDep := f.readAssets(t)
			_, err := f.svc.ExchangePrize(context.Background(), f.charID, cost, 2)
			if !errors.Is(err, depot.ErrDepotFull) {
				t.Fatalf("ExchangePrize error = %v, want ErrDepotFull", err)
			}
			afterAcc, afterDep := f.readAssets(t)
			if afterAcc.Coins != beforeAcc.Coins || !reflect.DeepEqual(afterDep, beforeDep) {
				t.Fatalf("capacity rejection changed assets: coins=%d, depot=%+v", afterAcc.Coins, afterDep)
			}
		})
	}
}

func TestExchangePrizeDatabaseLateFailureRollback(t *testing.T) {
	for _, cost := range []int64{30000, 8000, 100} {
		for _, stage := range []string{"after_depot_save", "after_coin_debit"} {
			t.Run(fmt.Sprintf("%d/%s", cost, stage), func(t *testing.T) {
				f := newPrizeDatabaseTest(t, 100000, 0, prizeExistingItems(t))
				beforeAcc, beforeDep := f.readAssets(t)
				prize, err := casino.GetPrizeByCost(cost)
				if err != nil {
					t.Fatal(err)
				}
				injected := errors.New("injected late prize failure")
				saveCalls, debitCalls := 0, 0
				f.depots.afterSave = func(ctx context.Context, saved depot.Depot) error {
					saveCalls++
					persisted, err := f.depots.FindByCharacterID(ctx, f.charID)
					if err != nil {
						return err
					}
					if !reflect.DeepEqual(persisted, saved) {
						t.Fatalf("depot Save did not persist proposed state: got %+v, want %+v", persisted, saved)
					}
					assertPrizeItems(t, beforeDep, persisted, prize, 2)
					if stage == "after_depot_save" {
						return injected
					}
					return nil
				}
				f.coins.afterDebit = func(ctx context.Context, acc casino.Account) error {
					debitCalls++
					persisted, err := f.coins.GetAccount(ctx, f.charID)
					if err != nil {
						return err
					}
					if persisted.Coins != beforeAcc.Coins-2*cost || persisted.Coins != acc.Coins {
						t.Fatalf("coin debit did not persist: got %d, want %d", persisted.Coins, beforeAcc.Coins-2*cost)
					}
					return injected
				}
				_, err = f.svc.ExchangePrize(context.Background(), f.charID, cost, 2)
				if !errors.Is(err, injected) {
					t.Fatalf("ExchangePrize error = %v, want injected failure", err)
				}
				wantDebits := 0
				if stage == "after_coin_debit" {
					wantDebits = 1
				}
				if saveCalls != 1 || debitCalls != wantDebits {
					t.Fatalf("fault boundary not reached: saves=%d, debits=%d", saveCalls, debitCalls)
				}
				afterAcc, afterDep := f.readAssets(t)
				if afterAcc.Coins != beforeAcc.Coins || !reflect.DeepEqual(afterDep, beforeDep) {
					t.Fatalf("late failure changed assets: coins=%d, depot=%+v", afterAcc.Coins, afterDep)
				}

				f.depots.afterSave, f.coins.afterDebit = nil, nil
				res, err := f.svc.ExchangePrize(context.Background(), f.charID, cost, 2)
				if err != nil {
					t.Fatalf("retry after clearing fault: %v", err)
				}
				afterAcc, afterDep = f.readAssets(t)
				if afterAcc.Coins != beforeAcc.Coins-2*cost || res.RemainingCoins != afterAcc.Coins || res.TotalCostCoins != 2*cost || res.Count != 2 || !res.TransferredToDepot {
					t.Fatalf("retry cost mismatch: account=%+v, result=%+v", afterAcc, res)
				}
				assertPrizeItems(t, beforeDep, afterDep, prize, 2)
			})
		}
	}
}

func TestConcurrencyStressCasinoPrizeConservation(t *testing.T) {
	for _, cost := range []int64{30000, 8000, 100} {
		t.Run(fmt.Sprint(cost), func(t *testing.T) {
			cfg := testutil.GetStressConfig()
			prize, err := casino.GetPrizeByCost(cost)
			if err != nil {
				t.Fatal(err)
			}
			const bulkCount = 2
			initialCoins := int64(cfg.Workers*cfg.OpsPerWorker*bulkCount) * cost
			if prize.Category == 3 {
				initialCoins = 7 * bulkCount * cost
			}
			f := newPrizeDatabaseTest(t, initialCoins, depot.MaxExDepot, prizeExistingItems(t))
			beforeAcc, beforeDep := f.readAssets(t)
			stats := testutil.RunConcurrentStressTest(t, cfg, func(workerID, op int) error {
				res, err := f.svc.ExchangePrize(context.Background(), f.charID, cost, bulkCount)
				if err != nil && !errors.Is(err, depot.ErrDepotFull) && !errors.Is(err, casino.ErrInsufficientCoins) {
					t.Errorf("unexpected exchange failure: %v", err)
				}
				if err == nil && (res.TotalCostCoins != cost*bulkCount || res.Count != bulkCount || !res.TransferredToDepot) {
					t.Errorf("unexpected successful exchange: %+v", res)
				}
				return err
			})
			wantSuccesses := int64((beforeDep.Capacity - len(beforeDep.Items)) / bulkCount)
			if prize.Category == 3 {
				wantSuccesses = beforeAcc.Coins / (cost * bulkCount)
			}
			if stats.Successes != wantSuccesses || stats.Failures != stats.TotalOps-wantSuccesses {
				t.Fatalf("unexpected successes/rejections: %+v, want %d successes", stats, wantSuccesses)
			}
			afterAcc, afterDep := f.readAssets(t)
			if beforeAcc.Coins-afterAcc.Coins != stats.Successes*cost*bulkCount {
				t.Errorf("coin conservation failed: before=%d, after=%d, successful exchanges=%d", beforeAcc.Coins, afterAcc.Coins, stats.Successes)
			}
			assertPrizeItems(t, beforeDep, afterDep, prize, int(stats.Successes)*bulkCount)
			t.Logf("prize %s: %+v", prize.ItemID, stats)
		})
	}
}
