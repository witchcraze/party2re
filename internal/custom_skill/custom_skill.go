package custom_skill

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	corejob "github.com/witchcraze/party2re/internal/core/job"
)

//go:embed data/skills.json
var skillsData []byte

var (
	ErrSkillNotFound       = errors.New("skill not found in catalog")
	ErrSlotOutOfBounds     = errors.New("skill slot index out of bounds")
	ErrSkillNotLearned     = errors.New("skill has not been learned or job not mastered")
	ErrLevelTooLow         = errors.New("character level is too low for this skill")
	ErrDuplicateSkillEquip = errors.New("skill is already equipped in another slot")
	ErrCharacterNotFound   = errors.New("character not found")
	ErrInvalidPriority     = errors.New("skill priority must be between 1 and 10")
)

const (
	DefaultMaxSlots = 4
	MinPriority     = 1
	MaxPriority     = 10
)

type SkillEntry struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	RequiredJobID string `json:"required_job_id,omitempty"`
	RequiredLevel int    `json:"required_level"`
	MPCost        int    `json:"mp_cost"`
	Power         int    `json:"power"`
	Kind          string `json:"kind"` // "damage", "healing", "buff", "debuff"
	Description   string `json:"description"`
}

type EquippedSkillSlot struct {
	SlotIndex int    `json:"slot_index"`
	SkillID   string `json:"skill_id"`
	SkillName string `json:"skill_name"`
	Priority  int    `json:"priority"`
	MPCost    int    `json:"mp_cost"`
	Power     int    `json:"power"`
	Kind      string `json:"kind"`
}

type CharacterSkillLoadout struct {
	CharacterID string              `json:"character_id"`
	MaxSlots    int                 `json:"max_slots"`
	Slots       []EquippedSkillSlot `json:"slots"`
	UpdatedAt   time.Time           `json:"updated_at"`
}

type CharacterProvider interface {
	FindByID(ctx context.Context, id string) (corecharacter.Character, error)
}

type CharacterJobProvider interface {
	FindByCharacterID(ctx context.Context, characterID string) (corejob.CharacterJob, error)
}

type Repository interface {
	SaveLoadout(ctx context.Context, loadout CharacterSkillLoadout) error
	FindLoadout(ctx context.Context, characterID string) (*CharacterSkillLoadout, error)
}

type Service struct {
	repo        Repository
	charRepo    CharacterProvider
	jobRepo     CharacterJobProvider
	catalog     map[string]SkillEntry
	catalogList []SkillEntry
}

func NewService(repo Repository, charRepo CharacterProvider, jobRepo CharacterJobProvider) (*Service, error) {
	if repo == nil {
		return nil, errors.New("custom skill repository is required")
	}
	if charRepo == nil {
		return nil, errors.New("character provider is required")
	}

	catalog, list, err := loadCatalog()
	if err != nil {
		return nil, fmt.Errorf("load custom skill catalog: %w", err)
	}

	return &Service{
		repo:        repo,
		charRepo:    charRepo,
		jobRepo:     jobRepo,
		catalog:     catalog,
		catalogList: list,
	}, nil
}

func loadCatalog() (map[string]SkillEntry, []SkillEntry, error) {
	var entries []SkillEntry
	if err := json.Unmarshal(skillsData, &entries); err != nil {
		return nil, nil, fmt.Errorf("decode skills JSON: %w", err)
	}

	catalog := make(map[string]SkillEntry, len(entries))
	for _, e := range entries {
		if e.ID == "" {
			return nil, nil, fmt.Errorf("skill entry has empty id")
		}
		catalog[e.ID] = e
	}

	list := make([]SkillEntry, len(entries))
	copy(list, entries)
	sort.Slice(list, func(i, j int) bool {
		return list[i].ID < list[j].ID
	})

	return catalog, list, nil
}

func (s *Service) ListCatalog() []SkillEntry {
	return s.catalogList
}

func (s *Service) GetSkill(skillID string) (*SkillEntry, error) {
	entry, ok := s.catalog[skillID]
	if !ok {
		return nil, ErrSkillNotFound
	}
	return &entry, nil
}

func (s *Service) GetLoadout(ctx context.Context, characterID string) (*CharacterSkillLoadout, error) {
	if strings.TrimSpace(characterID) == "" {
		return nil, errors.New("character id is required")
	}

	loadout, err := s.repo.FindLoadout(ctx, characterID)
	if err != nil {
		return nil, err
	}
	if loadout == nil {
		return &CharacterSkillLoadout{
			CharacterID: characterID,
			MaxSlots:    DefaultMaxSlots,
			Slots:       []EquippedSkillSlot{},
			UpdatedAt:   time.Now().UTC(),
		}, nil
	}
	return loadout, nil
}

