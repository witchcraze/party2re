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

func TestValidateUsageLocation(t *testing.T) {
	tests := []struct {
		name     string
		category UsageCategory
		location UsageLocation
		wantErr  error
		combatOK bool
		homeOK   bool
	}{
		{
			name:     "Category 1 CombatOnly allowed in combat",
			category: UsageCategoryCombatOnly,
			location: UsageLocationCombat,
			wantErr:  nil,
			combatOK: true,
			homeOK:   false,
		},
		{
			name:     "Category 1 CombatOnly rejected at home",
			category: UsageCategoryCombatOnly,
			location: UsageLocationHome,
			wantErr:  ErrCannotUseAtHome,
			combatOK: true,
			homeOK:   false,
		},
		{
			name:     "Category 2 Anytime allowed at home",
			category: UsageCategoryAnytime,
			location: UsageLocationHome,
			wantErr:  nil,
			combatOK: false,
			homeOK:   true,
		},
		{
			name:     "Category 2 Anytime rejected in combat",
			category: UsageCategoryAnytime,
			location: UsageLocationCombat,
			wantErr:  ErrCannotUseInCombat,
			combatOK: false,
			homeOK:   true,
		},
		{
			name:     "Category 0 None rejected in combat",
			category: UsageCategoryNone,
			location: UsageLocationCombat,
			wantErr:  ErrCannotUseInCombat,
			combatOK: false,
			homeOK:   false,
		},
		{
			name:     "Category 0 None rejected at home",
			category: UsageCategoryNone,
			location: UsageLocationHome,
			wantErr:  ErrCannotUseAtHome,
			combatOK: false,
			homeOK:   false,
		},
		{
			name:     "Category 3 CombatPassive rejected in combat command",
			category: UsageCategoryCombatPassive,
			location: UsageLocationCombat,
			wantErr:  ErrCannotUseInCombat,
			combatOK: false,
			homeOK:   false,
		},
		{
			name:     "Category 3 CombatPassive rejected at home",
			category: UsageCategoryCombatPassive,
			location: UsageLocationHome,
			wantErr:  ErrCannotUseAtHome,
			combatOK: false,
			homeOK:   false,
		},
		{
			name:     "Category 4 DepotAfterAction allowed at home",
			category: UsageCategoryDepotAfterAction,
			location: UsageLocationHome,
			wantErr:  nil,
			combatOK: false,
			homeOK:   true,
		},
		{
			name:     "Category 4 DepotAfterAction rejected in combat",
			category: UsageCategoryDepotAfterAction,
			location: UsageLocationCombat,
			wantErr:  ErrCannotUseInCombat,
			combatOK: false,
			homeOK:   true,
		},
		{
			name:     "Unknown location returns ErrInvalidUsageLocation",
			category: UsageCategoryCombatOnly,
			location: UsageLocation("nowhere"),
			wantErr:  ErrInvalidUsageLocation,
			combatOK: true,
			homeOK:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.category.IsUsableInCombatCommand(); got != tt.combatOK {
				t.Errorf("IsUsableInCombatCommand() = %v, want %v", got, tt.combatOK)
			}
			err := ValidateUsageLocation(tt.category, tt.location)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("ValidateUsageLocation() error = %v, want %v", err, tt.wantErr)
			}

			def := Definition{ID: "test-item", Name: "Test", UsageCategory: tt.category}
			if got := def.CanUseInCombat(); got != tt.combatOK {
				t.Errorf("def.CanUseInCombat() = %v, want %v", got, tt.combatOK)
			}
			defErr := def.ValidateUsageLocation(tt.location)
			if !errors.Is(defErr, tt.wantErr) {
				t.Errorf("def.ValidateUsageLocation() error = %v, want %v", defErr, tt.wantErr)
			}
		})
	}
}

func TestAll269ItemsCombatUsageMatrix(t *testing.T) {
	catalog, err := InitialCatalog()
	if err != nil {
		t.Fatalf("InitialCatalog failed: %v", err)
	}

	combatAllowedCount := 0
	combatRejectedCount := 0

	for _, ref := range LegacyItemCatalog {
		if ref.No == 0 {
			continue
		}
		itemID := fmt.Sprintf("item-%03d", ref.No)
		def, err := catalog.FindByID(itemID)
		if err != nil {
			t.Fatalf("[%s] item not found: %v", itemID, err)
		}

		if ref.UsageCategory == UsageCategoryCombatOnly {
			// Must be allowed in combat
			if !def.CanUseInCombat() {
				t.Errorf("[%s: %s] expected CanUseInCombat() == true for Category 1", itemID, def.Name)
			}
			if !def.UsageCategory.IsUsableInCombatCommand() {
				t.Errorf("[%s: %s] expected IsUsableInCombatCommand() == true for Category 1", itemID, def.Name)
			}
			if err := def.ValidateUsageLocation(UsageLocationCombat); err != nil {
				t.Errorf("[%s: %s] expected ValidateUsageLocation(Combat) == nil, got %v", itemID, def.Name, err)
			}
			combatAllowedCount++
		} else {
			// Must be rejected in combat command
			if def.CanUseInCombat() {
				t.Errorf("[%s: %s] expected CanUseInCombat() == false for Category %v", itemID, def.Name, ref.UsageCategory)
			}
			if def.UsageCategory.IsUsableInCombatCommand() {
				t.Errorf("[%s: %s] expected IsUsableInCombatCommand() == false for Category %v", itemID, def.Name, ref.UsageCategory)
			}
			if err := def.ValidateUsageLocation(UsageLocationCombat); !errors.Is(err, ErrCannotUseInCombat) {
				t.Errorf("[%s: %s] expected ErrCannotUseInCombat for Category %v, got %v", itemID, def.Name, ref.UsageCategory, err)
			}
			combatRejectedCount++
		}
	}

	if combatAllowedCount != 53 {
		t.Errorf("combatAllowedCount = %d, want 53", combatAllowedCount)
	}
	// 269 items total - 1 (item 0) = 268 items: 53 combat-only, 215 non-combat (43 None, 53 Anytime, 116 CombatPassive, 3 DepotAfterAction)
	if combatRejectedCount != 215 {
		t.Errorf("combatRejectedCount = %d, want 215", combatRejectedCount)
	}
}
