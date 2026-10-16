package job_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	corejob "github.com/witchcraze/party2re/internal/core/job"
	"github.com/witchcraze/party2re/internal/job"
)

type memoryJobRepository struct {
	data map[string]corejob.CharacterJob
	err  error
}

func newMemoryJobRepository() *memoryJobRepository {
	return &memoryJobRepository{data: make(map[string]corejob.CharacterJob)}
}

func (r *memoryJobRepository) Save(ctx context.Context, value corejob.CharacterJob) error {
	r.data[value.CharacterID] = value
	return nil
}

func (r *memoryJobRepository) FindByCharacterID(ctx context.Context, characterID string) (corejob.CharacterJob, error) {
	if r.err != nil {
		return corejob.CharacterJob{}, r.err
	}
	v, ok := r.data[characterID]
	if !ok {
		return corejob.CharacterJob{}, corejob.ErrCharacterJobNotFound
	}
	return v, nil
}

type memoryCharRepository struct {
	data map[string]corecharacter.Character
}

func newMemoryCharRepository() *memoryCharRepository {
	return &memoryCharRepository{data: make(map[string]corecharacter.Character)}
}

func (r *memoryCharRepository) FindByID(ctx context.Context, id string) (corecharacter.Character, error) {
	v, ok := r.data[id]
	if !ok {
		return corecharacter.Character{}, corecharacter.ErrNotFound
	}
	return v, nil
}

func (r *memoryCharRepository) FindByIDForUpdate(ctx context.Context, id string) (corecharacter.Character, error) {
	return r.FindByID(ctx, id)
}

func (r *memoryCharRepository) Update(ctx context.Context, value corecharacter.Character) error {
	r.data[value.ID] = value
	return nil
}

func TestCalculateMasteryPercentage(t *testing.T) {
	tests := []struct {
		name          string
		masteredCount int
		learningCount int
		totalJobs     int
		expected      int
	}{
		{
			name:          "zero mastered zero learning",
			masteredCount: 0,
			learningCount: 0,
			totalJobs:     87,
			expected:      0,
		},
		{
			name:          "one learning job (0.5 count)",
			masteredCount: 0,
			learningCount: 1,
			totalJobs:     87,
			expected:      0, // int(0.5 / 86 * 100) = int(0.581) = 0
		},
		{
			name:          "two learning jobs (1.0 count)",
			masteredCount: 0,
			learningCount: 2,
			totalJobs:     87,
			expected:      1, // int(1.0 / 86 * 100) = int(1.162) = 1
		},
		{
			name:          "one mastered job",
			masteredCount: 1,
			learningCount: 0,
			totalJobs:     87,
			expected:      1, // int(1.0 / 86 * 100) = int(1.162) = 1
		},
		{
			name:          "43 mastered jobs",
			masteredCount: 43,
			learningCount: 0,
			totalJobs:     87,
			expected:      50, // int(43.0 / 86 * 100) = 50
		},
		{
			name:          "43 mastered and 1 learning",
			masteredCount: 43,
			learningCount: 1,
			totalJobs:     87,
			expected:      50, // int(43.5 / 86 * 100) = int(50.581) = 50
		},
		{
			name:          "72 mastered jobs",
			masteredCount: 72,
			learningCount: 0,
			totalJobs:     87,
			expected:      83, // int(72.0 / 86 * 100) = int(83.72) = 83
		},
		{
			name:          "86 mastered jobs",
			masteredCount: 86,
			learningCount: 0,
			totalJobs:     87,
			expected:      100, // int(86.0 / 86 * 100) = 100
		},
		{
			name:          "all 87 mastered jobs (clamped to 100)",
			masteredCount: 87,
			learningCount: 0,
			totalJobs:     87,
			expected:      100, // int(87.0 / 86 * 100) = int(101.16) = 101 -> clamped to 100
		},
		{
			name:          "86 mastered and 1 learning (clamped to 100)",
			masteredCount: 86,
			learningCount: 1,
			totalJobs:     87,
			expected:      100, // int(86.5 / 86 * 100) = int(100.58) = 100
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := job.CalculateMasteryPercentage(tc.masteredCount, tc.learningCount, tc.totalJobs)
			if got != tc.expected {
				t.Fatalf("expected %d, got %d", tc.expected, got)
			}
		})
	}
}

