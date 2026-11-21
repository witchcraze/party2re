package home_test

import (
	"context"
	"errors"
	"testing"
)

const sacredManaDefinition = "item-203"

// Item 203 credits exactly 200 SP from either source, over-level or not and
// with a full Depot, and consumes one; a failed save rolls back both.
func TestSacredMana_CreditsSPAndConsumes(t *testing.T) {
	f := newBookFixture(t, bookFailures{})
	ctx := context.Background()
	for _, tc := range []struct {
		source    string
		sp        int
		overLevel bool
	}{{"inventory", 0, false}, {"inventory", 37, true}, {"depot", 0, true}, {"depot", 37, false}} {
		c := f.character(t, "m")
		if _, err := f.db.Exec(`UPDATE characters SET sp = ?, over_level = ? WHERE id = ?`, tc.sp, tc.overLevel, c.ID); err != nil {
			t.Fatal(err)
		}
		id := f.stockItem(t, c, tc.source, sacredManaDefinition)
		res, err := f.svc.UseHomeItem(ctx, c.ID, id, tc.source)
		if err != nil || !res.Consumed || f.held(t, c, tc.source, id) {
			t.Fatalf("%+v: want one consumed, got %+v %v", tc, res, err)
		}
		got, err := f.charRepo.FindByID(ctx, c.ID)
		if err != nil || got.SP != tc.sp+200 || got.Money != c.Money {
			t.Fatalf("%+v: SP=%d money=%d (was %d) %v", tc, got.SP, got.Money, c.Money, err)
		}
	}

	f.charSaveFail.Store(true)
	for _, source := range []string{"inventory", "depot"} {
		c := f.character(t, "f")
		id := f.stockItem(t, c, source, sacredManaDefinition)
		if _, err := f.svc.UseHomeItem(ctx, c.ID, id, source); !errors.Is(err, errInjectedBook) {
			t.Fatalf("%s: want injected failure, got %v", source, err)
		}
		got, err := f.charRepo.FindByID(ctx, c.ID)
		if err != nil || got.SP != c.SP || !f.held(t, c, source, id) {
			t.Fatalf("%s: partial commit SP=%d held=%v %v", source, got.SP, f.held(t, c, source, id), err)
		}
	}
}
