package skill

import (
	"errors"
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
