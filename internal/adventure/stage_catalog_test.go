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
		{"stage-00", 0},
		{"stage-01", 0},
		{"stage-02", 1},
		{"stage-03", 2},
		{"stage-04", 3},
		{"stage-14", 13},
		{"stage-22", 6},  // legacy stage 22: ワイルドアピアリー
		{"stage-24", 50}, // legacy stage 24: 白亜の宮殿
		{"stage-25", 10}, // legacy stage 25: 氷の彫刻館
		{"stage-26", 0},  // legacy stage 26: 神秘の森 / 四季
		{"stage-27", 0},  // legacy stage 27: ハロウィンタウン
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

	// stage-00: minLevel 1, jobLevel 0 -> OK
	if err := cat.CanAccessStage(novice, "stage-00"); err != nil {
		t.Errorf("novice should access stage-00, got: %v", err)
	}

	// stage-02: minLevel 5, jobLevel 1 -> Level fails first
	if err := cat.CanAccessStage(novice, "stage-02"); err != adventure.ErrLevelRequirementNotMet {
		t.Errorf("expected ErrLevelRequirementNotMet, got: %v", err)
	}

	// Lv 20, JobLevel 0 -> JobLevel fails on stage-02 (requires jobLevel 1)
	experiencedNovice := corecharacter.Character{
		ID:       "c-2",
		Level:    20,
		JobLevel: 0,
	}
	if err := cat.CanAccessStage(experiencedNovice, "stage-02"); err != adventure.ErrJobLevelRequirementNotMet {
		t.Errorf("expected ErrJobLevelRequirementNotMet, got: %v", err)
	}

	// Lv 20, JobLevel 1 -> OK for stage-02
	reincarnated := corecharacter.Character{
		ID:       "c-3",
		Level:    20,
		JobLevel: 1,
	}
	if err := cat.CanAccessStage(reincarnated, "stage-02"); err != nil {
		t.Errorf("reincarnated should access stage-02, got: %v", err)
	}
}

func TestStage26Seasons(t *testing.T) {
	cat, err := adventure.InitialStageCatalog()
	if err != nil {
		t.Fatalf("InitialStageCatalog() error = %v", err)
	}

	stage26, err := cat.FindByID("stage-26")
	if err != nil {
		t.Fatalf("FindByID(stage-26) error = %v", err)
	}

	if len(stage26.Seasons) != 4 {
		t.Fatalf("stage-26 seasons count = %d, want 4", len(stage26.Seasons))
	}

	seasonsExpected := map[string]struct {
		wantBoss   string
		wantWeapon string
	}{
		"spring": {wantBoss: "monster-279", wantWeapon: "weapon-01"},
		"summer": {wantBoss: "monster-287", wantWeapon: "weapon-06"},
		"autumn": {wantBoss: "monster-016", wantWeapon: "weapon-01"},
		"winter": {wantBoss: "monster-289", wantWeapon: "weapon-01"},
	}

	for season, expected := range seasonsExpected {
		seasonal := stage26.ForSeason(season)
		if len(seasonal.BossIDs) == 0 || seasonal.BossIDs[0] != expected.wantBoss {
			t.Errorf("season %s boss = %v, want %s", season, seasonal.BossIDs, expected.wantBoss)
		}
		if len(seasonal.TreasureWeapons) == 0 || seasonal.TreasureWeapons[0] != expected.wantWeapon {
			t.Errorf("season %s weapon = %v, want %s", season, seasonal.TreasureWeapons, expected.wantWeapon)
		}
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
