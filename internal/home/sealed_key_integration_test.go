package home_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/testutil"
)

const (
	sealedKey = "item-259"
	sealedBox = "item-258"
)

func (f *bookFixture) depotItems(t *testing.T, c corecharacter.Character) []coreitem.Instance {
	t.Helper()
	dp, err := f.depots.FindByCharacterID(context.Background(), c.ID)
	if err != nil {
		t.Fatal(err)
	}
	return dp.Items
}

// newRewards returns Depot items absent from before, checking the box prize.
func newRewards(t *testing.T, before, after []coreitem.Instance) []coreitem.Instance {
	t.Helper()
	var out []coreitem.Instance
	for _, it := range after {
		if !slices.ContainsFunc(before, func(b coreitem.Instance) bool { return b.ID == it.ID }) {
			out = append(out, it)
		}
	}
	if len(out) != 0 && (len(out) < 3 || len(out) > 5 || out[0].DefinitionID != "item-059") {
		t.Fatalf("want item-059 then 2..4 rewards, got %+v", out)
	}
	return out
}

func TestSealedKey_SourcesAndBoxes(t *testing.T) {
	f := newBookFixture(t, bookFailures{})
	ctx := context.Background()
	for _, tc := range []struct {
		name, keySource, boxSource string // boxSource "" means no box
		wantConsumed, wantOpened   bool
	}{
		{"held key, depot box", "inventory", "depot", true, true},
		{"held key, no box keeps key", "inventory", "", false, false},
		{"depot key, held box", "depot", "inventory", true, true},
		{"depot key, depot box", "depot", "depot", true, true},
		{"depot key, no box still consumes", "depot", "", true, false},
	} {
		c := f.character(t, "m")
		key := f.stockItemWithID(t, c, tc.keySource, "key1-"+c.ID[:8], sealedKey)
		f.stockItemWithID(t, c, "depot", "key2-"+c.ID[:8], sealedKey) // a second key stays
		var boxes []string
		if tc.boxSource != "" {
			boxes = append(boxes, f.stockItemWithID(t, c, tc.boxSource, "box1-"+c.ID[:8], sealedBox))
		}
		boxes = append(boxes, f.stockItemWithID(t, c, "depot", "box2-"+c.ID[:8], "item-018")) // not a box
		if tc.boxSource == "depot" {
			boxes = append(boxes, f.stockItemWithID(t, c, "depot", "box3-"+c.ID[:8], sealedBox)) // a second box stays
		}
		before := f.depotItems(t, c)

		res, err := f.svc.UseHomeItem(ctx, c.ID, key, tc.keySource)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if res.Consumed != tc.wantConsumed || f.held(t, c, tc.keySource, key) == tc.wantConsumed || !f.held(t, c, "depot", "key2-"+c.ID[:8]) {
			t.Fatalf("%s: consumed=%v %+v", tc.name, res.Consumed, res)
		}
		rewards := newRewards(t, before, f.depotItems(t, c))
		if tc.wantOpened != (len(rewards) > 0) {
			t.Fatalf("%s: rewards %+v", tc.name, rewards)
		}
		if !tc.wantOpened && res.Message != "何かの箱を開けるための鍵みたいだ" {
			t.Fatalf("%s: message %q", tc.name, res.Message)
		}
		if tc.boxSource != "" && f.held(t, c, tc.boxSource, boxes[0]) {
			t.Fatalf("%s: box not removed", tc.name)
		}
		for _, other := range boxes[1:] {
			if !f.held(t, c, "depot", other) {
				t.Fatalf("%s: extra item %s removed", tc.name, other)
			}
		}
	}
}

func TestSealedKey_SaveFailureRollsBack(t *testing.T) {
	f := newBookFixture(t, bookFailures{characterSave: true})
	ctx := context.Background()
	for _, keySource := range []string{"inventory", "depot"} {
		c := f.character(t, "f")
		key := f.stockItemWithID(t, c, keySource, "key-"+c.ID[:8], sealedKey)
		box := f.stockItemWithID(t, c, "depot", "box-"+c.ID[:8], sealedBox)
		before := f.depotItems(t, c)
		if _, err := f.svc.UseHomeItem(ctx, c.ID, key, keySource); !errors.Is(err, errInjectedBook) {
			t.Fatalf("%s: want injected failure, got %v", keySource, err)
		}
		if !f.held(t, c, keySource, key) || !f.held(t, c, "depot", box) || len(f.depotItems(t, c)) != len(before) {
			t.Fatalf("%s: partial commit", keySource)
		}
	}
}

// Depot keys racing for fewer boxes open each box once; every key is consumed
// (D4) and the Depot gains exactly one reward set per box.
func TestSealedKey_ConcurrentUsesOpenEachBoxOnce(t *testing.T) {
	f := newBookFixture(t, bookFailures{})
	ctx := context.Background()
	c := f.character(t, "m")
	workers, boxes := testutil.GetStressConfig().Workers, 3
	keys := make([]string, workers)
	for i := range keys {
		keys[i] = f.stockItemWithID(t, c, "depot", fmt.Sprintf("k%02d-%s", i, c.ID[:8]), sealedKey)
	}
	for i := range boxes {
		f.stockItemWithID(t, c, "depot", fmt.Sprintf("b%02d-%s", i, c.ID[:8]), sealedBox)
	}
	before := f.depotItems(t, c)

	uses := make([]func() error, workers)
	for i, k := range keys {
		uses[i] = func() error {
			res, err := f.svc.UseHomeItem(ctx, c.ID, k, "depot")
			if err == nil && !res.Consumed {
				return fmt.Errorf("key %s kept", k)
			}
			return err
		}
	}
	for i, err := range testutil.RunRace(uses...) {
		if err != nil {
			t.Fatalf("use %d: %v", i, err)
		}
	}
	after := f.depotItems(t, c)
	var added, prizes int
	for _, it := range after {
		if !slices.ContainsFunc(before, func(b coreitem.Instance) bool { return b.ID == it.ID }) {
			added++
			if it.DefinitionID == "item-059" {
				prizes++
			}
		}
		if it.DefinitionID == sealedKey || it.DefinitionID == sealedBox {
			t.Fatalf("left over %s", it.DefinitionID)
		}
	}
	if prizes != boxes || added < 3*boxes || added > 5*boxes || len(after) != len(before)-workers-boxes+added {
		t.Fatalf("not conserved: before=%d after=%d added=%d prizes=%d", len(before), len(after), added, prizes)
	}
}
