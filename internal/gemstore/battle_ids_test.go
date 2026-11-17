package gemstore_test

import (
	"testing"

	"github.com/witchcraze/party2re/internal/core/skill"
	"github.com/witchcraze/party2re/internal/gemstore"
)

// Registration and saved custom slots share catalog IDs. Runtime context is
// intentionally absent: this checks identity bindings, not callback admission.
func TestCatalogGemsHaveBattleCallbacks(t *testing.T) {
	catalog, err := gemstore.DefaultCatalog()
	if err != nil {
		t.Fatal(err)
	}
	var d skill.BattleDefinitions
	d.RegisterDamageGems(nil)
	d.RegisterRecoveryStatGems(nil)
	d.RegisterDefenseGems()
	d.RegisterRevivals(nil)
	d.RegisterResources(nil)
	d.RegisterStatuses(nil, nil)
	d.RegisterNonJobInvocations()
	d.RegisterSummonAppearanceGems(nil, nil)
	for _, gem := range catalog.AllGems() {
		def, ok := d.Invocations.Entries[gem.ID]
		if !ok || def.Invocation.Kind == "" && (def.Validate == nil || def.Execute == nil) {
			t.Errorf("catalog gem %s has no battle callback", gem.ID)
		}
	}
}
