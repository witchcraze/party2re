package adventure

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

var (
	ErrInvalidStage  = errors.New("stage definition is invalid")
	ErrStageNotFound = errors.New("stage definition not found")
)

type Stage struct {
	ID         string        `json:"id"`
	Name       string        `json:"name"`
	MinLevel   int           `json:"min_level"`
	MonsterIDs []string      `json:"monster_ids"`
	Duration   time.Duration `json:"-"`
}

type stageJSON struct {
	ID              string   `json:"id"`
	Name            string   `json:"name"`
	MinLevel        int      `json:"min_level"`
	MonsterIDs      []string `json:"monster_ids"`
	DurationSeconds int      `json:"duration_seconds"`
}

func NewStage(id, name string, minLevel int, monsterIDs []string, duration time.Duration) (Stage, error) {
	id = strings.TrimSpace(id)
	name = strings.TrimSpace(name)
	if id == "" || name == "" || minLevel < 1 || len(monsterIDs) == 0 || duration <= 0 {
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
	return Stage{
		ID:         id,
		Name:       name,
		MinLevel:   minLevel,
		MonsterIDs: cleanedMonsters,
		Duration:   duration,
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
		if s.ID == "" || s.Name == "" || s.MinLevel < 1 || len(s.MonsterIDs) == 0 || s.Duration <= 0 {
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

//go:embed data/stages.json
var stagesCatalogData []byte

func InitialStageCatalog() (*StageCatalog, error) {
	var rawList []stageJSON
	if err := json.Unmarshal(stagesCatalogData, &rawList); err != nil {
		return nil, fmt.Errorf("decode stage catalog: %w", err)
	}
	stages := make([]Stage, 0, len(rawList))
	for _, raw := range rawList {
		duration := time.Duration(raw.DurationSeconds) * time.Second
		if raw.DurationSeconds <= 0 {
			duration = time.Hour
		}
		stage, err := NewStage(raw.ID, raw.Name, raw.MinLevel, raw.MonsterIDs, duration)
		if err != nil {
			return nil, fmt.Errorf("invalid stage %s: %w", raw.ID, err)
		}
		stages = append(stages, stage)
	}
	return NewStageCatalog(stages)
}
