package adventure

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

var (
	ErrInvalidStage              = errors.New("stage definition is invalid")
	ErrStageNotFound             = errors.New("stage definition not found")
	ErrLevelRequirementNotMet    = errors.New("character level requirement not met for stage")
	ErrJobLevelRequirementNotMet = errors.New("character job level requirement not met for stage")
)

// Stage defines an adventure destination with required levels, normal monsters, and boss encounters.
type Stage struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	MinLevel   int      `json:"min_level"`
	MonsterIDs []string `json:"monster_ids"`
	BossIDs    []string `json:"boss_ids,omitempty"`
}

// GetBossIDs returns the stage boss IDs (for Floor 10).
// If not explicitly defined, the last monster in MonsterIDs is designated as the stage boss.
func (s Stage) GetBossIDs() []string {
	if len(s.BossIDs) > 0 {
		return s.BossIDs
	}
	if len(s.MonsterIDs) > 0 {
		return []string{s.MonsterIDs[len(s.MonsterIDs)-1]}
	}
	return nil
}

// GetNormalMonsterIDs returns normal monster IDs (for Floors 1 to 9), excluding stage bosses.
func (s Stage) GetNormalMonsterIDs() []string {
	bosses := s.GetBossIDs()
	bossMap := make(map[string]bool, len(bosses))
	for _, b := range bosses {
		bossMap[b] = true
	}
	var normal []string
	for _, m := range s.MonsterIDs {
		if !bossMap[m] {
			normal = append(normal, m)
		}
	}
	if len(normal) == 0 {
		return s.MonsterIDs
	}
	return normal
}

type stageJSON struct {
	ID              string   `json:"id"`
	Name            string   `json:"name"`
	MinLevel        int      `json:"min_level"`
	MonsterIDs      []string `json:"monster_ids"`
	BossIDs         []string `json:"boss_ids,omitempty"`
	DurationSeconds int      `json:"duration_seconds,omitempty"` // legacy compat, purged from domain
}

// NewStage creates a Stage definition without duration (purging the fictional timer).
func NewStage(id, name string, minLevel int, monsterIDs []string) (Stage, error) {
	return NewStageWithBosses(id, name, minLevel, monsterIDs, nil)
}

// NewStageWithBosses creates a Stage definition with explicit stage boss monster IDs.
func NewStageWithBosses(id, name string, minLevel int, monsterIDs []string, bossIDs []string) (Stage, error) {
	id = strings.TrimSpace(id)
	name = strings.TrimSpace(name)
	if id == "" || name == "" || minLevel < 1 || len(monsterIDs) == 0 {
		return Stage{}, ErrInvalidStage
	}
	cleanedMonsters := make([]string, 0, len(monsterIDs))
	for _, m := range monsterIDs {
		m = strings.TrimSpace(m)
		if m == "" {
			return Stage{}, ErrInvalidStage
		}
		cleanedMonsters = append(cleanedMonsters, m)
	}
	cleanedBosses := make([]string, 0, len(bossIDs))
	for _, b := range bossIDs {
		b = strings.TrimSpace(b)
		if b != "" {
			cleanedBosses = append(cleanedBosses, b)
		}
	}
	return Stage{
		ID:         id,
		Name:       name,
		MinLevel:   minLevel,
		MonsterIDs: cleanedMonsters,
		BossIDs:    cleanedBosses,
	}, nil
}

type StageCatalog struct {
	stages map[string]Stage
}

func NewStageCatalog(stages []Stage) (*StageCatalog, error) {
	catalog := &StageCatalog{stages: make(map[string]Stage, len(stages))}
	for _, s := range stages {
		s.ID = strings.TrimSpace(s.ID)
		s.Name = strings.TrimSpace(s.Name)
		if s.ID == "" || s.Name == "" || s.MinLevel < 1 || len(s.MonsterIDs) == 0 {
			return nil, ErrInvalidStage
		}
		if _, exists := catalog.stages[s.ID]; exists {
			return nil, ErrInvalidStage
		}
		catalog.stages[s.ID] = s
	}
	return catalog, nil
}

func (c *StageCatalog) FindByID(id string) (Stage, error) {
	if c == nil {
		return Stage{}, ErrStageNotFound
	}
	s, ok := c.stages[id]
	if !ok {
		return Stage{}, ErrStageNotFound
	}
	return s, nil
}

func (c *StageCatalog) Stages() []Stage {
	if c == nil {
		return nil
	}
	values := make([]Stage, 0, len(c.stages))
	for _, s := range c.stages {
		values = append(values, s)
	}
	sort.Slice(values, func(i, j int) bool {
		return values[i].ID < values[j].ID
	})
	return values
}

// RequiredJobLevel returns the required job level (reincarnation count / 転職回数)
// to challenge a stage, in 1:1 parity with legacy quest.cgi.
func RequiredJobLevel(stageID string) int {
	var num int
	_, _ = fmt.Sscanf(stageID, "stage-%d", &num)
	switch num {
	case 1, 2:
		return 0
	case 23: // legacy stage 22
		return 6
	case 25: // legacy stage 24
		return 50
	case 26: // legacy stage 25
		return 10
	default:
		if num >= 3 && num <= 15 {
			return num - 2
		}
		if num > 15 {
			return 14
		}
		return 0
	}
}

// CanAccessStage checks whether a character meets level and job-level criteria for a stage.
func (c *StageCatalog) CanAccessStage(char corecharacter.Character, stageID string) error {
	st, err := c.FindByID(stageID)
	if err != nil {
		return ErrStageNotFound
	}
	if char.Level < st.MinLevel {
		return ErrLevelRequirementNotMet
	}
	if char.JobLevel < RequiredJobLevel(st.ID) {
		return ErrJobLevelRequirementNotMet
	}
	return nil
}

//go:embed data/stages.json
var stagesCatalogData []byte

func InitialStageCatalog() (*StageCatalog, error) {
	var rawList []stageJSON
	if err := json.Unmarshal(stagesCatalogData, &rawList); err != nil {
		return nil, fmt.Errorf("decode stage catalog: %w", err)
	}
	stages := make([]Stage, 0, len(rawList))
	for _, raw := range rawList {
		stage, err := NewStageWithBosses(raw.ID, raw.Name, raw.MinLevel, raw.MonsterIDs, raw.BossIDs)
		if err != nil {
			return nil, fmt.Errorf("invalid stage %s: %w", raw.ID, err)
		}
		stages = append(stages, stage)
	}
	return NewStageCatalog(stages)
}
