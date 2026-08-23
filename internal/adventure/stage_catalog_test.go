package adventure_test

import (
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/adventure"
)

func TestStageCreation(t *testing.T) {
	st, err := adventure.NewStage("s-1", "Meadow", 1, []string{"m-1"}, time.Hour)
	if err != nil {
		t.Fatalf("NewStage() error = %v", err)
	}
	if st.ID != "s-1" || st.Name != "Meadow" || st.MinLevel != 1 || st.Duration != time.Hour {
		t.Errorf("NewStage() = %#v", st)
	}
}

func TestStageCreationValidation(t *testing.T) {
	tests := []struct {
		name      string
		id        string
		stageName string
		minLevel  int
		monsters  []string
		duration  time.Duration
	}{
		{name: "empty id", id: "", stageName: "Meadow", minLevel: 1, monsters: []string{"m-1"}, duration: time.Hour},
		{name: "empty name", id: "s-1", stageName: "", minLevel: 1, monsters: []string{"m-1"}, duration: time.Hour},
		{name: "non-positive minLevel", id: "s-1", stageName: "Meadow", minLevel: 0, monsters: []string{"m-1"}, duration: time.Hour},
		{name: "empty monsters", id: "s-1", stageName: "Meadow", minLevel: 1, monsters: []string{}, duration: time.Hour},
		{name: "nil monsters", id: "s-1", stageName: "Meadow", minLevel: 1, monsters: nil, duration: time.Hour},
		{name: "non-positive duration", id: "s-1", stageName: "Meadow", minLevel: 1, monsters: []string{"m-1"}, duration: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := adventure.NewStage(tt.id, tt.stageName, tt.minLevel, tt.monsters, tt.duration)
			if err == nil {
				t.Errorf("NewStage(%s) expected error, got nil", tt.name)
			}
		})
	}
}

func TestStageCatalogOperations(t *testing.T) {
	s1, _ := adventure.NewStage("s-1", "Meadow", 1, []string{"m-1"}, time.Hour)
	s2, _ := adventure.NewStage("s-2", "Cave", 5, []string{"m-2"}, time.Hour)

	cat, err := adventure.NewStageCatalog([]adventure.Stage{s1, s2})
	if err != nil {
		t.Fatalf("NewStageCatalog() error = %v", err)
	}

	found, err := cat.FindByID("s-1")
	if err != nil {
		t.Fatalf("FindByID(s-1) error = %v", err)
	}
	if found.Name != "Meadow" {
		t.Errorf("FindByID(s-1).Name = %s, want Meadow", found.Name)
	}

	if _, err := cat.FindByID("nonexistent"); err == nil {
		t.Error("FindByID(nonexistent) expected error, got nil")
	}

	stages := cat.Stages()
	if len(stages) != 2 {
		t.Errorf("Stages() count = %d, want 2", len(stages))
	}
}

func TestInitialStageCatalogValid(t *testing.T) {
	cat, err := adventure.InitialStageCatalog()
	if err != nil {
		t.Fatalf("InitialStageCatalog() error = %v", err)
	}

	stages := cat.Stages()
	if len(stages) == 0 {
		t.Fatal("InitialStageCatalog() returned empty list")
	}

	monsterCat, err := adventure.InitialMonsterCatalog()
	if err != nil {
		t.Fatalf("InitialMonsterCatalog() error = %v", err)
	}

	seenIDs := make(map[string]bool)
	for _, s := range stages {
		if seenIDs[s.ID] {
			t.Errorf("duplicate stage ID: %s", s.ID)
		}
		seenIDs[s.ID] = true

		if s.ID == "" {
			t.Errorf("stage has empty ID")
		}
		if s.Name == "" {
			t.Errorf("stage %s has empty Name", s.ID)
		}
		if s.MinLevel < 1 {
			t.Errorf("stage %s has invalid MinLevel: %d", s.ID, s.MinLevel)
		}
		if s.Duration <= 0 {
			t.Errorf("stage %s has invalid Duration: %v", s.ID, s.Duration)
		}
		if len(s.MonsterIDs) == 0 {
			t.Errorf("stage %s has no monsters", s.ID)
		}

		for _, monsterID := range s.MonsterIDs {
			if _, err := monsterCat.FindByID(monsterID); err != nil {
				t.Errorf("stage %s references unknown monster %s: %v", s.ID, monsterID, err)
			}
		}
	}
}
