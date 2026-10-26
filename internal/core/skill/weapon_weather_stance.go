package skill

import (
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/core/random"
)

// RegisterWeaponWeatherStance contributes full ordered tables, binding21
// bodies and preserving the separately owned wallet28.3 binding if supplied.
func (d *BattleDefinitions) RegisterWeaponWeatherStance(c *corebattle.WeaponWeatherStanceContext) {
	if d.Invocations.Entries == nil {
		d.Invocations.Entries = make(map[string]corebattle.CallbackDefinition)
	}
	if d.Invocations.JobTables == nil {
		d.Invocations.JobTables = make(map[int]corebattle.CallbackTable)
	}
	for _, job := range []int{24, 27, 28} {
		entries := corebattle.WeaponWeatherStanceEntries(job)
		d.Invocations.JobTables[job] = corebattle.CallbackTable{Variants: [][]corebattle.CallbackEntry{entries}}
		for _, entry := range entries {
			if entry.ID != "job-28-entry-3" {
				d.Invocations.Entries[entry.ID] = weaponWeatherStanceInvocation(c, entry.ID)
			}
		}
	}
}

func weaponWeatherStanceInvocation(c *corebattle.WeaponWeatherStanceContext, id string) corebattle.CallbackDefinition {
	context := func(f *corebattle.CallbackContext) (corebattle.WeaponWeatherStanceContext, error) {
		if c == nil {
			return corebattle.WeaponWeatherStanceContext{}, corebattle.ErrUnsupportedEffect
		}
		child := *c
		child.TargetContext, child.Arguments = f.TargetContext, append([]string(nil), f.Arguments...)
		return child, nil
	}
	return corebattle.CallbackDefinition{
		Validate: func(s *corebattle.LifetimeState, f *corebattle.CallbackContext) error {
			child, err := context(f)
			if err != nil {
				return err
			}
			if f.AfterEffects && id != "job-24-entry-5" && id != "job-27-entry-5" && id != "job-28-entry-0" && id != "job-28-entry-2" && id != "job-28-entry-4" {
				return corebattle.ErrUnsupportedEffect
			}
			return s.ValidateWeaponWeatherStance(&child, id, random.Default())
		},
		Project: func(s *corebattle.LifetimeState, f *corebattle.CallbackContext) (bool, error) {
			child, err := context(f)
			if err != nil {
				return false, err
			}
			return s.ProjectWeaponWeatherStance(&child, id)
		},
		Execute: func(s *corebattle.LifetimeState, f *corebattle.CallbackContext, rng random.Generator) error {
			if c == nil {
				return corebattle.ErrUnsupportedEffect
			}
			c.TargetContext, c.Arguments = f.TargetContext, append([]string(nil), f.Arguments...)
			err := s.ApplyWeaponWeatherStance(c, id, rng)
			f.TargetContext, f.Arguments = c.TargetContext, c.Arguments
			return err
		},
	}
}