func TestGetJobMastery(t *testing.T) {
	jobRepo := newMemoryJobRepository()
	charRepo := newMemoryCharRepository()

	svc, err := job.NewService(
		jobRepo,
		job.WithCharacterRepository(charRepo),
	)
	if err != nil {
		t.Fatalf("failed to create job service: %v", err)
	}

	ctx := context.Background()

	// 1. Character not found
	_, err = svc.GetJobMastery(ctx, "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent character, got nil")
	}

	// 2. Initial character (starter job, no experience)
	charRepo.data["char-1"] = corecharacter.Character{
		ID:    "char-1",
		JobID: "starter",
		SP:    0,
	}
	mastery, err := svc.GetJobMastery(ctx, "char-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mastery.CharacterID != "char-1" {
		t.Errorf("expected character id char-1, got %s", mastery.CharacterID)
	}
	if len(mastery.Jobs) != 87 {
		t.Errorf("expected 87 jobs, got %d", len(mastery.Jobs))
	}
	if mastery.MasteredCount != 0 || mastery.LearningCount != 0 || mastery.UnlearnedCount != 87 {
		t.Errorf("unexpected counts: mastered=%d, learning=%d, unlearned=%d",
			mastery.MasteredCount, mastery.LearningCount, mastery.UnlearnedCount)
	}
	if mastery.MasteryPercentage != 0 {
		t.Errorf("expected 0%% mastery, got %d%%", mastery.MasteryPercentage)
	}
	if mastery.AllJobsMastered {
		t.Error("expected AllJobsMastered=false")
	}

	// 3. Character currently learning job-01 (SP 20/80)
	charRepo.data["char-2"] = corecharacter.Character{
		ID:    "char-2",
		JobID: "job-01",
		SP:    20,
	}
	mastery, err = svc.GetJobMastery(ctx, "char-2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mastery.MasteredCount != 0 || mastery.LearningCount != 1 || mastery.UnlearnedCount != 86 {
		t.Errorf("unexpected counts: mastered=%d, learning=%d, unlearned=%d",
			mastery.MasteredCount, mastery.LearningCount, mastery.UnlearnedCount)
	}
	if mastery.Jobs[0].JobID != "job-01" || mastery.Jobs[0].Status != job.JobStatusLearning ||
		mastery.Jobs[0].CurrentSP != 20 || mastery.Jobs[0].MasterSP != 80 {
		t.Errorf("unexpected job-01 progress: %+v", mastery.Jobs[0])
	}

	// 4. Character with mastered job, old learning job, and history learning job
	charRepo.data["char-3"] = corecharacter.Character{
		ID:       "char-3",
		JobID:    "job-01",
		SP:       80, // job-01 threshold is 80 -> mastered!
		OldJobID: "job-02",
		OldSP:    15, // job-02 threshold is 100 -> learning
	}
	jobRepo.data["char-3"] = corejob.CharacterJob{
		CharacterID:   "char-3",
		CurrentJobID:  "job-01",
		MasteredJobs:  []string{"job-01"},
		MasteredJobSP: map[string]int{"job-01": 80},
		History: []corejob.Change{
			{FromJobID: "job-03", ToJobID: "job-02"},
			{FromJobID: "job-02", ToJobID: "job-01"},
		},
	}
	mastery, err = svc.GetJobMastery(ctx, "char-3")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mastery.MasteredCount != 1 {
		t.Errorf("expected 1 mastered, got %d", mastery.MasteredCount)
	}
	// Learning: job-02 and job-03
	if mastery.LearningCount != 2 {
		t.Errorf("expected 2 learning, got %d", mastery.LearningCount)
	}
	if mastery.UnlearnedCount != 84 {
		t.Errorf("expected 84 unlearned, got %d", mastery.UnlearnedCount)
	}
	// count = 1.0 + 0.5 * 2 = 2.0 -> int(2.0 / 86 * 100) = 2
	if mastery.MasteryPercentage != 2 {
		t.Errorf("expected 2%% mastery, got %d%%", mastery.MasteryPercentage)
	}

	// 5. 72 jobs mastered
	masteredList := make([]string, 0, 72)
	for i := 1; i <= 72; i++ {
		masteredList = append(masteredList, fmt.Sprintf("job-%02d", i))
	}
	charRepo.data["char-4"] = corecharacter.Character{
		ID:    "char-4",
		JobID: "job-73",
		SP:    0,
	}
	jobRepo.data["char-4"] = corejob.CharacterJob{
		CharacterID:     "char-4",
		CurrentJobID:    "job-73",
		MasteredJobs:    masteredList,
		AllJobsMastered: true,
	}
	mastery, err = svc.GetJobMastery(ctx, "char-4")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !mastery.AllJobsMastered {
		t.Error("expected AllJobsMastered=true")
	}
	if mastery.CompletionTitle != "ジョブマスター" {
		t.Errorf("expected title ジョブマスター, got %s", mastery.CompletionTitle)
	}
	if mastery.MasteredCount != 72 {
		t.Errorf("expected 72 mastered, got %d", mastery.MasteredCount)
	}
}

