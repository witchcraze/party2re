package job

import (
	"errors"
	"fmt"
	"testing"
)

func TestInitialCatalogProvidesGenericDefinitionsAndReferenceGrowthRates(t *testing.T) {
	catalog, err := InitialCatalog()
	if err != nil {
		t.Fatal(err)
	}
	value, err := catalog.FindByID("job-01")
	if err != nil {
		t.Fatal(err)
	}
	if value.Name != "戦士" || value.HPGrowth != 6 || value.MPGrowth != 1 ||
		value.AttackGrowth != 3 || value.DefenseGrowth != 5 || value.AgilityGrowth != 2 {
		t.Fatalf("definition = %#v", value)
	}
}

func TestInitialCatalogUsesReviewedGenericNames(t *testing.T) {
	catalog, err := InitialCatalog()
	if err != nil {
		t.Fatal(err)
	}
	for id, want := range map[string]string{
		"job-40": "鋼のさすらい人", "job-44": "駆鳥使い", "job-45": "森の小人",
		"job-49": "若芽剣士", "job-68": "小玉戦士", "job-69": "小型勇士",
		"job-70": "空竜の民", "job-71": "駆鳥騎手", "job-73": "自在士",
		"job-76": "蓮華術師", "job-79": "夜翼族",
	} {
		value, err := catalog.FindByID(id)
		if err != nil {
			t.Fatal(err)
		}
		if value.Name != want {
			t.Fatalf("%s name = %q, want %q", id, value.Name, want)
		}
	}
}

func TestInitialCatalogContainsAllReferenceJobSlots(t *testing.T) {
	catalog, err := InitialCatalog()
	if err != nil {
		t.Fatal(err)
	}
	for index := 0; index <= 87; index++ {
		id := "starter"
		if index > 0 {
			id = fmt.Sprintf("job-%02d", index)
		}
		if _, err := catalog.FindByID(id); err != nil {
			t.Fatalf("missing definition %s: %v", id, err)
		}
	}
}

func TestCatalogRejectsDuplicatesInvalidDefinitionsAndUnknownIDs(t *testing.T) {
	definition, _ := NewDefinition("starter", "Starter", 0, 0, 0, 0, 0, 1, "")
	if _, err := NewCatalog([]Definition{definition, definition}); !errors.Is(err, ErrInvalidDefinition) {
		t.Fatalf("duplicate error = %v, want %v", err, ErrInvalidDefinition)
	}
	if _, err := NewCatalog([]Definition{{ID: "broken", Name: "Broken", HPGrowth: -1, MinLevel: 1}}); !errors.Is(err, ErrInvalidDefinition) {
		t.Fatalf("invalid error = %v, want %v", err, ErrInvalidDefinition)
	}
	catalog, err := NewCatalog([]Definition{definition})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := catalog.FindByID("missing"); !errors.Is(err, ErrDefinitionNotFound) {
		t.Fatalf("unknown error = %v, want %v", err, ErrDefinitionNotFound)
	}
}
