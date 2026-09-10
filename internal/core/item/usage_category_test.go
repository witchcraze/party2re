package item

import (
	"errors"
	"fmt"
	"testing"
)

func TestUsageCategoryParityWithLegacyCGI(t *testing.T) {
	if len(LegacyItemCatalog) != 269 {
		t.Fatalf("expected 269 legacy item entries, got %d", len(LegacyItemCatalog))
	}

	catalog, err := InitialCatalog()
	if err != nil {
		t.Fatalf("InitialCatalog failed: %v", err)
	}

	categoryCounts := make(map[UsageCategory]int)

	for _, ref := range LegacyItemCatalog {
		categoryCounts[ref.UsageCategory]++

		if ref.No == 0 {
			if ref.LegacyName != "なし" || ref.Price != 0 || ref.UsageCategory != UsageCategoryNone {
				t.Errorf("item 0 invalid: %#v", ref)
			}
			continue
		}

		itemID := fmt.Sprintf("item-%03d", ref.No)
		def, err := catalog.FindByID(itemID)
		if err != nil {
			t.Errorf("item %s (%s) not found in InitialCatalog: %v", itemID, ref.CleanRoomName, err)
			continue
		}

		if def.Name != ref.CleanRoomName {
			t.Errorf("[%s] name mismatch: got %q, want clean-room %q (legacy: %q)",
				itemID, def.Name, ref.CleanRoomName, ref.LegacyName)
		}

		if def.Price != ref.Price {
			t.Errorf("[%s] price mismatch: got %d, want %d", itemID, def.Price, ref.Price)
		}

		if def.UsageCategory != ref.UsageCategory {
			t.Errorf("[%s] usage category mismatch: got %v (%s), want %v (%s)",
				itemID, def.UsageCategory, def.UsageCategory, ref.UsageCategory, ref.UsageCategory)
		}

		// Verify semantic predicates match category
		switch ref.UsageCategory {
		case UsageCategoryCombatOnly:
			if !def.IsCombatOnly() {
				t.Errorf("[%s] expected IsCombatOnly() to be true for Category 1", itemID)
			}
			if def.IsUsableAtHome() {
				t.Errorf("[%s] expected IsUsableAtHome() to be false for Category 1", itemID)
			}
		case UsageCategoryAnytime:
			if def.IsCombatOnly() {
				t.Errorf("[%s] expected IsCombatOnly() to be false for Category 2", itemID)
			}
			if !def.IsUsableAtHome() {
				t.Errorf("[%s] expected IsUsableAtHome() to be true for Category 2", itemID)
			}
		case UsageCategoryDepotAfterAction:
			if def.IsCombatOnly() {
				t.Errorf("[%s] expected IsCombatOnly() to be false for Category 4", itemID)
			}
			if !def.IsUsableAtHome() {
				t.Errorf("[%s] expected IsUsableAtHome() to be true for Category 4", itemID)
			}
		case UsageCategoryNone, UsageCategoryCombatPassive:
			if def.IsCombatOnly() {
				t.Errorf("[%s] expected IsCombatOnly() to be false for Category %v", itemID, ref.UsageCategory)
			}
			if def.IsUsableAtHome() {
				t.Errorf("[%s] expected IsUsableAtHome() to be false for Category %v", itemID, ref.UsageCategory)
			}
		}
	}

	// Verify exact category breakdown matching legacy $ites table
	expectedCounts := map[UsageCategory]int{
		UsageCategoryNone:             43,
		UsageCategoryCombatOnly:       53,
		UsageCategoryAnytime:          53,
		UsageCategoryCombatPassive:    117,
		UsageCategoryDepotAfterAction: 3,
	}

	for cat, want := range expectedCounts {
		if got := categoryCounts[cat]; got != want {
			t.Errorf("category %s (%d) count mismatch: got %d, want %d", cat, cat, got, want)
		}
	}
}

func TestUsageCategoryStringAndValidation(t *testing.T) {
	tests := []struct {
		cat      UsageCategory
		wantStr  string
		wantHome bool
		wantAtk  bool
		valid    bool
	}{
		{UsageCategoryNone, "none", false, false, true},
		{UsageCategoryCombatOnly, "combat_only", false, true, true},
		{UsageCategoryAnytime, "anytime", true, false, true},
		{UsageCategoryCombatPassive, "combat_passive", false, false, true},
		{UsageCategoryDepotAfterAction, "depot_after_action", true, false, true},
		{UsageCategory(99), "UsageCategory(99)", false, false, false},
		{UsageCategory(-1), "UsageCategory(-1)", false, false, false},
	}

	for _, tt := range tests {
		t.Run(tt.wantStr, func(t *testing.T) {
			if got := tt.cat.String(); got != tt.wantStr {
				t.Errorf("String() = %q, want %q", got, tt.wantStr)
			}
			if got := tt.cat.IsUsableAtHome(); got != tt.wantHome {
				t.Errorf("IsUsableAtHome() = %v, want %v", got, tt.wantHome)
			}
			if got := tt.cat.IsCombatOnly(); got != tt.wantAtk {
				t.Errorf("IsCombatOnly() = %v, want %v", got, tt.wantAtk)
			}
			if got := IsValidUsageCategory(tt.cat); got != tt.valid {
				t.Errorf("IsValidUsageCategory() = %v, want %v", got, tt.valid)
			}
		})
	}
}

func TestNewConsumableDefinition(t *testing.T) {
	def, err := NewConsumableDefinition("item-herb", "薬草", 30, UsageCategoryCombatOnly)
	if err != nil {
		t.Fatalf("NewConsumableDefinition failed: %v", err)
	}
	if def.ID != "item-herb" || def.Name != "薬草" || def.Price != 30 || def.UsageCategory != UsageCategoryCombatOnly {
		t.Errorf("unexpected def: %#v", def)
	}
	if !def.IsCombatOnly() || def.IsUsableAtHome() {
		t.Errorf("predicate mismatch for combat only def: %#v", def)
	}

	// Rejects invalid usage category
	if _, err := NewConsumableDefinition("item-bad", "Bad", 10, UsageCategory(99)); !errors.Is(err, ErrInvalidDefinition) {
		t.Errorf("expected ErrInvalidDefinition for invalid category, got %v", err)
	}
}

func TestNewCatalogRejectsInvalidUsageCategory(t *testing.T) {
	badDef := Definition{
		ID:            "bad-cat",
		Name:          "Bad Category",
		Price:         10,
		UsageCategory: UsageCategory(99),
	}
	if _, err := NewCatalog([]Definition{badDef}); !errors.Is(err, ErrInvalidDefinition) {
		t.Errorf("expected ErrInvalidDefinition for catalog with invalid UsageCategory, got %v", err)
	}
}
