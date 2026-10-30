package adventure

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

var (
	ErrInvalidStage              = errors.New("stage definition is invalid")
	ErrStageNotFound             = errors.New("stage definition not found")
	ErrLevelRequirementNotMet    = errors.New("character level requirement not met for stage")
	ErrJobLevelRequirementNotMet = errors.New("character job level requirement not met for stage")
	ErrStageNotAvailable         = errors.New("stage is not currently available")
)

var jstZone = time.FixedZone("JST", 9*60*60)

// SeasonalStageData defines monster encounters and loot pools for a seasonal variant of Stage 26.
type SeasonalStageData struct {
	Season          string   `json:"season"`
	BossIDs         []string `json:"boss_ids"`
	MonsterIDs      []string `json:"monster_ids"`
	TreasureWeapons []string `json:"treasure_weapons"`
	TreasureArmors  []string `json:"treasure_armors"`
	TreasureItems   []string `json:"treasure_items"`
}

// Stage defines an adventure destination with required levels, normal monsters, boss encounters, and treasure pools.
type Stage struct {
	ID              string              `json:"id"`
	Name            string              `json:"name"`
	MinLevel        int                 `json:"min_level"`
	MonsterIDs      []string            `json:"monster_ids"`
	BossIDs         []string            `json:"boss_ids,omitempty"`
	TreasureWeapons []string            `json:"treasure_weapons,omitempty"`
	TreasureArmors  []string            `json:"treasure_armors,omitempty"`
	TreasureItems   []string            `json:"treasure_items,omitempty"`
	Seasons         []SeasonalStageData `json:"seasons,omitempty"`
}

