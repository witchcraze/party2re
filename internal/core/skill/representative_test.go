package skill_test

import (
	"errors"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/skill"
)

func TestRepresentativeDefinitionsMetadata(t *testing.T) {
	defs := skill.RepresentativeDefinitions()
	if len(defs) != 3 {
		t.Fatalf("expected 3 representative definitions, got %d", len(defs))
	}

	expected := map[string]struct {
		name       string
		jobID      string
		requiredSP int
		mpCost     int
		power      int
		kind       string
	}{
		"job-5-entry-3": {"heal-30", "job-05", 6, 3, 30, corebattle.ActionKindHeal},
		"job-6-entry-1": {"magic-20", "job-06", 1, 2, 20, corebattle.ActionKindAttack},
		"job-6-entry-3": {"magic-wave-25", "job-06", 8, 5, 25, corebattle.ActionKindAttack},
	}

	for _, d := range defs {
		exp, ok := expected[d.ID]
		if !ok {
			t.Fatalf("unexpected definition ID: %s", d.ID)
		}
		if d.Name != exp.name {
			t.Errorf("[%s] expected name %s, got %s", d.ID, exp.name, d.Name)
		}
		if len(d.RequiredJobIDs) != 1 || d.RequiredJobIDs[0] != exp.jobID {
			t.Errorf("[%s] expected RequiredJobIDs [%s], got %v", d.ID, exp.jobID, d.RequiredJobIDs)
		}
		if d.RequiredSP != exp.requiredSP {
			t.Errorf("[%s] expected RequiredSP %d, got %d", d.ID, exp.requiredSP, d.RequiredSP)
		}
		if d.MPCost != exp.mpCost {
			t.Errorf("[%s] expected MPCost %d, got %d", d.ID, exp.mpCost, d.MPCost)
		}
		if d.Effect.Power != exp.power {
			t.Errorf("[%s] expected power %d, got %d", d.ID, exp.power, d.Effect.Power)
		}
		if d.Effect.Kind != exp.kind {
			t.Errorf("[%s] expected kind %s, got %s", d.ID, exp.kind, d.Effect.Kind)
		}
	}
}

func TestRepresentativeBattleDefinitionsAdmission(t *testing.T) {
	bd := skill.ProductionBattleDefinitions()

	tests := []struct {
		id          string
		expectedAct corebattle.ActionSkill
	}{
		{
			id: "job-5-entry-3",
			expectedAct: corebattle.ActionSkill{
				ID:              "job-5-entry-3",
				Name:            "heal-30",
				MPCost:          3,
				Power:           30,
				Kind:            corebattle.ActionKindHeal,
				TargetScope:     corebattle.TargetScopeSingleAlly,
				Category:        string(corebattle.CategoryMagic),
				Item249Modifier: 2,
			},
		},
		{
			id: "job-6-entry-1",
			expectedAct: corebattle.ActionSkill{
				ID:              "job-6-entry-1",
				Name:            "magic-20",
				MPCost:          2,
				Power:           20,
				Kind:            corebattle.ActionKindAttack,
				TargetScope:     corebattle.TargetScopeSingleEnemy,
				Category:        string(corebattle.CategoryMagic),
				Direct:          true,
				Item249Modifier: 1,
			},
		},
		{
			id: "job-6-entry-3",
			expectedAct: corebattle.ActionSkill{
				ID:              "job-6-entry-3",
				Name:            "magic-wave-25",
				MPCost:          5,
				Power:           25,
				Kind:            corebattle.ActionKindAttack,
				TargetScope:     corebattle.TargetScopeAllEnemies,
				Category:        string(corebattle.CategoryMagic),
				Direct:          true,
				Item249Modifier: 1,
			},
		},
	}

	catalog := skill.InitialCatalog()
	for _, tc := range tests {
		def, err := catalog.FindByID(tc.id)
		if err != nil {
			t.Fatalf("FindByID(%s) error = %v", tc.id, err)
		}
		action, err := bd.JobAction(def)
		if err != nil {
			t.Fatalf("[%s] JobAction error = %v", tc.id, err)
		}
		if action != tc.expectedAct {
			t.Errorf("[%s] expected action %+v, got %+v", tc.id, tc.expectedAct, action)
		}
	}

	// Uncertified entry must be rejected
	uncertified := skill.Definition{
		ID:         "job-99-unknown",
		Name:       "unknown",
		RequiredSP: 1,
		MPCost:     5,
		Effect:     corebattle.Effect{Kind: corebattle.ActionKindAttack, Power: 50},
	}
	if _, err := bd.JobAction(uncertified); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
		t.Fatalf("expected ErrUnsupportedEffect for uncertified entry, got %v", err)
	}
}

