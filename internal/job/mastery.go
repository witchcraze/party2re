package job

import (
	"context"
	"errors"
	"slices"

	corejob "github.com/witchcraze/party2re/internal/core/job"
)

const (
	TotalMasteryJobs         = 87
	CompletionJobMasterTitle = "ジョブマスター"
)

type JobStatus string

const (
	JobStatusUnlearned JobStatus = "unlearned"
	JobStatusLearning  JobStatus = "learning"
	JobStatusMastered  JobStatus = "mastered"
)

type JobProgress struct {
	JobID     string    `json:"job_id"`
	JobName   string    `json:"job_name"`
	Status    JobStatus `json:"status"`
	CurrentSP int       `json:"current_sp"`
	MasterSP  int       `json:"master_sp"`
}

type CharacterJobMastery struct {
	CharacterID       string        `json:"character_id"`
	MasteryPercentage int           `json:"mastery_percentage"`
	MasteredCount     int           `json:"mastered_count"`
	LearningCount     int           `json:"learning_count"`
	UnlearnedCount    int           `json:"unlearned_count"`
	AllJobsMastered   bool          `json:"all_jobs_mastered"`
	CompletionTitle   string        `json:"completion_title,omitempty"`
	Jobs              []JobProgress `json:"jobs"`
}

// CalculateMasteryPercentage computes the legacy completion percentage:
// $count += $is_master ? 1 : 0.5;
// $comp_par = int($count / ($#jobs-1) * 100);
// $comp_par = 100 if $comp_par > 100;
func CalculateMasteryPercentage(masteredCount, learningCount, totalJobs int) int {
	if totalJobs <= 1 {
		return 0
	}
	count := float64(masteredCount) + 0.5*float64(learningCount)
	denominator := float64(totalJobs - 1)
	percentage := int(count / denominator * 100.0)
	if percentage > 100 {
		return 100
	}
	if percentage < 0 {
		return 0
	}
	return percentage
}

// GetJobMastery returns the character's mastery progress across all 87 catalog jobs.
func (s *Service) GetJobMastery(ctx context.Context, characterID string) (CharacterJobMastery, error) {
	if s.characters == nil {
		return CharacterJobMastery{}, errors.New("character repository not configured")
	}
	char, err := s.characters.FindByID(ctx, characterID)
	if err != nil {
		return CharacterJobMastery{}, err
	}

	jobState, err := s.repository.FindByCharacterID(ctx, characterID)
	if err != nil {
		jobState, _ = corejob.NewCharacterJob(characterID, char.JobID)
	}

	definitions := s.ListDefinitions()
	jobsProgress := make([]JobProgress, 0, TotalMasteryJobs)

	masteredCount := 0
	learningCount := 0
	unlearnedCount := 0

	// Check if all 72 completion jobs are mastered
	allJobsMastered := jobState.AllJobsMastered || jobState.HasMasteredAllCompletionJobs()

	for _, def := range definitions {
		if def.ID == "" || def.ID == "starter" {
			continue
		}

		masterSP := s.masterySP(def)
		isMastered := jobState.IsMastered(def.ID) || (char.JobID == def.ID && char.SP >= masterSP && masterSP > 0)

		var status JobStatus
		var currentSP int

		if isMastered {
			status = JobStatusMastered
			if char.JobID == def.ID {
				currentSP = char.SP
			} else if sp, ok := jobState.MasteredSP(def.ID); ok && sp > 0 {
				currentSP = sp
			} else if char.OldJobID == def.ID && char.OldSP > 0 {
				currentSP = char.OldSP
			} else {
				currentSP = masterSP
			}
			masteredCount++
		} else {
			isLearning := (char.JobID == def.ID && def.ID != "starter") ||
				(char.OldJobID == def.ID && def.ID != "starter") ||
				slices.ContainsFunc(jobState.History, func(ch corejob.Change) bool {
					return ch.FromJobID == def.ID || ch.ToJobID == def.ID
				})

			if isLearning {
				status = JobStatusLearning
				if char.JobID == def.ID {
					currentSP = char.SP
				} else if char.OldJobID == def.ID {
					currentSP = char.OldSP
				} else {
					currentSP = 0
				}
				learningCount++
			} else {
				status = JobStatusUnlearned
				currentSP = 0
				unlearnedCount++
			}
		}

		jobsProgress = append(jobsProgress, JobProgress{
			JobID:     def.ID,
			JobName:   def.Name,
			Status:    status,
			CurrentSP: currentSP,
			MasterSP:  masterSP,
		})
	}

	totalRealJobs := len(jobsProgress)
	masteryPercentage := CalculateMasteryPercentage(masteredCount, learningCount, totalRealJobs)

	completionTitle := ""
	if allJobsMastered {
		completionTitle = CompletionJobMasterTitle
	}

	return CharacterJobMastery{
		CharacterID:       characterID,
		MasteryPercentage: masteryPercentage,
		MasteredCount:     masteredCount,
		LearningCount:     learningCount,
		UnlearnedCount:    unlearnedCount,
		AllJobsMastered:   allJobsMastered,
		CompletionTitle:   completionTitle,
		Jobs:              jobsProgress,
	}, nil
}