type mockLegendInductor struct {
	calls []legendCall
}

type legendCall struct {
	Category    string
	CharacterID string
}

func (m *mockLegendInductor) RecordLegend(_ context.Context, category, characterID string) error {
	m.calls = append(m.calls, legendCall{Category: category, CharacterID: characterID})
	return nil
}

type mockNewsPublisher struct {
	newsCount int
}

func (m *mockNewsPublisher) PublishNews(_ context.Context, _, _, _, _ string, _ time.Time) error {
	m.newsCount++
	return nil
}

func TestCheckAndApplyMastery_AllJobsMastered_LegendInduction(t *testing.T) {
	ctx := context.Background()
	jobRepo := newMemoryJobRepository()
	charRepo := newMemoryCharRepository()
	legend := &mockLegendInductor{}
	news := &mockNewsPublisher{}

	svc, err := job.NewService(
		jobRepo,
		job.WithCharacterRepository(charRepo),
		job.WithLegendInductor(legend),
		job.WithNewsPublisher(news),
	)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	// 71 jobs mastered (job-01 to job-71)
	mastered71 := make([]string, 0, 71)
	for i := 1; i <= 71; i++ {
		mastered71 = append(mastered71, fmt.Sprintf("job-%02d", i))
	}

	charRepo.data["char-legend"] = corecharacter.Character{
		ID:    "char-legend",
		Name:  "JobMasterHero",
		JobID: "job-72",
		SP:    500, // plenty of SP
	}
	jobRepo.data["char-legend"] = corejob.CharacterJob{
		CharacterID:  "char-legend",
		CurrentJobID: "job-72",
		MasteredJobs: mastered71,
	}

	// Master 72nd job
	mastered, err := svc.CheckAndApplyMastery(ctx, "char-legend", 500)
	if err != nil {
		t.Fatalf("CheckAndApplyMastery failed: %v", err)
	}
	if !mastered {
		t.Fatal("expected 72nd job to be mastered")
	}

	// Verify news and legend induction
	if news.newsCount != 1 {
		t.Errorf("expected 1 news publication, got %d", news.newsCount)
	}
	if len(legend.calls) != 1 {
		t.Fatalf("expected 1 legend call, got %d", len(legend.calls))
	}
	if legend.calls[0].Category != "comp_job" || legend.calls[0].CharacterID != "char-legend" {
		t.Errorf("unexpected legend call: %+v", legend.calls[0])
	}

	// Second check -> already completed, idempotent
	masteredAgain, err := svc.CheckAndApplyMastery(ctx, "char-legend", 500)
	if err != nil {
		t.Fatalf("second CheckAndApplyMastery failed: %v", err)
	}
	if masteredAgain {
		t.Error("expected second mastery to return false")
	}
	if len(legend.calls) != 1 {
		t.Errorf("expected still 1 legend call, got %d", len(legend.calls))
	}
}

