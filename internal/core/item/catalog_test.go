package item

import (
	"errors"
	"strings"
	"testing"
)

func TestInitialCatalogLoadsValidDefinitions(t *testing.T) {
	catalog, err := InitialCatalog()
	if err != nil {
		t.Fatalf("InitialCatalog(): %v", err)
	}

	definitions := catalog.Definitions()
	if len(definitions) != 394 {
		t.Fatalf("expected 394 item definitions, got %d", len(definitions))
	}

	slotCounts := make(map[Slot]int)
	for _, d := range definitions {
		slotCounts[d.Slot]++
	}
	if slotCounts[SlotMainHand] != 71 {
		t.Fatalf("main-hand count = %d, want 71", slotCounts[SlotMainHand])
	}
	if slotCounts[SlotBody] != 55 {
		t.Fatalf("body count = %d, want 55", slotCounts[SlotBody])
	}
	if slotCounts[SlotOffHand] != 5 {
		t.Fatalf("off-hand count = %d, want 5", slotCounts[SlotOffHand])
	}
	if slotCounts[SlotAccessory] != 36 {
		t.Fatalf("accessory count = %d, want 36", slotCounts[SlotAccessory])
	}
	if slotCounts[SlotNone] != 227 {
		t.Fatalf("none/consumables count = %d, want 227", slotCounts[SlotNone])
	}

	seen := make(map[string]struct{}, len(definitions))
	for _, definition := range definitions {
		definition := definition
		t.Run(definition.ID, func(t *testing.T) {
			if strings.TrimSpace(definition.ID) == "" || strings.TrimSpace(definition.Name) == "" || definition.Price < 0 {
				t.Fatalf("definition has empty ID, Name, or negative price: %#v", definition)
			}
			if !IsValidSlot(definition.Slot) {
				t.Fatalf("definition %s has invalid slot %q", definition.ID, definition.Slot)
			}
			if _, exists := seen[definition.ID]; exists {
				t.Fatalf("duplicate definition ID %q", definition.ID)
			}
			seen[definition.ID] = struct{}{}

			loaded, err := catalog.FindByID(definition.ID)
			if err != nil {
				t.Fatalf("FindByID(%q): %v", definition.ID, err)
			}
			if loaded != definition {
				t.Fatalf("FindByID(%q) = %#v, want %#v", definition.ID, loaded, definition)
			}

			instance, err := NewInstance(definition.ID, 1)
			if err != nil {
				t.Fatalf("NewInstance(%q): %v", definition.ID, err)
			}
			if instance.DefinitionID != definition.ID || instance.Quantity != 1 {
				t.Fatalf("instance mismatch: %#v", instance)
			}
		})
	}
}

func TestCatalogRejectsDuplicatesInvalidDefinitionsAndUnknownIDs(t *testing.T) {
	valid, err := NewDefinition("potion", "Recovery Potion", 30)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := NewCatalog([]Definition{valid, valid}); !errors.Is(err, ErrInvalidDefinition) {
		t.Fatalf("NewCatalog(duplicate) error = %v, want %v", err, ErrInvalidDefinition)
	}

	invalidDef := Definition{ID: "", Name: "No ID", Price: 10}
	if _, err := NewCatalog([]Definition{invalidDef}); !errors.Is(err, ErrInvalidDefinition) {
		t.Fatalf("NewCatalog(empty ID) error = %v, want %v", err, ErrInvalidDefinition)
	}

	invalidPrice := Definition{ID: "bad-price", Name: "Bad Price", Price: -5}
	if _, err := NewCatalog([]Definition{invalidPrice}); !errors.Is(err, ErrInvalidDefinition) {
		t.Fatalf("NewCatalog(negative price) error = %v, want %v", err, ErrInvalidDefinition)
	}

	invalidSlot := Definition{ID: "bad-slot", Name: "Bad Slot", Price: 10, Slot: Slot("invalid-slot")}
	if _, err := NewCatalog([]Definition{invalidSlot}); !errors.Is(err, ErrInvalidDefinition) {
		t.Fatalf("NewCatalog(invalid slot) error = %v, want %v", err, ErrInvalidDefinition)
	}

	catalog, err := NewCatalog([]Definition{valid})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := catalog.FindByID("unknown"); !errors.Is(err, ErrDefinitionNotFound) {
		t.Fatalf("FindByID(unknown) error = %v, want %v", err, ErrDefinitionNotFound)
	}

	var nilCatalog *Catalog
	if _, err := nilCatalog.FindByID("potion"); !errors.Is(err, ErrDefinitionNotFound) {
		t.Fatalf("nilCatalog.FindByID() error = %v, want %v", err, ErrDefinitionNotFound)
	}
	if defs := nilCatalog.Definitions(); defs != nil {
		t.Fatalf("nilCatalog.Definitions() = %#v, want nil", defs)
	}
}

func TestDefinitionProviderInterface(t *testing.T) {
	valid, err := NewDefinition("herb", "Herb", 10)
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := NewCatalog([]Definition{valid})
	if err != nil {
		t.Fatal(err)
	}

	// Verify Catalog satisfies DefinitionProvider
	var defProvider DefinitionProvider = catalog
	def, err := defProvider.FindByID("herb")
	if err != nil {
		t.Fatalf("DefinitionProvider.FindByID failed: %v", err)
	}
	if def.ID != "herb" {
		t.Errorf("expected def ID 'herb', got %s", def.ID)
	}

	// Verify Catalog satisfies ItemDefinitionProvider alias
	var itemDefProvider ItemDefinitionProvider = catalog
	itemDef, err := itemDefProvider.FindByID("herb")
	if err != nil {
		t.Fatalf("ItemDefinitionProvider.FindByID failed: %v", err)
	}
	if itemDef.ID != "herb" {
		t.Errorf("expected itemDef ID 'herb', got %s", itemDef.ID)
	}
}