// ForSeason returns a copy of the Stage configured for the given seasonal variant.
func (s Stage) ForSeason(season string) Stage {
	for _, sd := range s.Seasons {
		if strings.EqualFold(sd.Season, season) {
			cp := s
			cp.MonsterIDs = sd.MonsterIDs
			cp.BossIDs = sd.BossIDs
			cp.TreasureWeapons = sd.TreasureWeapons
			cp.TreasureArmors = sd.TreasureArmors
			cp.TreasureItems = sd.TreasureItems
			return cp
		}
	}
	return s
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
	ID              string              `json:"id"`
	Name            string              `json:"name"`
	MinLevel        int                 `json:"min_level"`
	MonsterIDs      []string            `json:"monster_ids"`
	BossIDs         []string            `json:"boss_ids,omitempty"`
	TreasureWeapons []string            `json:"treasure_weapons,omitempty"`
	TreasureArmors  []string            `json:"treasure_armors,omitempty"`
	TreasureItems   []string            `json:"treasure_items,omitempty"`
	Seasons         []SeasonalStageData `json:"seasons,omitempty"`
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
	stages  map[string]Stage
	nowFunc func() time.Time
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

func (c *StageCatalog) now() time.Time {
	if c != nil && c.nowFunc != nil {
		return c.nowFunc()
	}
	return time.Now()
}

// SetNowFunc configures a custom time source for catalog access checks.
func (c *StageCatalog) SetNowFunc(fn func() time.Time) {
	if c != nil {
		c.nowFunc = fn
	}
}

func parseStageNum(stageID string) (int, bool) {
	var num int
	if _, err := fmt.Sscanf(stageID, "stage-%d", &num); err == nil {
		return num, true
	}
	if _, err := fmt.Sscanf(stageID, "%d", &num); err == nil {
		return num, true
	}
	return 0, false
}

// IsStageInSeason reports whether a seasonal stage is currently in season at time t in JST.
// Non-seasonal stages always return true.
//
// Legacy discrepancy note (party2/lib/quest.cgi @ ff97fc549932a146cc945d7edf71b44c868a840e):
// In original Party2 CGI, seasonal conditions (is_winter / is_halloween) were checked exclusively
// when populating the stage selection UI (sub tsukuru / get_limited_stages:1213-1222), while the
// server-side creation handler (check_create_quest:474-568) omitted seasonal verification.
// Consequently, direct requests could historically initiate off-season adventures.
// In party2re, the intended selection UI rules are adopted as the unified canonical standard
// across observation (AvailableStages) and admission (CanAccessStage / StartPartyAdventure).
func IsStageInSeason(stageID string, t time.Time) bool {
	num, ok := parseStageNum(stageID)
	if !ok {
		return true
	}
	inJST := t.In(jstZone)
	month := inJST.Month()

	switch num {
	case 23, 25:
		// Winter only: December, January, February (lib/quest.cgi:1215-1216, 1310-1315 is_winter)
		return month == time.December || month == time.January || month == time.February
	case 27:
		// Halloween only: October (lib/quest.cgi:1218, 1317-1322 is_halloween)
		return month == time.October
	default:
		return true
	}
}

func (c *StageCatalog) FindByID(id string) (Stage, error) {
	if c == nil {
		return Stage{}, ErrStageNotFound
	}
	s, ok := c.stages[id]
	if !ok {
		var num int
		if _, err := fmt.Sscanf(id, "stage-%d", &num); err == nil {
			formatted := fmt.Sprintf("stage-%02d", num)
			if s2, ok2 := c.stages[formatted]; ok2 {
				return s2, nil
			}
		} else if _, err := fmt.Sscanf(id, "%d", &num); err == nil {
			formatted := fmt.Sprintf("stage-%02d", num)
			if s2, ok2 := c.stages[formatted]; ok2 {
				return s2, nil
			}
		}
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
// to challenge a stage.
//
// Legacy discrepancy note (party2/lib/quest.cgi @ ff97fc549932a146cc945d7edf71b44c868a840e):
// Original Party2 CGI contained divergences between UI stage display and server admission:
//   - Normal stages 0–14: sub tsukuru:220-223 and check_create_quest:527-528 align at stage <= job_lv + 2
//     (stages 0–2 require JobLevel 0, stages 3–14 require JobLevel >= stage - 2).
//   - Stage 22: sub tsukuru:208 (@ex_stage) requires JobLevel 6, but check_create_quest bypassed the check
//     via exclude_job_lv_stage:1265-1274.
//   - Stage 23: get_limited_stages:1215 requires JobLevel 0 in winter, and was also in exclude_job_lv_stage.
//   - Stage 24: sub tsukuru:208 (@ex_stage) requires JobLevel 50, passing server check when >= 50.
//   - Stage 25: get_limited_stages:1216 requires JobLevel 10 in winter, but check_create_quest bypassed the check
//     via exclude_job_lv_stage.
//   - Stage 27: get_limited_stages:1218 requires JobLevel 0 in October, but check_create_quest omitted 27 from
//     exclude_job_lv_stage, inadvertently demanding JobLevel >= 25 (27 > job_lv + 2).
//
// Following Issue #1194, party2re adopts the intended selection UI thresholds as the canonical access
// rules for both observation (AvailableStages) and execution admission (CanAccessStage).
func RequiredJobLevel(stageID string) int {
	num, ok := parseStageNum(stageID)
	if !ok {
		return 0
	}
	switch num {
	case 0, 1:
		return 0
	case 22:
		return 6
	case 23:
		return 0
	case 24:
		return 50
	case 25:
		return 10
	case 26, 27:
		return 0
	default:
		if num >= 2 && num <= 14 {
			return num - 2
		}
		if num > 14 {
			return 14
		}
		return 0
	}
}

// CanAccessStage checks whether a character meets level, job-level, and seasonal criteria for a stage at current time.
func (c *StageCatalog) CanAccessStage(char corecharacter.Character, stageID string) error {
	return c.CanAccessStageAt(char, stageID, c.now())
}

// CanAccessStageAt checks whether a character meets level, job-level, and seasonal criteria for a stage at the given time.
func (c *StageCatalog) CanAccessStageAt(char corecharacter.Character, stageID string, t time.Time) error {
	st, err := c.FindByID(stageID)
	if err != nil {
		return ErrStageNotFound
	}
	if !IsStageInSeason(st.ID, t) {
		return ErrStageNotAvailable
	}
	if char.Level < st.MinLevel {
		return ErrLevelRequirementNotMet
	}
	if char.JobLevel < RequiredJobLevel(st.ID) {
		return ErrJobLevelRequirementNotMet
	}
	return nil
}

// AvailableStages returns all stages accessible by the character at current time, sorted by ID.
func (c *StageCatalog) AvailableStages(char corecharacter.Character) []Stage {
	return c.AvailableStagesAt(char, c.now())
}

// AvailableStagesAt returns all stages accessible by the character at time t, sorted by ID.
func (c *StageCatalog) AvailableStagesAt(char corecharacter.Character, t time.Time) []Stage {
	all := c.Stages()
	available := make([]Stage, 0, len(all))
	for _, st := range all {
		if err := c.CanAccessStageAt(char, st.ID, t); err == nil {
			available = append(available, st)
		}
	}
	return available
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
		stage.TreasureWeapons = raw.TreasureWeapons
		stage.TreasureArmors = raw.TreasureArmors
		stage.TreasureItems = raw.TreasureItems
		stage.Seasons = raw.Seasons
		stages = append(stages, stage)
	}
	return NewStageCatalog(stages)
}
