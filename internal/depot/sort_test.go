package depot

import (
	"context"
	"errors"
	"reflect"
	"testing"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/item"
)

func newSortFixture(t *testing.T, items []item.Instance, opts ...Option) (*Service, *memoryDepotRepo, string) {
	t.Helper()
	depotRepo := newMemoryDepotRepo()
	charRepo := newMemoryCharRepo()
	char, _ := corecharacter.New("Sorter")
	charRepo.characters[char.ID] = char
	service, err := NewService(depotRepo, charRepo, newMemoryInvRepo(), opts...)
	if err != nil {
		t.Fatal(err)
	}
	dep, _ := NewDepot(char.ID)
	dep.Capacity = len(items) + 1
	dep.Items = append([]item.Instance(nil), items...)
	if err := depotRepo.Save(context.Background(), dep); err != nil {
		t.Fatal(err)
	}
	return service, depotRepo, char.ID
}

func defaultCatalogOption(t *testing.T) Option {
	t.Helper()
	catalog, err := item.DefaultCatalog()
	if err != nil {
		t.Fatal(err)
	}
	return WithItemDefinitionProvider(catalog)
}

func definitionIDs(items []item.Instance) []string {
	ids := make([]string, len(items))
	for i, inst := range items {
		ids[i] = inst.DefinitionID
	}
	return ids
}

// Legacy depot.cgi seiton sorts the stored kind, then the numeric item number.
// Shields and accessories are item-catalog entries (kind 3), not armor.
func TestSortItemsUsesLegacyStoredKindAndNumber(t *testing.T) {
	ids := []string{"item-071", "item-012", "item-001", "armor-09", "armor-05", "weapon-10", "weapon-02"}
	items := make([]item.Instance, len(ids))
	for i, id := range ids {
		items[i] = item.Instance{ID: "inst-" + id, DefinitionID: id, Quantity: 1}
	}
	service, _, characterID := newSortFixture(t, items, defaultCatalogOption(t))

	got, err := service.SortItems(context.Background(), characterID)
	if err != nil {
		t.Fatalf("SortItems error: %v", err)
	}
	want := []string{"weapon-02", "weapon-10", "armor-05", "armor-09", "item-001", "item-012", "item-071"}
	if !reflect.DeepEqual(definitionIDs(got.Items), want) {
		t.Fatalf("order = %v, want %v", definitionIDs(got.Items), want)
	}
}

func TestSortItemsKeepsEqualKeyOrderAndInstanceFacts(t *testing.T) {
	items := []item.Instance{
		{ID: "inst-b", DefinitionID: "weapon-01", Quantity: 1, EnhancementLevel: 6},
		{ID: "inst-herb", DefinitionID: "item-001", Quantity: 8},
		{ID: "inst-a", DefinitionID: "weapon-01", Quantity: 1},
		{ID: "inst-c", DefinitionID: "weapon-01", Quantity: 1, EnhancementLevel: 2},
	}
	service, repo, characterID := newSortFixture(t, items, defaultCatalogOption(t))

	got, err := service.SortItems(context.Background(), characterID)
	if err != nil {
		t.Fatalf("SortItems error: %v", err)
	}
	want := []item.Instance{items[0], items[2], items[3], items[1]}
	if !reflect.DeepEqual(got.Items, want) {
		t.Fatalf("items = %+v, want %+v", got.Items, want)
	}
	stored, _ := repo.FindByCharacterID(context.Background(), characterID)
	if !reflect.DeepEqual(stored.Items, want) {
		t.Fatalf("persisted items = %+v, want %+v", stored.Items, want)
	}
}

func TestSortItemsFailsWithoutResolvableKeyAndLeavesDepotUnchanged(t *testing.T) {
	memoryCatalog := newMemoryItemCatalog() // defines non-canonical wea-01 / arm-01
	tests := []struct {
		name    string
		options func(t *testing.T) []Option
		ids     []string
		wantErr error
	}{
		{"unknown definition", func(t *testing.T) []Option { return []Option{defaultCatalogOption(t)} },
			[]string{"item-001", "weapon-01", "ghost-1"}, item.ErrDefinitionNotFound},
		{"non-canonical identity", func(*testing.T) []Option { return []Option{WithItemDefinitionProvider(memoryCatalog)} },
			[]string{"item-001", "wea-01"}, item.ErrNoLegacyStorageKey},
		{"no definition provider", func(*testing.T) []Option { return nil },
			[]string{"item-001", "weapon-01"}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			items := make([]item.Instance, len(tt.ids))
			for i, id := range tt.ids {
				items[i] = item.Instance{ID: "inst-" + id, DefinitionID: id, Quantity: 1}
			}
			service, repo, characterID := newSortFixture(t, items, tt.options(t)...)

			_, err := service.SortItems(context.Background(), characterID)
			if err == nil || (tt.wantErr != nil && !errors.Is(err, tt.wantErr)) {
				t.Fatalf("SortItems error = %v, want %v", err, tt.wantErr)
			}
			stored, _ := repo.FindByCharacterID(context.Background(), characterID)
			if !reflect.DeepEqual(stored.Items, items) {
				t.Fatalf("depot changed on failure: %+v", stored.Items)
			}
		})
	}
}

func TestSortItemsEmptyDepotNeedsNoDefinitions(t *testing.T) {
	service, _, characterID := newSortFixture(t, nil)
	got, err := service.SortItems(context.Background(), characterID)
	if err != nil || len(got.Items) != 0 {
		t.Fatalf("empty sort = %+v, %v", got.Items, err)
	}
}
