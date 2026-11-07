package home

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/depot"
)

// scriptedRNG returns queued values and records each requested bound.
type scriptedRNG struct {
	vals   []int
	bounds []int
}

func (r *scriptedRNG) Intn(n int) int {
	r.bounds = append(r.bounds, n)
	v := r.vals[0]
	r.vals = r.vals[1:]
	return v
}
func (r *scriptedRNG) IntN(n int) int   { return r.Intn(n) }
func (r *scriptedRNG) Float64() float64 { return 0 }

func countRange(m map[int]int, from, to, weight int) {
	for n := from; n <= to; n++ {
		m[n] += weight
	}
}

// Expected weights over reachable slots 1..N−1, derived by hand from
// lib/_data.cgi:1479–1505,1809–1820 (slot 0 excluded).
func expectedBagWeights() map[string]map[int]int {
	fuku := map[int]int{1: 5, 7: 1, 130: 3}
	countRange(fuku, 7, 10, 1)
	countRange(fuku, 1, 26, 1)
	countRange(fuku, 41, 56, 1)
	countRange(fuku, 72, 87, 1)
	countRange(fuku, 101, 103, 1)
	countRange(fuku, 125, 128, 1)
	countRange(fuku, 130, 134, 1)

	koufuku := map[int]int{}
	countRange(koufuku, 129, 134, 1)
	countRange(koufuku, 128, 136, 1)

	treasure := map[int]int{23: 3, 24: 4, 25: 4, 26: 3, 105: 1, 106: 1, 134: 2, 12: 2, 15: 2, 43: 1,
		147: 2, 150: 1, 151: 2, 168: 1, 193: 3, 196: 1, 215: 2, 238: 1, 249: 1}
	return map[string]map[int]int{"福袋": fuku, "幸福袋": koufuku, "太古の財宝": treasure}
}

func TestRewardBag_PoolWeightsAndEndpoints(t *testing.T) {
	cat, err := coreitem.InitialCatalog()
	if err != nil {
		t.Fatal(err)
	}
	ends := map[string][2]int{"福袋": {1, 134}, "幸福袋": {129, 136}, "太古の財宝": {23, 249}}
	totals := map[string]int{"福袋": 83, "幸福袋": 15, "太古の財宝": 37}
	for name, want := range expectedBagWeights() {
		bag := rewardBags[name]
		reachable := len(bag.slots) - 1
		if reachable != totals[name] {
			t.Fatalf("%s: reachable slots = %d, want %d", name, reachable, totals[name])
		}
		got := map[int]int{}
		for k := 0; k < reachable; k++ {
			rng := &scriptedRNG{vals: []int{0, k, 0, 0, 0, 0}}
			rewards := bag.draw(rng.Intn)
			if rng.bounds[1] != reachable {
				t.Fatalf("%s: slot bound = %d, want %d", name, rng.bounds[1], reachable)
			}
			got[rewards[0]]++
			if k == 0 && rewards[0] != ends[name][0] || k == reachable-1 && rewards[0] != ends[name][1] {
				t.Fatalf("%s: index %d gave %d, want endpoints %v", name, k+1, rewards[0], ends[name])
			}
			if _, err := cat.FindByID(fmt.Sprintf("item-%03d", rewards[0])); err != nil {
				t.Fatalf("%s: reward %d not in catalog: %v", name, rewards[0], err)
			}
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("%s: weights = %v, want %v", name, got, want)
		}
	}
}

func TestRewardBag_CountEndpoints(t *testing.T) {
	for _, tc := range []struct {
		name      string
		countRoll int
		bound     int
		want      int
	}{
		{"福袋", 0, 1, 1},
		{"幸福袋", 0, 1, 1},
		{"太古の財宝", 0, 4, 2},
		{"太古の財宝", 3, 4, 5},
	} {
		rng := &scriptedRNG{vals: []int{tc.countRoll, 0, 0, 0, 0, 0}}
		if got := len(rewardBags[tc.name].draw(rng.Intn)); got != tc.want || rng.bounds[0] != tc.bound {
			t.Fatalf("%s roll %d: count=%d bound=%d, want %d/%d", tc.name, tc.countRoll, got, rng.bounds[0], tc.want, tc.bound)
		}
	}
}

func TestUseHomeItem_RewardBag_BothSourcesDeliverAboveCapacity(t *testing.T) {
	ctx := context.Background()
	cat, err := coreitem.InitialCatalog()
	if err != nil {
		t.Fatal(err)
	}
	for _, source := range []string{"inventory", "depot"} {
		t.Run(source, func(t *testing.T) {
			charID := "char-bag"
			chars := map[string]corecharacter.Character{charID: {ID: charID, Name: "Bag"}}
			inv, _ := coreinventory.New(charID)
			dp, _ := depot.NewDepot(charID)
			bag := coreitem.Instance{ID: "inst-bag", DefinitionID: "item-125", Quantity: 2}
			if source == "inventory" {
				if err := inv.Add(bag); err != nil {
					t.Fatal(err)
				}
			} else {
				dp.Items = append(dp.Items, bag)
			}
			for i := len(dp.Items); i < dp.Capacity; i++ {
				dp.Items = append(dp.Items, coreitem.Instance{ID: fmt.Sprintf("fill-%d", i), DefinitionID: "item-018", Quantity: 1})
			}
			invs := &mockInventoryManager{invs: map[string]coreinventory.Inventory{charID: inv}}
			depots := &mockDepotManager{depots: map[string]depot.Depot{charID: dp}}
			// Slot 72 of 福袋 is item-125: the bag itself is the reward.
			svc, err := NewService(newMockHomeRepo(chars), &mockCharReader{chars: chars},
				WithCharacterUpdater(&mockCharUpdater{chars: chars}),
				WithInventoryManager(invs), WithDepotManager(depots), WithItemCatalog(cat),
				WithRNG(&scriptedRNG{vals: []int{0, 71}}))
			if err != nil {
				t.Fatal(err)
			}
			res, err := svc.UseHomeItem(ctx, charID, "inst-bag", source)
			if err != nil {
				t.Fatal(err)
			}
			if !res.Consumed || res.Message != "なんと、福袋の中身は 福袋 だった！" {
				t.Fatalf("unexpected result %+v", res)
			}
			got := depots.depots[charID]
			if len(got.Items) != got.Capacity+1 {
				t.Fatalf("depot slots = %d, want capacity+1 (%d)", len(got.Items), got.Capacity+1)
			}
			reward := got.Items[len(got.Items)-1]
			if reward.DefinitionID != "item-125" || reward.Quantity != 1 || reward.ID == "inst-bag" {
				t.Fatalf("reward must be a separate item-125 instance, got %+v", reward)
			}
			var remaining coreitem.Instance
			if source == "inventory" {
				left := invs.invs[charID]
				remaining, _ = left.Find("inst-bag")
			} else {
				remaining = got.Items[0]
			}
			if remaining.ID != "inst-bag" || remaining.Quantity != 1 {
				t.Fatalf("exactly one bag must be consumed, remaining %+v", remaining)
			}
		})
	}
}
