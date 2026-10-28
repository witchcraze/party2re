package battle

import (
	"slices"
	"strings"

	"github.com/witchcraze/party2re/internal/core/random"
)

// InspectionContent freezes the encounter owner's resolved reward definition
// and source treasure predicate. RewardKind1/2 are weapon/armor; others are tools.
// Empty is independent of treasure status; ordinary enemies also have rewards.
type InspectionContent struct {
	Treasure     bool   `json:"treasure"`
	Empty        bool   `json:"empty"`
	RewardKind   int    `json:"reward_kind"`
	DefinitionID string `json:"definition_id,omitempty"`
	Price        int    `json:"price"`
}

// InspectionFact contains presentation-neutral observations, never loot grants.
// Flavor is an index0..7; valuation Band0..5 has lower bounds0/1000/5000/
// 10000/15000/20000. Stats are current resources and equipment-free baselines.
type InspectionFact struct {
	Kind         string          `json:"kind"`
	TargetID     string          `json:"target_id,omitempty"`
	RewardKind   int             `json:"reward_kind,omitempty"`
	Empty        bool            `json:"empty,omitempty"`
	Flavor       int             `json:"flavor,omitempty"`
	Band         int             `json:"band,omitempty"`
	DefinitionID string          `json:"definition_id,omitempty"`
	Found        bool            `json:"found,omitempty"`
	HP           int             `json:"hp,omitempty"`
	MaxHP        int             `json:"max_hp,omitempty"`
	MP           int             `json:"mp,omitempty"`
	MaxMP        int             `json:"max_mp,omitempty"`
	BaseStats    BattleBaseStats `json:"base_stats"`
}

// InspectionEntries contributes positional metadata without inventing partial
// job tables. The full ordered tables belong to the job-table assembly owner.
func InspectionEntries() []CallbackEntry {
	return []CallbackEntry{
		{ID: "job-7-entry-4", Name: "member-content-inspection", RequiredSP: 20},
		{ID: "job-9-entry-4", Name: "member-content-inspection", RequiredSP: 20},
		{ID: "job-9-entry-6", Name: "member-stat-inspection", RequiredSP: 50, MPCost: 1},
		{ID: "job-16-entry-1", Name: "member-stat-inspection", RequiredSP: 10, MPCost: 2},
		{ID: "job-78-entry-1", Name: "reward-valuation-inspection", RequiredSP: 15},
		{ID: "job-81-entry-1", Name: "member-content-inspection", RequiredSP: 30},
	}
}

func inspectionEntry(id string) bool {
	for _, entry := range InspectionEntries() {
		if entry.ID == id {
			return true
		}
	}
	return false
}

func (s *LifetimeState) validateInspection(c *OrdinaryToolContext) error {
	for _, a := range s.Actors {
		if err := c.validateInspectionContent(a.ID); err != nil {
			return err
		}
	}
	return nil
}

func (c *OrdinaryToolContext) validateInspectionContent(target string) error {
	content, ok := c.Inspection[target]
	if !ok || content.Price < 0 || content.RewardKind < 0 || (!content.Empty && content.DefinitionID == "") {
		return ErrUnsupportedEffect
	}
	return nil
}

func inspectionBand(price int) int {
	band := 0
	for _, threshold := range []int{1000, 5000, 10000, 15000, 20000} {
		if price >= threshold {
			band++
		}
	}
	return band
}

func (s *LifetimeState) inspectMember(c *OrdinaryToolContext, target string, valuation bool, rng random.Generator) InspectionFact {
	if s.actor(target) == nil {
		target = s.randomMember(rng)
	}
	content := c.Inspection[target]
	fact := InspectionFact{TargetID: target}
	if content.Treasure {
		fact.Kind = "content"
		if valuation {
			fact.Kind, fact.Band = "valuation", inspectionBand(content.Price)
			if content.Empty {
				fact.Band = 0
			}
		} else {
			fact.Empty = content.Empty
			if !content.Empty {
				fact.DefinitionID, fact.RewardKind = content.DefinitionID, content.RewardKind
			}
		}
	} else {
		fact.Kind, fact.Flavor = "scent", int(rng.Float64()*8)
		if valuation {
			fact.Kind = "character"
		}
	}
	return fact
}

// ApplyInspection shares tool151's content/flavor facts without transferring
// assets. Job78 tests treasure first, then nonparty search, then member fallback.
func (s *LifetimeState) ApplyInspection(c *OrdinaryToolContext, id string, rng random.Generator) error {
	if !inspectionEntry(id) {
		return ErrUnsupportedEffect
	}
	if err := s.ValidateOrdinaryTool(c, id, rng); err != nil {
		return err
	}
	target := c.shift()
	var fact InspectionFact
	switch id {
	case "job-9-entry-6", "job-16-entry-1":
		if s.actor(target) == nil {
			target = s.randomMember(rng)
		}
		a := s.actor(target)
		fact = InspectionFact{Kind: "stats", TargetID: target, HP: a.HP, MaxHP: a.MaxHP, MP: a.MP, MaxMP: a.MaxMP, BaseStats: a.BaseStats}
	case "job-78-entry-1":
		if c.Inspection[target].Treasure {
			fact = s.inspectMember(c, target, true, rng)
		} else if !slices.Contains(c.PartyIDs, target) && strings.TrimSpace(target) != "" {
			fact = InspectionFact{Kind: "reward-search", DefinitionID: target}
			for _, enemy := range c.EnemyIDs {
				content := c.Inspection[enemy]
				if !content.Empty && content.DefinitionID == target {
					fact.Found, fact.TargetID = true, enemy
					break
				}
			}
		} else {
			fact = s.inspectMember(c, target, true, rng)
		}
	default:
		fact = s.inspectMember(c, target, false, rng)
	}
	c.Results = append(c.Results, OrdinaryToolResult{ID: id, Facts: []InspectionFact{fact}})
	return nil
}
