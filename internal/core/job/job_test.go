package job

import (
	"errors"
	"fmt"
	"testing"
)

func TestCharacterJobChangesAndRecordsHistory(t *testing.T) {
	state, err := NewCharacterJob("character-1", "starter")
	if err != nil {
		t.Fatal(err)
	}
	target, err := NewDefinition("vanguard", "Vanguard", 6, 1, 3, 5, 2, 1, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := state.ChangeTo(target, 20, "unspecified"); err != nil {
		t.Fatal(err)
	}
	if state.CurrentJobID != "vanguard" || len(state.History) != 1 ||
		state.History[0] != (Change{FromJobID: "starter", ToJobID: "vanguard"}) {
		t.Fatalf("state = %#v", state)
	}
}

func TestCharacterJobRejectsUnmetRequirements(t *testing.T) {
	state, _ := NewCharacterJob("character-1", "starter")
	target, _ := NewDefinition("advanced", "Advanced", 3, 3, 3, 3, 3, 10, "female")
	if err := state.ChangeTo(target, 9, "male"); !errors.Is(err, ErrJobUnavailable) {
		t.Fatalf("ChangeTo() error = %v", err)
	}

}

func TestCharacterJobRequiresLevelTwentyAndMasteredPrerequisites(t *testing.T) {
	state, _ := NewCharacterJob("character-1", "starter")
	target := Definition{ID: "advanced", Name: "Advanced", MinLevel: 1, RequiredJobIDs: []string{"base"}}
	if err := state.ChangeTo(target, 20, "unspecified"); !errors.Is(err, ErrJobUnavailable) {
		t.Fatalf("missing prerequisite error = %v", err)
	}
	state.Master("base")
	if err := state.ChangeTo(target, 19, "unspecified"); !errors.Is(err, ErrJobUnavailable) {
		t.Fatalf("low level error = %v", err)
	}
	if err := state.ChangeTo(target, 20, "unspecified"); err != nil {
		t.Fatal(err)
	}
}

func TestCharacterJobRecordsMasterySP(t *testing.T) {
	state, _ := NewCharacterJob("character-1", "job-01")
	if !state.RecordMastery("job-01", 12, 10) || !state.IsMastered("job-01") {
		t.Fatal("expected mastery to be recorded")
	}
	if got, ok := state.MasteredSP("job-01"); !ok || got != 12 {
		t.Fatalf("mastered SP = %d, %v", got, ok)
	}
	if state.RecordMastery("job-01", 20, 10) {
		t.Fatal("mastery should only be recorded once")
	}
}

func TestLegacyRequirementsUseFinalSkillThresholdsAndCatalogItems(t *testing.T) {
	if got := (Definition{ID: "job-22"}).MasteryThreshold(); got != 140 {
		t.Fatalf("job-22 mastery threshold = %d, want 140", got)
	}
	if got := (Definition{ID: "job-70"}).MasteryThreshold(); got != 200 {
		t.Fatalf("job-70 mastery threshold = %d, want 200", got)
	}
	if got := (Definition{ID: "job-84"}).RequiredItem(); got != "armor-29" {
		t.Fatalf("job-84 required item = %q, want armor-29", got)
	}
}

func TestIsCompletionJob(t *testing.T) {
	if !IsCompletionJob("job-01") || !IsCompletionJob("job-72") || !IsCompletionJob("job-42") {
		t.Fatal("expected job-01..job-72 to be completion jobs")
	}
	if IsCompletionJob("job-73") || IsCompletionJob("job-74") || IsCompletionJob("job-00") ||
		IsCompletionJob("starter") || IsCompletionJob("") {
		t.Fatal("expected job-73+ and non-job IDs to NOT be completion jobs")
	}
}

func TestCompletionJobCountAndSuppinUnlock(t *testing.T) {
	state, _ := NewCharacterJob("character-1", "job-01")
	suppinDef, _ := NewDefinition("job-73", "すっぴん", 5, 5, 4, 4, 4, 20, "")

	// Cannot change to job-73 without AllJobsMastered
	if err := state.ChangeTo(suppinDef, 20, "male"); !errors.Is(err, ErrJobUnavailable) {
		t.Fatalf("expected ErrJobUnavailable when changing to job-73 without all jobs mastered, got %v", err)
	}

	// Master 71 jobs
	for i := 1; i <= 71; i++ {
		jobID := fmt.Sprintf("job-%02d", i)
		state.Master(jobID)
	}
	if state.MasteredCompletionJobCount() != 71 {
		t.Fatalf("expected 71 completion jobs mastered, got %d", state.MasteredCompletionJobCount())
	}
	if state.HasMasteredAllCompletionJobs() {
		t.Fatal("should not have mastered all completion jobs at 71")
	}
	if state.CheckAndSetAllJobsMastered() {
		t.Fatal("CheckAndSetAllJobsMastered should return false at 71")
	}

	// Master 72nd job
	state.Master("job-72")
	if state.MasteredCompletionJobCount() != 72 {
		t.Fatalf("expected 72 completion jobs mastered, got %d", state.MasteredCompletionJobCount())
	}
	if !state.HasMasteredAllCompletionJobs() {
		t.Fatal("expected HasMasteredAllCompletionJobs to be true at 72")
	}

	// First trigger of completion
	if !state.CheckAndSetAllJobsMastered() {
		t.Fatal("expected CheckAndSetAllJobsMastered to return true on first completion")
	}
	if !state.AllJobsMastered {
		t.Fatal("expected AllJobsMastered to be true")
	}

	// Second check should return false (no re-trigger)
	if state.CheckAndSetAllJobsMastered() {
		t.Fatal("expected CheckAndSetAllJobsMastered to return false when already completed")
	}

	// Now changing to job-73 should succeed
	if err := state.ChangeTo(suppinDef, 20, "male"); err != nil {
		t.Fatalf("expected ChangeTo suppin to succeed after all jobs mastered, got %v", err)
	}
	if state.CurrentJobID != "job-73" {
		t.Fatalf("expected CurrentJobID to be job-73, got %s", state.CurrentJobID)
	}
}

func TestCharacterJob_Master(t *testing.T) {
	t.Run("nil receiver is safe no-op", func(t *testing.T) {
		var c *CharacterJob
		c.Master("job-01")
	})

	t.Run("empty or whitespace jobID is safe no-op", func(t *testing.T) {
		c, err := NewCharacterJob("char-1", "job-01")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		c.Master("")
		c.Master("   ")
		if len(c.MasteredJobs) != 0 {
			t.Fatalf("expected 0 mastered jobs, got %d", len(c.MasteredJobs))
		}
	})

	t.Run("adds job and trims whitespace", func(t *testing.T) {
		c := &CharacterJob{
			CharacterID:  "char-1",
			CurrentJobID: "job-01",
		}
		c.Master("  job-02  ")
		if len(c.MasteredJobs) != 1 || c.MasteredJobs[0] != "job-02" {
			t.Fatalf("expected [job-02], got %v", c.MasteredJobs)
		}
		if c.MasteredJobSP == nil {
			t.Fatal("expected MasteredJobSP map to be initialized")
		}
	})

	t.Run("prevents duplicate job mastery", func(t *testing.T) {
		c, _ := NewCharacterJob("char-1", "job-01")
		c.Master("job-02")
		c.Master("job-02")
		c.Master(" job-02 ")
		if len(c.MasteredJobs) != 1 {
			t.Fatalf("expected exactly 1 mastered job, got %d (%v)", len(c.MasteredJobs), c.MasteredJobs)
		}
	})
}

func TestCharacterJob_IsMastered(t *testing.T) {
	t.Run("nil receiver returns false", func(t *testing.T) {
		var c *CharacterJob
		if c.IsMastered("job-01") {
			t.Fatal("expected nil receiver to return false")
		}
	})

	t.Run("returns true for mastered job and trims whitespace", func(t *testing.T) {
		c, _ := NewCharacterJob("char-1", "job-01")
		c.Master("job-02")

		if !c.IsMastered("job-02") {
			t.Fatal("expected job-02 to be mastered")
		}
		if !c.IsMastered("  job-02  ") {
			t.Fatal("expected trimmed job-02 to be mastered")
		}
	})

	t.Run("returns false for unmastered or empty jobID", func(t *testing.T) {
		c, _ := NewCharacterJob("char-1", "job-01")
		c.Master("job-02")

		if c.IsMastered("job-03") {
			t.Fatal("expected job-03 to NOT be mastered")
		}
		if c.IsMastered("") {
			t.Fatal("expected empty string to NOT be mastered")
		}
		if c.IsMastered("   ") {
			t.Fatal("expected whitespace-only string to NOT be mastered")
		}
	})
}

func TestCharacterJob_HelperMethods(t *testing.T) {
	t.Run("MasteredJobCount", func(t *testing.T) {
		var nilJob *CharacterJob
		if nilJob.MasteredJobCount() != 0 {
			t.Fatalf("expected 0 for nil receiver, got %d", nilJob.MasteredJobCount())
		}
		c, _ := NewCharacterJob("char-1", "job-01")
		if c.MasteredJobCount() != 0 {
			t.Fatalf("expected 0, got %d", c.MasteredJobCount())
		}
		c.Master("job-01")
		c.Master("job-02")
		if c.MasteredJobCount() != 2 {
			t.Fatalf("expected 2, got %d", c.MasteredJobCount())
		}
	})

	t.Run("HasMasteredAll", func(t *testing.T) {
		c, _ := NewCharacterJob("char-1", "job-01")
		if c.HasMasteredAll(0) {
			t.Fatal("expected false for required <= 0")
		}
		if c.HasMasteredAll(-1) {
			t.Fatal("expected false for negative required")
		}
		if c.HasMasteredAll(1) {
			t.Fatal("expected false when 0 mastered")
		}
		c.Master("job-01")
		if !c.HasMasteredAll(1) {
			t.Fatal("expected true when 1 mastered and 1 required")
		}
	})

	t.Run("RestoreCurrentJob", func(t *testing.T) {
		var nilJob *CharacterJob
		nilJob.RestoreCurrentJob("job-02")

		c, _ := NewCharacterJob("char-1", "job-01")
		c.RestoreCurrentJob("")
		if c.CurrentJobID != "job-01" {
			t.Fatalf("expected job-01 unchanged, got %s", c.CurrentJobID)
		}
		c.RestoreCurrentJob("   ")
		if c.CurrentJobID != "job-01" {
			t.Fatalf("expected job-01 unchanged, got %s", c.CurrentJobID)
		}
		c.RestoreCurrentJob("  job-05  ")
		if c.CurrentJobID != "job-05" {
			t.Fatalf("expected job-05, got %s", c.CurrentJobID)
		}
	})
}
