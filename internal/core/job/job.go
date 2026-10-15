package job

import (
	"errors"
	"slices"
	"strings"
)

var (
	ErrInvalidDefinition    = errors.New("job definition is invalid")
	ErrInvalidCharacter     = errors.New("character job state is invalid")
	ErrJobUnavailable       = errors.New("job requirements are not met")
	ErrCharacterJobNotFound = errors.New("character job not found")
)

const (
	MinimumChangeLevel = 20
	CompletionJobCount = 72
	SuppinJobID        = "job-73"
)

// IsCompletionJob reports whether jobID is one of the original 72 jobs (job-01 .. job-72)
// that contribute to the all-job mastery completion condition.
func IsCompletionJob(jobID string) bool {
	if !strings.HasPrefix(jobID, "job-") {
		return false
	}
	numStr := strings.TrimPrefix(jobID, "job-")
	if len(numStr) != 2 {
		return false
	}
	var n int
	for _, ch := range numStr {
		if ch < '0' || ch > '9' {
			return false
		}
		n = n*10 + int(ch-'0')
	}
	return n >= 1 && n <= CompletionJobCount
}

type Definition struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	HPGrowth       int      `json:"hp_growth"`
	MPGrowth       int      `json:"mp_growth"`
	AttackGrowth   int      `json:"attack_growth"`
	DefenseGrowth  int      `json:"defense_growth"`
	AgilityGrowth  int      `json:"agility_growth"`
	RequiredGender string   `json:"required_gender,omitempty"`
	RequiredJobIDs []string `json:"required_job_ids,omitempty"`
	RequiredItemID string   `json:"required_item_id,omitempty"`
	MasterySP      int      `json:"mastery_sp,omitempty"`
	CMPTier        int      `json:"c_mp_tier"`
}

func NewDefinition(id, name string, hp, mp, attack, defense, agility, cmpTier int, gender string) (Definition, error) {
	if strings.TrimSpace(id) == "" || strings.TrimSpace(name) == "" || hp < 0 || mp < 0 ||
		attack < 0 || defense < 0 || agility < 0 || cmpTier < 0 || cmpTier > 5 {
		return Definition{}, ErrInvalidDefinition
	}
	return Definition{
		ID: id, Name: name, HPGrowth: hp, MPGrowth: mp, AttackGrowth: attack,
		DefenseGrowth: defense, AgilityGrowth: agility, RequiredGender: gender, CMPTier: cmpTier,
	}, nil
}

type Change struct {
	FromJobID string `json:"from_job_id"`
	ToJobID   string `json:"to_job_id"`
}

type CharacterJob struct {
	CharacterID     string         `json:"character_id"`
	CurrentJobID    string         `json:"current_job_id"`
	History         []Change       `json:"history"`
	MasteredJobs    []string       `json:"mastered_jobs"`
	MasteredJobSP   map[string]int `json:"mastered_job_sp"`
	RecordedJobSP   map[string]int `json:"recorded_job_sp,omitempty"` // Known SP does not prove a job record exists.
	RecordedJobIDs  []string       `json:"recorded_job_ids,omitempty"`
	AllJobsMastered bool           `json:"all_jobs_mastered"`
}

// RecordJobSP retains the latest departure/save value, including decreases and
// unmastered jobs. Mastery notifications remain RecordMastery's responsibility.
// Call only when writing an actual job record, not when preserving imported SP.
func (c *CharacterJob) RecordJobSP(jobID string, sp int) error {
	if c == nil || c.CharacterID == "" || strings.TrimSpace(jobID) == "" || sp < 0 {
		return ErrInvalidCharacter
	}
	jobID = strings.TrimSpace(jobID)
	if c.RecordedJobSP == nil {
		c.RecordedJobSP = make(map[string]int)
	}
	c.RecordedJobSP[jobID] = sp
	if !slices.Contains(c.RecordedJobIDs, jobID) {
		c.RecordedJobIDs = append(c.RecordedJobIDs, jobID)
	}
	if c.IsMastered(jobID) {
		if c.MasteredJobSP == nil {
			c.MasteredJobSP = make(map[string]int)
		}
		c.MasteredJobSP[jobID] = sp
	}
	return nil
}

// SavedSP prefers the latest recorded value, retaining compatibility with
// mastery-only snapshots created before independent learned-job storage.
func (c CharacterJob) SavedSP(jobID string) (int, bool) {
	jobID = strings.TrimSpace(jobID)
	if sp, ok := c.RecordedJobSP[jobID]; ok {
		return sp, true
	}
	sp, ok := c.MasteredJobSP[jobID]
	return sp, ok
}

