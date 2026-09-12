package adventure_test

import (
	"testing"

	"github.com/witchcraze/party2re/internal/adventure"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

func TestStageCreation(t *testing.T) {
	st, err := adventure.NewStage("s-1", "Meadow", 1, []string{"m-1"})
	if err != nil {
		t.Fatalf("NewStage() error = %v", err)
	}
	if st.ID != "s-1" || st.Name != "Meadow" || st.MinLevel != 1 {
		t.Errorf("NewStage() = %#v", st)
	}
	if len(st.GetBossIDs()) != 1 || st.GetBossIDs()[0] != "m-1" {
		t.Errorf("default boss should be m-1, got %v", st.GetBossIDs())
	}
}

func TestStageCreationValidation(t *testing.T) {
	tests := []struct {
		name      string
		id        string
		stageName string
		minLevel  int
		monsters  []string
	}{
		{name: "empty id", id: "", stageName: "Meadow", minLevel: 1, monsters: []string{"m-1"}},
		{name: "empty name", id: "s-1", stageName: "", minLevel: 1, monsters: []string{"m-1"}},
		{name: "non-positive minLevel", id: "s-1", stageName: "Meadow", minLevel: 0, monsters: []string{"m-1"}},
		{name: "empty monsters", id: "s-1", stageName: "Meadow", minLevel: 1, monsters: []string{}},
		{name: "nil monsters", id: "s-1", stageName: "Meadow", minLevel: 1, monsters: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := adventure.NewStage(tt.id, tt.stageName, tt.minLevel, tt.monsters)
			if err == nil {
				t.Errorf("NewStage(%s) expected error, got nil", tt.name)
			}
		})
	}
}

func TestStageCatalogOperations(t *testing.T) {
	s1, _ := adventure.NewStage("s-1", "Meadow", 1, []string{"m-1", "m-boss"})
	s2, _ := adventure.NewStageWithBosses("s-2", "Cave", 5, []string{"m-2"}, []string{"m-boss2"})

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

func TestStage_BossAndNormalMonsters(t *testing.T) {
	// Without explicit bosses, last monster is boss
	s1, _ := adventure.NewStage("s-1", "Meadow", 1, []string{"m-1", "m-2", "m-boss"})
	bosses := s1.GetBossIDs()
	if len(bosses) != 1 || bosses[0] != "m-boss" {
		t.Fatalf("expected m-boss, got %v", bosses)
	}
	normals := s1.GetNormalMonsterIDs()
	if len(normals) != 2 || normals[0] != "m-1" || normals[1] != "m-2" {
		t.Fatalf("expected [m-1, m-2], got %v", normals)
	}

	// With explicit bosses
	s2, _ := adventure.NewStageWithBosses("s-2", "Forest", 3, []string{"m-1", "m-2", "b-1", "b-2"}, []string{"b-1", "b-2"})
	bosses2 := s2.GetBossIDs()
	if len(bosses2) != 2 || bosses2[0] != "b-1" || bosses2[1] != "b-2" {
		t.Fatalf("expected [b-1, b-2], got %v", bosses2)
	}
	normals2 := s2.GetNormalMonsterIDs()
	if len(normals2) != 2 || normals2[0] != "m-1" || normals2[1] != "m-2" {
		t.Fatalf("expected [m-1, m-2], got %v", normals2)
	}
}

func TestRequiredJobLevel_LegacyParity(t *testing.T) {
	tests := []struct {
		stageID string
		want    int
	}{
		{"stage-01", 0},
		{"stage-02", 0},
		{"stage-03", 1},
		{"stage-04", 2},
		{"stage-15", 13},
		{"stage-23", 6},  // legacy stage 22
		{"stage-25", 50}, // legacy stage 24
		{"stage-26", 10}, // legacy stage 25
	}

	for _, tt := range tests {
		t.Run(tt.stageID, func(t *testing.T) {
			got := adventure.RequiredJobLevel(tt.stageID)
			if got != tt.want {
				t.Errorf("RequiredJobLevel(%s) = %d, want %d", tt.stageID, got, tt.want)
			}
		})
	}
}

func TestCanAccessStage(t *testing.T) {
	cat, err := adventure.InitialStageCatalog()
	if err != nil {
		t.Fatalf("InitialStageCatalog() error = %v", err)
	}

	novice := corecharacter.Character{
		ID:       "c-1",
		Level:    1,
		JobLevel: 0,
	}

	// stage-01: minLevel 1, jobLevel 0 -> OK
	if err := cat.CanAccessStage(novice, "stage-01"); err != nil {
		t.Errorf("novice should access stage-01, got: %v", err)
	}

	// stage-03: minLevel 6, jobLevel 1 -> Level fails first
	if err := cat.CanAccessStage(novice, "stage-03"); err != adventure.ErrLevelRequirementNotMet {
		t.Errorf("expected ErrLevelRequirementNotMet, got: %v", err)
	}

	// Lv 20, JobLevel 0 -> JobLevel fails on stage-03 (requires jobLevel 1)
	experiencedNovice := corecharacter.Character{
		ID:       "c-2",
		Level:    20,
		JobLevel: 0,
	}
	if err := cat.CanAccessStage(experiencedNovice, "stage-03"); err != adventure.ErrJobLevelRequirementNotMet {
		t.Errorf("expected ErrJobLevelRequirementNotMet, got: %v", err)
	}

	// Lv 20, JobLevel 1 -> OK for stage-03
	reincarnated := corecharacter.Character{
		ID:       "c-3",
		Level:    20,
		JobLevel: 1,
	}
	if err := cat.CanAccessStage(reincarnated, "stage-03"); err != nil {
		t.Errorf("reincarnated should access stage-03, got: %v", err)
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
