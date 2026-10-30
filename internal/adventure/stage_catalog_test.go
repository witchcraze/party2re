package adventure_test

import (
	"errors"
	"fmt"
	"testing"
	"time"

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
		{"stage-02", 0},
		{"stage-03", 1},
		{"stage-04", 2},
		{"stage-05", 3},
		{"stage-06", 4},
		{"stage-07", 5},
		{"stage-08", 6},
		{"stage-09", 7},
		{"stage-10", 8},
		{"stage-11", 9},
		{"stage-12", 10},
		{"stage-13", 11},
		{"stage-14", 12},
		{"stage-15", 14},
		{"stage-21", 14},
		{"stage-22", 6},  // legacy stage 22: ワイルドアピアリー
		{"stage-23", 0},  // legacy stage 23: プニプニ雪原
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

	// stage-02: minLevel 5, jobLevel 0 -> Level fails first (level 1 < 5)
	if err := cat.CanAccessStage(novice, "stage-02"); err != adventure.ErrLevelRequirementNotMet {
		t.Errorf("expected ErrLevelRequirementNotMet, got: %v", err)
	}

	// Lv 20, JobLevel 0 -> OK for stage-02 (stage-02 requires jobLevel 0)
	experiencedNovice := corecharacter.Character{
		ID:       "c-2",
		Level:    20,
		JobLevel: 0,
	}
	if err := cat.CanAccessStage(experiencedNovice, "stage-02"); err != nil {
		t.Errorf("expected experiencedNovice to access stage-02, got: %v", err)
	}

	// Lv 20, JobLevel 0 -> JobLevel fails on stage-03 (requires jobLevel 1)
	if err := cat.CanAccessStage(experiencedNovice, "stage-03"); err != adventure.ErrJobLevelRequirementNotMet {
		t.Errorf("expected ErrJobLevelRequirementNotMet for stage-03, got: %v", err)
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

func TestRequiredJobLevel_Stage2To14Boundaries(t *testing.T) {
	cat, err := adventure.InitialStageCatalog()
	if err != nil {
		t.Fatalf("InitialStageCatalog() error = %v", err)
	}

	// For normal stages 2 to 14, RequiredJobLevel = stageNum - 2.
	// For each stage, test JobLevel before (req-1), at (req), and after (req+1).
	for stageNum := 2; stageNum <= 14; stageNum++ {
		stageID := fmt.Sprintf("stage-%02d", stageNum)
		req := stageNum - 2

		t.Run(fmt.Sprintf("%s_req_%d", stageID, req), func(t *testing.T) {
			// Character level 99 ensures MinLevel requirement is always satisfied.
			if req > 0 {
				before := corecharacter.Character{ID: "char-before", Level: 99, JobLevel: req - 1}
				if err := cat.CanAccessStage(before, stageID); !errors.Is(err, adventure.ErrJobLevelRequirementNotMet) {
					t.Errorf("[%s] JobLevel %d (before %d): expected ErrJobLevelRequirementNotMet, got %v", stageID, req-1, req, err)
				}
			}

			at := corecharacter.Character{ID: "char-at", Level: 99, JobLevel: req}
			if err := cat.CanAccessStage(at, stageID); err != nil {
				t.Errorf("[%s] JobLevel %d (exact match): expected nil, got %v", stageID, req, err)
			}

			after := corecharacter.Character{ID: "char-after", Level: 99, JobLevel: req + 1}
			if err := cat.CanAccessStage(after, stageID); err != nil {
				t.Errorf("[%s] JobLevel %d (after %d): expected nil, got %v", stageID, req+1, req, err)
			}
		})
	}

	// Also verify special stages 22, 24, 25 boundaries.
	specialCases := []struct {
		stageID string
		req     int
	}{
		{"stage-22", 6},
		{"stage-24", 50},
	}
	for _, sc := range specialCases {
		t.Run(fmt.Sprintf("%s_special_req_%d", sc.stageID, sc.req), func(t *testing.T) {
			before := corecharacter.Character{ID: "char-before", Level: 99, JobLevel: sc.req - 1}
			if err := cat.CanAccessStage(before, sc.stageID); !errors.Is(err, adventure.ErrJobLevelRequirementNotMet) {
				t.Errorf("[%s] JobLevel %d: expected ErrJobLevelRequirementNotMet, got %v", sc.stageID, sc.req-1, err)
			}

			at := corecharacter.Character{ID: "char-at", Level: 99, JobLevel: sc.req}
			if err := cat.CanAccessStage(at, sc.stageID); err != nil {
				t.Errorf("[%s] JobLevel %d: expected nil, got %v", sc.stageID, sc.req, err)
			}

			after := corecharacter.Character{ID: "char-after", Level: 99, JobLevel: sc.req + 1}
			if err := cat.CanAccessStage(after, sc.stageID); err != nil {
				t.Errorf("[%s] JobLevel %d: expected nil, got %v", sc.stageID, sc.req+1, err)
			}
		})
	}
}

func TestCanAccessStage_SeasonalCalendarBoundaries(t *testing.T) {
	cat, err := adventure.InitialStageCatalog()
	if err != nil {
		t.Fatalf("InitialStageCatalog() error = %v", err)
	}

	jst := time.FixedZone("JST", 9*60*60)
	// Character with high level and job level satisfies level/joblevel for all seasonal stages.
	qualifiedChar := corecharacter.Character{ID: "c-qual", Level: 99, JobLevel: 50}

	t.Run("WinterStages_Stage23_And_Stage25", func(t *testing.T) {
		winterStages := []string{"stage-23", "stage-25"}
		tests := []struct {
			name        string
			t           time.Time
			wantAllowed bool
		}{
			{"Nov 30 23:59:59 JST (autumn, just before winter)", time.Date(2026, 11, 30, 23, 59, 59, 0, jst), false},
			{"Dec 01 00:00:00 JST (winter start boundary)", time.Date(2026, 12, 1, 0, 0, 0, 0, jst), true},
			{"Dec 31 23:59:59 JST (mid winter)", time.Date(2026, 12, 31, 23, 59, 59, 0, jst), true},
			{"Jan 15 12:00:00 JST (mid winter)", time.Date(2027, 1, 15, 12, 0, 0, 0, jst), true},
			{"Feb 28 23:59:59 JST (winter end boundary, common year)", time.Date(2027, 2, 28, 23, 59, 59, 0, jst), true},
			{"Mar 01 00:00:00 JST (spring, just after winter)", time.Date(2027, 3, 1, 0, 0, 0, 0, jst), false},
			{"Feb 29 23:59:59 JST (winter end boundary, leap year 2028)", time.Date(2028, 2, 29, 23, 59, 59, 0, jst), true},
			{"Mar 01 00:00:00 JST (leap year spring start)", time.Date(2028, 3, 1, 0, 0, 0, 0, jst), false},
			{"UTC equivalence boundary: 2026-11-30 14:59:59 UTC = 23:59:59 JST (autumn)", time.Date(2026, 11, 30, 14, 59, 59, 0, time.UTC), false},
			{"UTC equivalence boundary: 2026-11-30 15:00:00 UTC = 2026-12-01 00:00:00 JST (winter)", time.Date(2026, 11, 30, 15, 0, 0, 0, time.UTC), true},
		}

		for _, stID := range winterStages {
			for _, tt := range tests {
				t.Run(fmt.Sprintf("%s/%s", stID, tt.name), func(t *testing.T) {
					err := cat.CanAccessStageAt(qualifiedChar, stID, tt.t)
					if tt.wantAllowed {
						if err != nil {
							t.Errorf("expected access allowed for %s at %v, got %v", stID, tt.t, err)
						}
					} else {
						if !errors.Is(err, adventure.ErrStageNotAvailable) {
							t.Errorf("expected ErrStageNotAvailable for %s at %v, got %v", stID, tt.t, err)
						}
					}
				})
			}
		}
	})

	t.Run("HalloweenStage_Stage27", func(t *testing.T) {
		tests := []struct {
			name        string
			t           time.Time
			wantAllowed bool
		}{
			{"Sep 30 23:59:59 JST (just before October)", time.Date(2026, 9, 30, 23, 59, 59, 0, jst), false},
			{"Oct 01 00:00:00 JST (Halloween start boundary)", time.Date(2026, 10, 1, 0, 0, 0, 0, jst), true},
			{"Oct 15 12:00:00 JST (mid Halloween)", time.Date(2026, 10, 15, 12, 0, 0, 0, jst), true},
			{"Oct 31 23:59:59 JST (Halloween end boundary)", time.Date(2026, 10, 31, 23, 59, 59, 0, jst), true},
			{"Nov 01 00:00:00 JST (just after Halloween)", time.Date(2026, 11, 1, 0, 0, 0, 0, jst), false},
			{"UTC equivalence boundary: 2026-09-30 14:59:59 UTC = 23:59:59 JST (September)", time.Date(2026, 9, 30, 14, 59, 59, 0, time.UTC), false},
			{"UTC equivalence boundary: 2026-09-30 15:00:00 UTC = 2026-10-01 00:00:00 JST (October)", time.Date(2026, 9, 30, 15, 0, 0, 0, time.UTC), true},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				err := cat.CanAccessStageAt(qualifiedChar, "stage-27", tt.t)
				if tt.wantAllowed {
					if err != nil {
						t.Errorf("expected access allowed for stage-27 at %v, got %v", tt.t, err)
					}
				} else {
					if !errors.Is(err, adventure.ErrStageNotAvailable) {
						t.Errorf("expected ErrStageNotAvailable for stage-27 at %v, got %v", tt.t, err)
					}
				}
			})
		}
	})
}

