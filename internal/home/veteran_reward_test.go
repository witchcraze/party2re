package home

import (
	"context"
	"errors"
	"testing"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/depot"
)

// floatRNG returns a fixed Float64 for U[1,1.5) endpoint checks.
type floatRNG struct{ f float64 }

func (r floatRNG) Intn(int) int     { return 0 }
func (r floatRNG) IntN(int) int     { return 0 }
func (r floatRNG) Float64() float64 { return r.f }

const rngTop = 0.9999999999 // largest practical rand(0.5)+1 draw

// Expected values are computed independently from legacy _data.cgi:1994:
// gold += int((exp-98010) * (rand(0.5)+1)), gold capped at 999999, exp=98010.
func TestUseHomeItem_VeteranReward_Boundaries(t *testing.T) {
	cases := []struct {
		name                string
		level, exp, gold    int
		rng                 float64
		wantExp, wantGold   int
		wantConsumed, noEff bool
		wantMsg             string
	}{
		{name: "Lv98 unchanged", level: 98, exp: 98020, gold: 100, wantExp: 98020, wantGold: 100, noEff: true, wantMsg: noEffectMessage},
		{name: "Lv99 EXP98000 low", level: 99, exp: 98000, gold: 100, wantExp: 98010, wantGold: 90, noEff: true, wantMsg: noEffectMessage},
		{name: "Lv99 EXP98000 high", level: 99, exp: 98000, gold: 100, rng: rngTop, wantExp: 98010, wantGold: 86, noEff: true, wantMsg: noEffectMessage},
		{name: "Lv99 EXP98010", level: 99, exp: 98010, gold: 100, rng: rngTop, wantExp: 98010, wantGold: 100, noEff: true, wantMsg: noEffectMessage},
		{name: "Lv99 EXP98020 low", level: 99, exp: 98020, gold: 100, wantExp: 98010, wantGold: 110, wantConsumed: true, wantMsg: "これまでの戦いの報酬として 10 Gもらった!"},
		{name: "Lv99 EXP98020 high", level: 99, exp: 98020, gold: 100, rng: rngTop, wantExp: 98010, wantGold: 114, wantConsumed: true, wantMsg: "これまでの戦いの報酬として 14 Gもらった!"},
		{name: "gold cap still consumes", level: 99, exp: 98020, gold: 999999, wantExp: 98010, wantGold: 999999, wantConsumed: true, wantMsg: "これまでの戦いの報酬として 10 Gもらった!"},
		{name: "insufficient gold goes negative", level: 99, exp: 96040, gold: 100, rng: rngTop, wantExp: 98010, wantGold: 100 - 2954, noEff: true, wantMsg: noEffectMessage},
		{name: "over-level", level: 120, exp: 144000, gold: 0, wantExp: 98010, wantGold: 45990, wantConsumed: true, wantMsg: "これまでの戦いの報酬として 45990 Gもらった!"},
	}
	for _, tc := range cases {
		for _, source := range []string{"inventory", "depot"} {
			t.Run(tc.name+"/"+source, func(t *testing.T) {
				svc, runner, depots := newVeteranRewardService(t, tc.level, tc.exp, tc.gold, tc.rng)
				res, err := svc.UseHomeItem(context.Background(), "vet", "vet-"+source, source)
				if err != nil {
					t.Fatalf("UseHomeItem: %v", err)
				}
				if res.Consumed != tc.wantConsumed || (res.Action == "no_effect") != tc.noEff || res.Message != tc.wantMsg {
					t.Fatalf("result = %+v", res)
				}
				got := runner.chars["vet"]
				if got.Experience != tc.wantExp || got.Money != tc.wantGold || res.Character.Money != tc.wantGold {
					t.Fatalf("stored exp/gold = %d/%d, result gold %d; want %d/%d", got.Experience, got.Money, res.Character.Money, tc.wantExp, tc.wantGold)
				}
				inv := runner.invs["vet"]
				_, inInv := inv.Find("vet-inventory")
				inDepot := len(depots.depots["vet"].Items) == depot.DefaultDepotCapacity
				owned := map[string]bool{"inventory": inInv, "depot": inDepot}[source]
				if owned == tc.wantConsumed {
					t.Fatalf("%s item owned=%v, want consumed=%v", source, owned, tc.wantConsumed)
				}
			})
		}
	}
}