func TestGetJobMastery_PersistedJobRecordsAndZeroSP(t *testing.T) {
	ctx := context.Background()
	jobRepo := newMemoryJobRepository()
	charRepo := newMemoryCharRepository()

	svc, err := job.NewService(
		jobRepo,
		job.WithCharacterRepository(charRepo),
	)
	if err != nil {
		t.Fatalf("failed to create job service: %v", err)
	}

	charID := "char-persisted"
	char := corecharacter.Character{
		ID:       charID,
		JobID:    "job-03",
		SP:       9,
		OldJobID: "job-04",
		OldSP:    11,
	}
	charRepo.data[charID] = char

	jobState, err := corejob.NewCharacterJob(charID, "job-03")
	if err != nil {
		t.Fatal(err)
	}
	// 1. job-01: mastered, then recorded SP set to 0
	jobState.RecordMastery("job-01", 80, 80)
	if err := jobState.RecordJobSP("job-01", 0); err != nil {
		t.Fatal(err)
	}
	// 2. job-02: unmastered, recorded SP set to 17 (History is empty)
	if err := jobState.RecordJobSP("job-02", 17); err != nil {
		t.Fatal(err)
	}
	// 3. job-05: migration-only preserved SP (is_recorded = false, not in RecordedJobIDs)
	jobState.RecordedJobSP["job-05"] = 50
	// 4. job-06: history-only transition, unknown SP
	jobState.History = append(jobState.History, corejob.Change{FromJobID: "job-06", ToJobID: "job-03"})
	// 5. job-07: mastered without saved SP (legacy fallback)
	jobState.MasteredJobs = append(jobState.MasteredJobs, "job-07")

	jobRepo.data[charID] = jobState

	mastery, err := svc.GetJobMastery(ctx, charID)
	if err != nil {
		t.Fatalf("GetJobMastery failed: %v", err)
	}

	findProgress := func(id string) job.JobProgress {
		for _, j := range mastery.Jobs {
			if j.JobID == id {
				return j
			}
		}
		t.Fatalf("job %s not found in mastery progress", id)
		return job.JobProgress{}
	}

	// job-01: mastered with 0 SP must return mastered and CurrentSP == 0
	j01 := findProgress("job-01")
	if j01.Status != job.JobStatusMastered || j01.CurrentSP != 0 {
		t.Errorf("job-01: expected mastered / SP 0, got %s / SP %d", j01.Status, j01.CurrentSP)
	}

	// job-02: explicit record must return learning and CurrentSP == 17
	j02 := findProgress("job-02")
	if j02.Status != job.JobStatusLearning || j02.CurrentSP != 17 {
		t.Errorf("job-02: expected learning / SP 17, got %s / SP %d", j02.Status, j02.CurrentSP)
	}

	// job-03: live current job must return learning and CurrentSP == 9
	j03 := findProgress("job-03")
	if j03.Status != job.JobStatusLearning || j03.CurrentSP != 9 {
		t.Errorf("job-03: expected learning / SP 9, got %s / SP %d", j03.Status, j03.CurrentSP)
	}

	// job-04: live old job must return learning and CurrentSP == 11
	j04 := findProgress("job-04")
	if j04.Status != job.JobStatusLearning || j04.CurrentSP != 11 {
		t.Errorf("job-04: expected learning / SP 11, got %s / SP %d", j04.Status, j04.CurrentSP)
	}

	// job-05: migration-only preserved SP without record presence must remain unlearned
	j05 := findProgress("job-05")
	if j05.Status != job.JobStatusUnlearned || j05.CurrentSP != 0 {
		t.Errorf("job-05: expected unlearned / SP 0, got %s / SP %d", j05.Status, j05.CurrentSP)
	}

	// job-06: history-only job without saved SP must be learning with SP 0
	j06 := findProgress("job-06")
	if j06.Status != job.JobStatusLearning || j06.CurrentSP != 0 {
		t.Errorf("job-06: expected learning / SP 0, got %s / SP %d", j06.Status, j06.CurrentSP)
	}

	// job-07: mastered without saved SP falls back to masterSP
	j07 := findProgress("job-07")
	if j07.Status != job.JobStatusMastered || j07.CurrentSP != j07.MasterSP {
		t.Errorf("job-07: expected mastered / SP %d, got %s / SP %d", j07.MasterSP, j07.Status, j07.CurrentSP)
	}

	// Check counts: mastered = 2 (job-01, job-07), learning = 4 (job-02, job-03, job-04, job-06)
	if mastery.MasteredCount != 2 {
		t.Errorf("expected MasteredCount=2, got %d", mastery.MasteredCount)
	}
	if mastery.LearningCount != 4 {
		t.Errorf("expected LearningCount=4, got %d", mastery.LearningCount)
	}
}