// HasRecordedJob requires an actual record, independently of preserved SP.
func (c CharacterJob) HasRecordedJob(jobID string) bool {
	jobID = strings.TrimSpace(jobID)
	if jobID == "" {
		return false
	}
	if slices.Contains(c.RecordedJobIDs, jobID) || c.IsMastered(jobID) {
		return true
	}
	return slices.ContainsFunc(c.History, func(change Change) bool { return change.FromJobID == jobID })
}

func NewCharacterJob(characterID, currentJobID string) (CharacterJob, error) {
	if strings.TrimSpace(characterID) == "" || strings.TrimSpace(currentJobID) == "" {
		return CharacterJob{}, ErrInvalidCharacter
	}
	return CharacterJob{CharacterID: characterID, CurrentJobID: currentJobID, MasteredJobSP: make(map[string]int)}, nil
}

func (c *CharacterJob) ChangeTo(target Definition, level int, gender string) error {
	if c == nil || c.CharacterID == "" || c.CurrentJobID == "" {
		return ErrInvalidCharacter
	}
	if target.ID == "" || target.ID == "starter" || level < MinimumChangeLevel ||
		(target.RequiredGender != "" && target.RequiredGender != gender) {
		return ErrJobUnavailable
	}
	if target.ID == SuppinJobID && !c.AllJobsMastered {
		return ErrJobUnavailable
	}
	if target.ID != c.CurrentJobID {
		c.History = append(c.History, Change{FromJobID: c.CurrentJobID, ToJobID: target.ID})
		c.CurrentJobID = target.ID
	}
	return nil
}

// RestoreCurrentJob sets the active job identifier without validating level or
// recording history transitions (e.g. when recalling a future memory snapshot).
func (c *CharacterJob) RestoreCurrentJob(jobID string) {
	if c == nil || strings.TrimSpace(jobID) == "" {
		return
	}
	c.CurrentJobID = strings.TrimSpace(jobID)
}

func (c *CharacterJob) Master(jobID string) {
	if c == nil || strings.TrimSpace(jobID) == "" {
		return
	}
	jobID = strings.TrimSpace(jobID)
	if slices.Contains(c.MasteredJobs, jobID) {
		return
	}
	c.MasteredJobs = append(c.MasteredJobs, jobID)
	if c.MasteredJobSP == nil {
		c.MasteredJobSP = make(map[string]int)
	}
}

// RecordMastery records the first mastery and reports whether to notify.
// RecordJobSP separately refreshes retained SP on departure or future save.
func (c *CharacterJob) RecordMastery(jobID string, sp, masterySP int) bool {
	if c == nil || strings.TrimSpace(jobID) == "" || sp < 0 || masterySP <= 0 || sp < masterySP {
		return false
	}
	if c.IsMastered(jobID) {
		return false
	}
	c.Master(jobID)
	c.MasteredJobSP[jobID] = sp
	return true
}

// MasteredSP returns the retained SP for a mastered job.
func (c *CharacterJob) MasteredSP(jobID string) (int, bool) {
	if c == nil || c.MasteredJobSP == nil {
		return 0, false
	}
	value, ok := c.MasteredJobSP[strings.TrimSpace(jobID)]
	return value, ok
}

// MasteredJobCount returns the number of distinct mastered jobs.
func (c *CharacterJob) MasteredJobCount() int {
	if c == nil {
		return 0
	}
	return len(c.MasteredJobs)
}

// HasMasteredAll reports whether the required number of jobs is mastered.
func (c *CharacterJob) HasMasteredAll(required int) bool {
	return required > 0 && c.MasteredJobCount() >= required
}

func (c *CharacterJob) IsMastered(jobID string) bool {
	if c == nil {
		return false
	}
	return slices.Contains(c.MasteredJobs, strings.TrimSpace(jobID))
}

// MasteredCompletionJobCount returns the number of distinct jobs mastered among
// the original 72 completion jobs (job-01 to job-72).
func (c *CharacterJob) MasteredCompletionJobCount() int {
	if c == nil {
		return 0
	}
	count := 0
	for _, jobID := range c.MasteredJobs {
		if IsCompletionJob(jobID) {
			count++
		}
	}
	return count
}

// HasMasteredAllCompletionJobs reports whether all 72 completion jobs have been mastered.
func (c *CharacterJob) HasMasteredAllCompletionJobs() bool {
	return c.MasteredCompletionJobCount() >= CompletionJobCount
}

// CheckAndSetAllJobsMastered checks if all 72 completion jobs are mastered.
// If so and AllJobsMastered is not yet set, it sets AllJobsMastered to true and returns true (indicating newly completed).
func (c *CharacterJob) CheckAndSetAllJobsMastered() bool {
	if c == nil || c.AllJobsMastered {
		return false
	}
	if c.HasMasteredAllCompletionJobs() {
		c.AllJobsMastered = true
		return true
	}
	return false
}