func TestCatalogLookup(t *testing.T) {
	c := skill.InitialCatalog()

	// SkillsForJob canonical IDs
	j05Skills := c.SkillsForJob("job-05")
	if len(j05Skills) != 1 || j05Skills[0].ID != "job-5-entry-3" {
		t.Fatalf("SkillsForJob(job-05) = %v, want [job-5-entry-3]", j05Skills)
	}

	j06Skills := c.SkillsForJob("job-06")
	if len(j06Skills) != 2 || j06Skills[0].ID != "job-6-entry-1" || j06Skills[1].ID != "job-6-entry-3" {
		t.Fatalf("SkillsForJob(job-06) = %v, want [job-6-entry-1, job-6-entry-3]", j06Skills)
	}

	j01Skills := c.SkillsForJob("job-01")
	if len(j01Skills) != 0 {
		t.Fatalf("SkillsForJob(job-01) expected empty, got %v", j01Skills)
	}

	// Non-canonical "job-5" should not match canonical "job-05"
	if len(c.SkillsForJob("job-5")) != 0 {
		t.Fatalf("SkillsForJob(job-5) should not match stored canonical job-05")
	}

	// FindByID
	if _, err := c.FindByID("job-5-entry-3"); err != nil {
		t.Fatalf("FindByID(job-5-entry-3) error = %v", err)
	}
	if _, err := c.FindByID("non-existent"); !errors.Is(err, skill.ErrDefinitionNotFound) {
		t.Fatalf("FindByID(non-existent) error = %v, want ErrDefinitionNotFound", err)
	}

	// Definitions
	if len(c.Definitions()) != 3 {
		t.Fatalf("Definitions() length = %d, want 3", len(c.Definitions()))
	}
}

