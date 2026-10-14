package skill

import (
	"errors"
	"fmt"
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

// Source101/102's broken CODE entries stay absent from certified definitions.
// Adding a lifetime/end-processing representation must not enable those slots.
func TestDeferredGemsRejectWholeCustomEntry(t *testing.T) {
	definitions := BattleDefinitions{Gems: map[string]corebattle.GemEffect{
		"supported": {Kind: corebattle.ActionKindAttack, Power: 10, TargetScope: corebattle.TargetScopeSingleEnemy},
	}}
	for _, unsupported := range []string{"gem-101", "gem-102"} {
		for _, slots := range [][3]string{{"supported", unsupported, ""}, {unsupported, "supported", ""}} {
			action, err := definitions.CustomAction("custom", "Custom", "", 5, slots)
			if !errors.Is(err, corebattle.ErrUnsupportedEffect) || !reflect.DeepEqual(action, corebattle.ActionCustomSkill{}) {
				t.Fatalf("%v: %+v %v", slots, action, err)
			}
		}
	}
}

func TestSharedHelpersDoNotCertifyOuterCatalogEntries(t *testing.T) {
	definitions := BattleDefinitions{Gems: map[string]corebattle.GemEffect{
		"supported": {Kind: corebattle.ActionKindAttack, Power: 10},
	}}
	for _, sourceID := range []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 26, 28, 29, 30, 31, 32, 33, 72, 73, 93, 94, 95, 96} {
		id := fmt.Sprintf("gem-%03d", sourceID)
		for _, slots := range [][3]string{{"supported", id, ""}, {id, "supported", ""}} {
			action, err := definitions.CustomAction("custom", "Recovery", "", 5, slots)
			if !errors.Is(err, corebattle.ErrUnsupportedEffect) || !reflect.DeepEqual(action, corebattle.ActionCustomSkill{}) {
				t.Fatal(sourceID, slots, action, err)
			}
		}
	}
}
