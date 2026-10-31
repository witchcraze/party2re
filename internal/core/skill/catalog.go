package skill

import (
	"slices"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

// RepresentativeDefinitions returns the certified representative player-job skill
// definitions (#1188 / #1409):
// 5.3 single-ally magic heal30 (SP6/MP3),
// 6.1 fixed single-enemy magic damage20 (SP1/MP2), and
// 6.3 fixed all-enemy magic damage25 (SP8/MP5).
func RepresentativeDefinitions() []Definition {
	return []Definition{
		{
			ID:             "job-5-entry-3",
			Name:           "heal-30",
			RequiredJobIDs: []string{"job-05"},
			RequiredSP:     6,
			MPCost:         3,
			Effect:         corebattle.Effect{Kind: corebattle.ActionKindHeal, Power: 30},
		},
		{
			ID:             "job-6-entry-1",
			Name:           "magic-20",
			RequiredJobIDs: []string{"job-06"},
			RequiredSP:     1,
			MPCost:         2,
			Effect:         corebattle.Effect{Kind: corebattle.ActionKindAttack, Power: 20},
		},
		{
			ID:             "job-6-entry-3",
			Name:           "magic-wave-25",
			RequiredJobIDs: []string{"job-06"},
			RequiredSP:     8,
			MPCost:         5,
			Effect:         corebattle.Effect{Kind: corebattle.ActionKindAttack, Power: 25},
		},
	}
}

// Catalog holds registered skill definitions and provides lookup by canonical job ID.
type Catalog struct {
	definitions []Definition
}

// NewCatalog constructs a Catalog with the given definitions.
func NewCatalog(definitions []Definition) *Catalog {
	return &Catalog{
		definitions: append([]Definition(nil), definitions...),
	}
}

// InitialCatalog returns the default production skill catalog populated with representative definitions.
func InitialCatalog() *Catalog {
	return NewCatalog(RepresentativeDefinitions())
}

// SkillsForJob returns all skill definitions unlocked for the given canonical job ID.
func (c *Catalog) SkillsForJob(jobID string) []Definition {
	if c == nil {
		return nil
	}
	var res []Definition
	for _, def := range c.definitions {
		if slices.Contains(def.RequiredJobIDs, jobID) {
			res = append(res, def)
		}
	}
	return res
}

// FindByID returns the definition matching the given ID.
func (c *Catalog) FindByID(id string) (Definition, error) {
	if c == nil {
		return Definition{}, ErrDefinitionNotFound
	}
	for _, def := range c.definitions {
		if def.ID == id {
			return def, nil
		}
	}
	return Definition{}, ErrDefinitionNotFound
}

// Definitions returns a shallow copy of all registered definitions.
func (c *Catalog) Definitions() []Definition {
	if c == nil {
		return nil
	}
	return append([]Definition(nil), c.definitions...)
}

// RegisterRepresentativeSkills populates BattleDefinitions.JobSkills with certified
// native ActionSkills for the representative entries.
func (d *BattleDefinitions) RegisterRepresentativeSkills() {
	if d.JobSkills == nil {
		d.JobSkills = make(map[string]corebattle.ActionSkill)
	}
	d.JobSkills["job-5-entry-3"] = corebattle.ActionSkill{
		Kind:        corebattle.ActionKindHeal,
		Power:       30,
		TargetScope: corebattle.TargetScopeSingleAlly,
		Category:    string(corebattle.CategoryMagic),
	}
	d.JobSkills["job-6-entry-1"] = corebattle.ActionSkill{
		Kind:        corebattle.ActionKindAttack,
		Power:       20,
		TargetScope: corebattle.TargetScopeSingleEnemy,
		Category:    string(corebattle.CategoryMagic),
		Direct:      true,
	}
	d.JobSkills["job-6-entry-3"] = corebattle.ActionSkill{
		Kind:        corebattle.ActionKindAttack,
		Power:       25,
		TargetScope: corebattle.TargetScopeAllEnemies,
		Category:    string(corebattle.CategoryMagic),
		Direct:      true,
	}
}

// ProductionBattleDefinitions returns BattleDefinitions with production-certified native skill actions.
func ProductionBattleDefinitions() BattleDefinitions {
	var d BattleDefinitions
	d.RegisterRepresentativeSkills()
	return d
}