func (s *Service) GetAvailableSkills(ctx context.Context, characterID string) ([]SkillEntry, error) {
	char, err := s.charRepo.FindByID(ctx, characterID)
	if err != nil {
		return nil, ErrCharacterNotFound
	}

	charLevel := char.Level
	if charLevel <= 0 {
		charLevel = 1
	}

	var currentJob string
	var masteredJobs []string
	if s.jobRepo != nil {
		cj, err := s.jobRepo.FindByCharacterID(ctx, characterID)
		if err == nil {
			currentJob = cj.CurrentJobID
			masteredJobs = cj.MasteredJobs
		}
	}
	if currentJob == "" {
		currentJob = char.JobID
	}

	var available []SkillEntry
	for _, skill := range s.catalogList {
		if charLevel < skill.RequiredLevel {
			continue
		}
		if skill.RequiredJobID == "" {
			available = append(available, skill)
			continue
		}
		if skill.RequiredJobID == currentJob || contains(masteredJobs, skill.RequiredJobID) {
			available = append(available, skill)
		}
	}

	return available, nil
}

func (s *Service) EquipSkill(ctx context.Context, characterID string, slotIndex int, skillID string, priority int) (*CharacterSkillLoadout, error) {
	skill, err := s.GetSkill(skillID)
	if err != nil {
		return nil, err
	}

	if priority < MinPriority || priority > MaxPriority {
		return nil, ErrInvalidPriority
	}

	char, err := s.charRepo.FindByID(ctx, characterID)
	if err != nil {
		return nil, ErrCharacterNotFound
	}

	charLevel := char.Level
	if charLevel <= 0 {
		charLevel = 1
	}
	if charLevel < skill.RequiredLevel {
		return nil, ErrLevelTooLow
	}

	// Verify job mastery / access
	if skill.RequiredJobID != "" {
		var currentJob string
		var masteredJobs []string
		if s.jobRepo != nil {
			cj, err := s.jobRepo.FindByCharacterID(ctx, characterID)
			if err == nil {
				currentJob = cj.CurrentJobID
				masteredJobs = cj.MasteredJobs
			}
		}
		if currentJob == "" {
			currentJob = char.JobID
		}

		if skill.RequiredJobID != currentJob && !contains(masteredJobs, skill.RequiredJobID) {
			return nil, ErrSkillNotLearned
		}
	}

	loadout, err := s.GetLoadout(ctx, characterID)
	if err != nil {
		return nil, err
	}

	if slotIndex < 1 || slotIndex > loadout.MaxSlots {
		return nil, ErrSlotOutOfBounds
	}

	// Check if already equipped in another slot
	for _, slot := range loadout.Slots {
		if slot.SkillID == skillID && slot.SlotIndex != slotIndex {
			return nil, ErrDuplicateSkillEquip
		}
	}

	// Update or insert slot
	newSlot := EquippedSkillSlot{
		SlotIndex: slotIndex,
		SkillID:   skill.ID,
		SkillName: skill.Name,
		Priority:  priority,
		MPCost:    skill.MPCost,
		Power:     skill.Power,
		Kind:      skill.Kind,
	}

	replaced := false
	for i, slot := range loadout.Slots {
		if slot.SlotIndex == slotIndex {
			loadout.Slots[i] = newSlot
			replaced = true
			break
		}
	}
	if !replaced {
		loadout.Slots = append(loadout.Slots, newSlot)
	}

	sort.Slice(loadout.Slots, func(i, j int) bool {
		return loadout.Slots[i].SlotIndex < loadout.Slots[j].SlotIndex
	})
	loadout.UpdatedAt = time.Now().UTC()

	if err := s.repo.SaveLoadout(ctx, *loadout); err != nil {
		return nil, err
	}

	return loadout, nil
}

func (s *Service) UnequipSlot(ctx context.Context, characterID string, slotIndex int) (*CharacterSkillLoadout, error) {
	loadout, err := s.GetLoadout(ctx, characterID)
	if err != nil {
		return nil, err
	}

	if slotIndex < 1 || slotIndex > loadout.MaxSlots {
		return nil, ErrSlotOutOfBounds
	}

	var filtered []EquippedSkillSlot
	for _, slot := range loadout.Slots {
		if slot.SlotIndex != slotIndex {
			filtered = append(filtered, slot)
		}
	}
	loadout.Slots = filtered
	loadout.UpdatedAt = time.Now().UTC()

	if err := s.repo.SaveLoadout(ctx, *loadout); err != nil {
		return nil, err
	}

	return loadout, nil
}

func contains(slice []string, target string) bool {
	for _, s := range slice {
		if s == target {
			return true
		}
	}
	return false
}

func EncodeJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func DecodeJSON[T any](data string) (T, error) {
	var target T
	err := json.Unmarshal([]byte(data), &target)
	return target, err
}