func TestGetJobMastery_LiveSPPriorityOverSavedSP(t *testing.T) {
	ctx := context.Background()
	jobRepo := newMemoryJobRepository()
	charRepo := newMemoryCharRepository()

	svc, err := job.NewService(
		jobRepo,
		job.WithCharacterRepository(charRepo),
	)
	if err != nil {
		t.Fatalf("failed to create job service: %v", err)
	}

	charID := "char-priority"
	char := corecharacter.Character{
		ID:       charID,
		JobID:    "job-01",
		SP:       5, // live current SP
		OldJobID: "job-02",
		OldSP:    7, // live old SP
	}
	charRepo.data[charID] = char

	jobState, err := corejob.NewCharacterJob(charID, "job-01")
	if err != nil {
		t.Fatal(err)
	}
	// SavedSP has different values
	if err := jobState.RecordJobSP("job-01", 30); err != nil {
		t.Fatal(err)
	}
	if err := jobState.RecordJobSP("job-02", 40); err != nil {
		t.Fatal(err)
	}
	jobRepo.data[charID] = jobState

	mastery, err := svc.GetJobMastery(ctx, charID)
	if err != nil {
		t.Fatalf("GetJobMastery failed: %v", err)
	}

	for _, j := range mastery.Jobs {
		if j.JobID == "job-01" {
			if j.CurrentSP != 5 {
				t.Errorf("job-01: expected live SP 5 to override saved SP 30, got %d", j.CurrentSP)
			}
		}
		if j.JobID == "job-02" {
			if j.CurrentSP != 7 {
				t.Errorf("job-02: expected live OldSP 7 to override saved SP 40, got %d", j.CurrentSP)
			}
		}
	}
}

func TestGetJobMastery_PropagatesStorageAndCancellationErrors(t *testing.T) {
	ctx := context.Background()
	jobRepo := newMemoryJobRepository()
	charRepo := newMemoryCharRepository()

	svc, err := job.NewService(
		jobRepo,
		job.WithCharacterRepository(charRepo),
	)
	if err != nil {
		t.Fatalf("failed to create job service: %v", err)
	}

	charID := "char-unreadable"
	charRepo.data[charID] = corecharacter.Character{
		ID:       charID,
		JobID:    "job-03",
		SP:       9,
		OldJobID: "job-04",
		OldSP:    11,
	}

	for _, wantErr := range []error{context.Canceled, errors.New("storage failure")} {
		jobRepo.err = wantErr
		_, err := svc.GetJobMastery(ctx, charID)
		if !errors.Is(err, wantErr) {
			t.Fatalf("GetJobMastery error = %v, want %v", err, wantErr)
		}
	}
}
