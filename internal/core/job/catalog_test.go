package job

import (
	"errors"
	"fmt"
	"strings"
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

func TestInitialCatalogValidatesAndExercisesEveryDefinition(t *testing.T) {
	catalog, err := InitialCatalog()
	if err != nil {
		t.Fatal(err)
	}

	definitions := catalog.Definitions()
	if len(definitions) != 88 {
		t.Fatalf("definition count = %d, want 88", len(definitions))
	}

	seen := make(map[string]struct{}, len(definitions))
	for _, definition := range definitions {
		definition := definition
		t.Run(definition.ID, func(t *testing.T) {
			if strings.TrimSpace(definition.ID) == "" || strings.TrimSpace(definition.Name) == "" {
				t.Fatal("definition must have an ID and name")
			}
			if definition.HPGrowth < 0 || definition.MPGrowth < 0 ||
				definition.AttackGrowth < 0 || definition.DefenseGrowth < 0 ||
				definition.AgilityGrowth < 0 || definition.MinLevel < 1 {
				t.Fatalf("invalid definition = %#v", definition)
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

			state, err := NewCharacterJob("character-1", "starter")
			if err != nil {
				t.Fatal(err)
			}
			if definition.ID == "starter" {
				if err := state.ChangeTo(definition, definition.MinLevel, "unspecified"); !errors.Is(err, ErrJobUnavailable) {
					t.Fatalf("starter ChangeTo() error = %v, want %v", err, ErrJobUnavailable)
				}
				return
			}
			if err := state.ChangeTo(definition, definition.MinLevel, definition.RequiredGender); err != nil {
				t.Fatalf("ChangeTo() at minimum level: %v", err)
			}
		})
	}
}

func TestInitialCatalogExercisesMinimumLevelBoundaryForEveryJob(t *testing.T) {
	catalog, err := InitialCatalog()
	if err != nil {
		t.Fatal(err)
	}
	for _, definition := range catalog.Definitions() {
		if definition.ID == "starter" {
			continue
		}
		definition := definition
		t.Run(definition.ID, func(t *testing.T) {
			state, err := NewCharacterJob("character-1", "starter")
			if err != nil {
				t.Fatal(err)
			}
			if err := state.ChangeTo(definition, definition.MinLevel-1, definition.RequiredGender); !errors.Is(err, ErrJobUnavailable) {
				t.Fatalf("ChangeTo() below minimum level error = %v, want %v", err, ErrJobUnavailable)
			}
		})
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