func TestAvailableStages_ParityWithCanAccessStage(t *testing.T) {
	cat, err := adventure.InitialStageCatalog()
	if err != nil {
		t.Fatalf("InitialStageCatalog() error = %v", err)
	}

	jst := time.FixedZone("JST", 9*60*60)
	characters := []corecharacter.Character{
		{ID: "novice-1", Level: 1, JobLevel: 0},
		{ID: "novice-20", Level: 20, JobLevel: 0},
		{ID: "reincarnated-1", Level: 20, JobLevel: 1},
		{ID: "advanced-5", Level: 80, JobLevel: 5},
		{ID: "expert-12", Level: 99, JobLevel: 12},
		{ID: "master-50", Level: 99, JobLevel: 50},
	}

	times := []struct {
		name string
		t    time.Time
	}{
		{"winter-dec", time.Date(2026, 12, 15, 12, 0, 0, 0, jst)},
		{"winter-jan", time.Date(2027, 1, 15, 12, 0, 0, 0, jst)},
		{"spring-apr", time.Date(2027, 4, 15, 12, 0, 0, 0, jst)},
		{"summer-jul", time.Date(2027, 7, 15, 12, 0, 0, 0, jst)},
		{"autumn-oct-halloween", time.Date(2026, 10, 15, 12, 0, 0, 0, jst)},
		{"autumn-nov", time.Date(2026, 11, 15, 12, 0, 0, 0, jst)},
	}

	for _, c := range characters {
		for _, tm := range times {
			t.Run(fmt.Sprintf("%s_%s", c.ID, tm.name), func(t *testing.T) {
				available := cat.AvailableStagesAt(c, tm.t)
				availMap := make(map[string]bool, len(available))
				for _, st := range available {
					availMap[st.ID] = true
				}

				for _, st := range cat.Stages() {
					err := cat.CanAccessStageAt(c, st.ID, tm.t)
					isAvailable := availMap[st.ID]

					if err == nil && !isAvailable {
						t.Errorf("stage %s should be in AvailableStages (CanAccessStageAt succeeded)", st.ID)
					}
					if err != nil && isAvailable {
						t.Errorf("stage %s should NOT be in AvailableStages (CanAccessStageAt failed: %v)", st.ID, err)
					}
				}
			})
		}
	}
}
