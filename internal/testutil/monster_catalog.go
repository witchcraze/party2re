package testutil

import (
	"testing"

	"github.com/witchcraze/party2re/internal/adventure"
)

// MonsterNameLookup supplies native definitions to tests of consumers outside Adventure.
func MonsterNameLookup(t *testing.T) func(string) (string, bool) {
	t.Helper()
	catalog, err := adventure.InitialMonsterCatalog()
	if err != nil {
		t.Fatal(err)
	}
	return func(id string) (string, bool) {
		m, err := catalog.FindByID(id)
		return m.Name, err == nil
	}
}