func TestSkillSPGatesCurrentAndPreviousJob(t *testing.T) {
	catalog := skill.InitialCatalog()
	healDef, _ := catalog.FindByID("job-5-entry-3")   // req: job-05, SP 6, MP 3
	singleAtk, _ := catalog.FindByID("job-6-entry-1") // req: job-06, SP 1, MP 2
	allAtk, _ := catalog.FindByID("job-6-entry-3")    // req: job-06, SP 8, MP 5

	// 1. Current Job SP gates: immediately before, equal to, immediately after
	testsCurrent := []struct {
		name       string
		def        skill.Definition
		jobID      string
		sp         int
		mp         int
		wantUnlock bool
		wantErr    error
	}{
		// heal-30 (SP 6, MP 3)
		{"heal current SP 5 (before)", healDef, "job-05", 5, 10, false, skill.ErrUnavailable},
		{"heal current SP 6 (equal)", healDef, "job-05", 6, 10, true, nil},
		{"heal current SP 7 (after)", healDef, "job-05", 7, 10, true, nil},
		{"heal current SP 6 with insufficient MP", healDef, "job-05", 6, 2, true, skill.ErrInsufficientMP},

		// magic-20 (SP 1, MP 2)
		{"magic-20 current SP 0 (before)", singleAtk, "job-06", 0, 10, false, skill.ErrUnavailable},
		{"magic-20 current SP 1 (equal)", singleAtk, "job-06", 1, 10, true, nil},
		{"magic-20 current SP 2 (after)", singleAtk, "job-06", 2, 10, true, nil},
		{"magic-20 current SP 1 with insufficient MP", singleAtk, "job-06", 1, 1, true, skill.ErrInsufficientMP},

		// magic-wave-25 (SP 8, MP 5)
		{"magic-wave-25 current SP 7 (before)", allAtk, "job-06", 7, 10, false, skill.ErrUnavailable},
		{"magic-wave-25 current SP 8 (equal)", allAtk, "job-06", 8, 10, true, nil},
		{"magic-wave-25 current SP 9 (after)", allAtk, "job-06", 9, 10, true, nil},
		{"magic-wave-25 current SP 8 with insufficient MP", allAtk, "job-06", 8, 4, true, skill.ErrInsufficientMP},
	}

	for _, tc := range testsCurrent {
		t.Run(tc.name, func(t *testing.T) {
			char := corecharacter.Character{
				JobID: tc.jobID,
				SP:    tc.sp,
				Stats: corecharacter.Stats{MP: tc.mp},
			}
			unlocked := tc.def.IsUnlocked(char)
			if unlocked != tc.wantUnlock {
				t.Fatalf("IsUnlocked = %v, want %v", unlocked, tc.wantUnlock)
			}
			err := tc.def.CanUse(skill.UseRequest{Character: &char})
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("CanUse error = %v, want %v", err, tc.wantErr)
			}
		})
	}

	// 2. Previous Job SP gates: current SP must NEVER substitute for previous SP
	testsPrevious := []struct {
		name       string
		def        skill.Definition
		currentJob string
		currentSP  int
		oldJob     string
		oldSP      int
		mp         int
		wantUnlock bool
		wantErr    error
	}{
		// heal-30 via old job (job-05, req SP 6): current job has SP 100
		{"heal old SP 5 (before, current SP 100)", healDef, "job-01", 100, "job-05", 5, 10, false, skill.ErrUnavailable},
		{"heal old SP 6 (equal, current SP 100)", healDef, "job-01", 100, "job-05", 6, 10, true, nil},
		{"heal old SP 7 (after, current SP 100)", healDef, "job-01", 100, "job-05", 7, 10, true, nil},
		{"heal old SP 6 with insufficient MP", healDef, "job-01", 100, "job-05", 6, 1, true, skill.ErrInsufficientMP},

		// magic-20 via old job (job-06, req SP 1): current job has SP 50
		{"magic-20 old SP 0 (before)", singleAtk, "job-01", 50, "job-06", 0, 10, false, skill.ErrUnavailable},
		{"magic-20 old SP 1 (equal)", singleAtk, "job-01", 50, "job-06", 1, 10, true, nil},
		{"magic-20 old SP 2 (after)", singleAtk, "job-01", 50, "job-06", 2, 10, true, nil},

		// magic-wave-25 via old job (job-06, req SP 8): current job has SP 50
		{"magic-wave-25 old SP 7 (before)", allAtk, "job-01", 50, "job-06", 7, 10, false, skill.ErrUnavailable},
		{"magic-wave-25 old SP 8 (equal)", allAtk, "job-01", 50, "job-06", 8, 10, true, nil},
		{"magic-wave-25 old SP 9 (after)", allAtk, "job-01", 50, "job-06", 9, 10, true, nil},

		// Same job in current and old: current SP insufficient (5), old SP sufficient (6)
		{"same job current SP 5 old SP 6 unlocks", healDef, "job-05", 5, "job-05", 6, 10, true, nil},
		// Same job in current and old: both insufficient (5 and 5)
		{"same job current SP 5 old SP 5 locked", healDef, "job-05", 5, "job-05", 5, 10, false, skill.ErrUnavailable},
	}

	for _, tc := range testsPrevious {
		t.Run(tc.name, func(t *testing.T) {
			char := corecharacter.Character{
				JobID:    tc.currentJob,
				SP:       tc.currentSP,
				OldJobID: tc.oldJob,
				OldSP:    tc.oldSP,
				Stats:    corecharacter.Stats{MP: tc.mp},
			}
			unlocked := tc.def.IsUnlocked(char)
			if unlocked != tc.wantUnlock {
				t.Fatalf("IsUnlocked = %v, want %v", unlocked, tc.wantUnlock)
			}
			err := tc.def.CanUse(skill.UseRequest{Character: &char})
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("CanUse error = %v, want %v", err, tc.wantErr)
			}
		})
	}
}