func TestUseHomeItem_VeteranReward_FailureRollsBack(t *testing.T) {
	for _, exp := range []int{98000, 98020} {
		svc, runner, _ := newVeteranRewardService(t, 99, exp, 100, 0)
		runner.failOnCommit = true
		if _, err := svc.UseHomeItem(context.Background(), "vet", "vet-inventory", "inventory"); err == nil || errors.Is(err, errNoEffect) {
			t.Fatalf("exp %d: want storage failure, got %v", exp, err)
		}
		if got := runner.chars["vet"]; got.Experience != exp || got.Money != 100 {
			t.Fatalf("exp %d: failure must not persist mutations, got %d/%d", exp, got.Experience, got.Money)
		}
		inv := runner.invs["vet"]
		if _, ok := inv.Find("vet-inventory"); !ok {
			t.Fatalf("exp %d: failure must keep the item", exp)
		}
	}
}

// newVeteranRewardService stores item-237 in inventory and in a full Depot.
func newVeteranRewardService(t *testing.T, level, exp, gold int, rng float64) (*Service, *mockTransactionRunner, *transactionalMockDepotManager) {
	t.Helper()
	cat, err := coreitem.InitialCatalog()
	if err != nil {
		t.Fatal(err)
	}
	chars := map[string]corecharacter.Character{"vet": {ID: "vet", Name: "Vet", Level: level, Experience: exp, Money: gold}}
	inv, err := coreinventory.New("vet")
	if err != nil {
		t.Fatal(err)
	}
	if err := inv.Add(coreitem.Instance{ID: "vet-inventory", DefinitionID: "item-237", Quantity: 1}); err != nil {
		t.Fatal(err)
	}
	invs := map[string]coreinventory.Inventory{"vet": inv}
	dp, err := depot.NewDepot("vet")
	if err != nil {
		t.Fatal(err)
	}
	dp.Items = append(dp.Items, coreitem.Instance{ID: "vet-depot", DefinitionID: "item-237", Quantity: 1})
	for len(dp.Items) < depot.DefaultDepotCapacity {
		dp.Items = append(dp.Items, coreitem.Instance{ID: "filler-" + string(rune('a'+len(dp.Items))), DefinitionID: "item-018", Quantity: 1})
	}
	depots := &transactionalMockDepotManager{depots: map[string]depot.Depot{"vet": dp}}
	runner := &mockTransactionRunner{chars: chars, invs: invs}
	svc, err := NewService(newMockHomeRepo(chars), &mockCharReader{chars: chars},
		WithInventoryManager(&mockInventoryManager{invs: invs}),
		WithDepotManager(depots),
		WithItemCatalog(cat),
		WithTransactionRunner(runner),
		WithRNG(floatRNG{f: rng}),
	)
	if err != nil {
		t.Fatal(err)
	}
	return svc, runner, depots
}

// Without a TransactionRunner, consumption must still be decided from the
// pre-effect EXP (the effect resets it to 98010).
func TestUseHomeItem_VeteranReward_FallbackDecidesBeforeEffect(t *testing.T) {
	for _, tc := range []struct {
		exp          int
		wantGold     int
		wantConsumed bool
	}{{98020, 110, true}, {98000, 90, false}} {
		for _, source := range []string{"inventory", "depot"} {
			_, runner, depots := newVeteranRewardService(t, 99, tc.exp, 100, 0)
			cat, err := coreitem.InitialCatalog()
			if err != nil {
				t.Fatal(err)
			}
			updater := &mockCharUpdater{chars: runner.chars}
			svc, err := NewService(newMockHomeRepo(runner.chars), &mockCharReader{chars: runner.chars},
				WithInventoryManager(&mockInventoryManager{invs: runner.invs}),
				WithDepotManager(depots),
				WithItemCatalog(cat),
				WithCharacterUpdater(updater),
				WithRNG(floatRNG{}),
			)
			if err != nil {
				t.Fatal(err)
			}
			res, err := svc.UseHomeItem(context.Background(), "vet", "vet-"+source, source)
			if err != nil {
				t.Fatalf("exp %d/%s: %v", tc.exp, source, err)
			}
			got := updater.chars["vet"]
			inv := runner.invs["vet"]
			_, inInv := inv.Find("vet-inventory")
			owned := map[string]bool{"inventory": inInv, "depot": len(depots.depots["vet"].Items) == depot.DefaultDepotCapacity}[source]
			if res.Consumed != tc.wantConsumed || owned == tc.wantConsumed || got.Experience != 98010 || got.Money != tc.wantGold {
				t.Fatalf("exp %d/%s: res=%+v owned=%v stored=%d/%d", tc.exp, source, res, owned, got.Experience, got.Money)
			}
		}
	}
}
