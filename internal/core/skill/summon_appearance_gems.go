package skill

import (
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/core/random"
)

type SummonAppearanceGemEntry struct {
	ID                string
	SourceID, CMPCost int
}

// SummonAppearanceGemEntries retains intrinsic identities and outer CMP metadata
// from ff97fc5:_data.cgi:2170,2188–2193. The consumer owns charging.
func SummonAppearanceGemEntries() []SummonAppearanceGemEntry {
	return []SummonAppearanceGemEntry{{"gem-82", 82, 18}, {"gem-100-appearance", 100, 0}}
}

// RegisterSummonAppearanceGems binds the existing bodies and pure alternatives.
// Rebuild operation ports from frozen stage/appearance facts after reload.
func (d *BattleDefinitions) RegisterSummonAppearanceGems(roster *corebattle.RosterContext, appearance *corebattle.TransformationContext) {
	if d.Invocations.Entries == nil {
		d.Invocations.Entries = make(map[string]corebattle.CallbackDefinition)
	}
	if d.RosterSkills == nil {
		d.RosterSkills = make(map[string]bool)
	}
	d.RosterSkills["gem-82"] = true
	summon := d.RosterInvocation(roster, "gem-82")
	summon.ProjectBranches = func(s *corebattle.LifetimeState, f *corebattle.CallbackContext) ([]corebattle.CallbackContinuation, error) {
		if roster == nil || f.AfterEffects {
			return nil, corebattle.ErrUnsupportedEffect
		}
		return s.ProjectEnemySummon(roster, f)
	}
	d.Invocations.Entries["gem-82"] = summon
	d.Invocations.Entries["gem-100-appearance"] = corebattle.CallbackDefinition{
		Validate: func(s *corebattle.LifetimeState, f *corebattle.CallbackContext) error {
			if appearance == nil || f.AfterEffects {
				return corebattle.ErrUnsupportedEffect
			}
			child := *appearance
			child.TargetContext = f.TargetContext
			return s.ValidateTransformation(&child, "gem-100-appearance", "", random.Default())
		},
		ProjectBranches: func(s *corebattle.LifetimeState, f *corebattle.CallbackContext) ([]corebattle.CallbackContinuation, error) {
			if appearance == nil || f.AfterEffects {
				return nil, corebattle.ErrUnsupportedEffect
			}
			return s.ProjectAppearanceGem(appearance, f)
		},
		Execute: func(s *corebattle.LifetimeState, f *corebattle.CallbackContext, rng random.Generator) error {
			if appearance == nil {
				return corebattle.ErrUnsupportedEffect
			}
			appearance.TargetContext = f.TargetContext
			_, err := s.ApplyTransformation(appearance, "gem-100-appearance", "", rng)
			f.TargetContext = appearance.TargetContext
			return err
		},
	}
}
