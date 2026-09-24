package job_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	corejob "github.com/witchcraze/party2re/internal/core/job"
	"github.com/witchcraze/party2re/internal/job"
)

type memoryJobRepository struct {
	data map[string]corejob.CharacterJob
}

func newMemoryJobRepository() *memoryJobRepository {
	return &memoryJobRepository{data: make(map[string]corejob.CharacterJob)}
}

func (r *memoryJobRepository) Save(ctx context.Context, value corejob.CharacterJob) error {
	r.data[value.CharacterID] = value
	return nil
}

func (r *memoryJobRepository) FindByCharacterID(ctx context.Context, characterID string) (corejob.CharacterJob, error) {
	v, ok := r.data[characterID]
	if !ok {
		return corejob.CharacterJob{}, corejob.ErrInvalidCharacter
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
