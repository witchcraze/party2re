package item

import (
	"errors"
	"testing"
)

func TestDefinitionLegacyStorageKey(t *testing.T) {
	tests := []struct {
		id       string
		wantKind int
		wantNo   int
	}{
		{"weapon-01", 1, 1},
		{"weapon-71", 1, 71},
		{"armor-05", 2, 5},
		{"item-001", 3, 1},
		{"item-071", 3, 71}, // shield: legacy stores item-catalog entries as kind 3
		{"item-012", 3, 12}, // accessory
		{"item-268", 3, 268},
	}
	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			kind, no, err := Definition{ID: tt.id}.LegacyStorageKey()
			if err != nil {
				t.Fatalf("LegacyStorageKey() error = %v", err)
			}
			if kind != tt.wantKind || no != tt.wantNo {
				t.Fatalf("LegacyStorageKey() = (%d, %d), want (%d, %d)", kind, no, tt.wantKind, tt.wantNo)
			}
		})
	}
}

func TestDefinitionLegacyStorageKeyRejectsNonCanonicalIDs(t *testing.T) {
	for _, id := range []string{"", "wea-01", "arm-01", "herb", "weapon-", "weapon-x", "item--1", "item-+1", "armor-1a"} {
		t.Run(id, func(t *testing.T) {
			if _, _, err := (Definition{ID: id}).LegacyStorageKey(); !errors.Is(err, ErrNoLegacyStorageKey) {
				t.Fatalf("LegacyStorageKey(%q) error = %v, want ErrNoLegacyStorageKey", id, err)
			}
		})
	}
}

func TestEveryDefaultCatalogDefinitionHasLegacyStorageKey(t *testing.T) {
	catalog, err := DefaultCatalog()
	if err != nil {
		t.Fatal(err)
	}
	for _, def := range catalog.Definitions() {
		if _, _, err := def.LegacyStorageKey(); err != nil {
			t.Errorf("%s: %v", def.ID, err)
		}
	}
}

func TestDefinitionIsToolAndIsToolID(t *testing.T) {
	tests := []struct {
		id       string
		wantTool bool
	}{
		{"item-001", true},
		{"item-012", true}, // accessory
		{"item-071", true}, // shield
		{"item-268", true},
		{"weapon-01", false},
		{"weapon-71", false},
		{"armor-01", false},
		{"armor-55", false},
		{"", false},
		{"unknown", false},
		{"wea-01", false},
	}
	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			if got := IsToolID(tt.id); got != tt.wantTool {
				t.Errorf("IsToolID(%q) = %v, want %v", tt.id, got, tt.wantTool)
			}
			def := Definition{ID: tt.id}
			if got := def.IsTool(); got != tt.wantTool {
				t.Errorf("Definition{%q}.IsTool() = %v, want %v", tt.id, got, tt.wantTool)
			}
		})
	}
}
