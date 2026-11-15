package battle

import (
	"reflect"
	"testing"
)

func TestSetHeldToolPreservesMissingFactsAndOtherIdentities(t *testing.T) {
	for _, known := range []bool{false, true} {
		a := LifetimeActor{Participant: Participant{ItemDefinitionIDs: []string{"weapon-01", "armor-01", "item-169"}}}
		previous := "item-169"
		if known {
			a.HeldToolID = &previous
		} else {
			a.Gear = &BattleGearStats{AccessoryID: previous}
		}
		original := a.ItemDefinitionIDs
		a.setHeldTool("item-175")
		if !reflect.DeepEqual(a.ItemDefinitionIDs, []string{"weapon-01", "armor-01", "item-175"}) ||
			!reflect.DeepEqual(original, []string{"weapon-01", "armor-01", "item-169"}) || *a.HeldToolID != "item-175" {
			t.Fatal("hand identity or input slice", a, original)
		}
		if known && (a.Gear != nil || previous != "item-175") {
			t.Fatal("fabricated reset facts or detached known hand", a, previous)
		}
		if !known && a.Gear.AccessoryID != "item-175" {
			t.Fatal("stale accessory", a.Gear)
		}
	}
}
