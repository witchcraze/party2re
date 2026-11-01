package skill

import (
	"slices"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/core/random"
)

// RegisterDefenseGems binds complete bodies, independently of scalar Gems.
// Rebuild these operation-local closures after loading serialized battle facts.
func (d *BattleDefinitions) RegisterDefenseGems() {
	if d.DefenseGems == nil {
		d.DefenseGems = make(map[string]bool)
	}
	if d.Invocations.Entries == nil {
		d.Invocations.Entries = make(map[string]corebattle.CallbackDefinition)
	}
	for _, e := range corebattle.DefenseGemEntries() {
		d.DefenseGems[e.ID] = true
		d.Invocations.Entries[e.ID] = d.DefenseGemInvocation([]string{e.ID})
	}
}

// DefenseGemAction shares discovery/submission's complete ordered-slot gate.
// Returned CMP contributions are charged by the consumer after admission.
func (d BattleDefinitions) DefenseGemAction(s *corebattle.LifetimeState, c corebattle.TargetContext, ids []string, targetID string, rng random.Generator) ([]corebattle.DefenseGemEntry, error) {
	if len(ids) > 3 {
		return nil, corebattle.ErrUnsupportedEffect
	}
	var entries []corebattle.DefenseGemEntry
	var sources []int
	for _, id := range ids {
		if id == "" {
			continue
		}
		if !d.DefenseGems[id] {
			return nil, corebattle.ErrUnsupportedEffect
		}
		found := false
		for _, e := range corebattle.DefenseGemEntries() {
			if e.ID == id {
				entries, sources = append(entries, e), append(sources, e.SourceID)
				found = true
				break
			}
		}
		if !found {
			return nil, corebattle.ErrUnsupportedEffect
		}
	}
	if err := s.ValidateDefenseGems(sources, c, targetID, rng); err != nil {
		return nil, err
	}
	return entries, nil
}

// DefenseGemInvocation freezes one to three slots with the same outer target.
// Unknown preceding callbacks need their own transition projection; no body
// may certify an arbitrary changed actor/roster through AfterEffects.
func (d BattleDefinitions) DefenseGemInvocation(ids []string) corebattle.CallbackDefinition {
	ids = slices.Clone(ids)
	bind := func(s *corebattle.LifetimeState, f *corebattle.CallbackContext, rng random.Generator) ([]int, string, error) {
		if f.AfterEffects {
			return nil, "", corebattle.ErrUnsupportedEffect
		}
		target := ""
		if len(f.Arguments) > 0 {
			target = f.Arguments[0]
		}
		entries, err := d.DefenseGemAction(s, f.TargetContext, ids, target, rng)
		var sources []int
		for _, e := range entries {
			sources = append(sources, e.SourceID)
		}
		return sources, target, err
	}
	consume := func(f *corebattle.CallbackContext, sources []int) {
		e := corebattle.DefenseGemEntries()
		for _, entry := range e {
			if entry.SourceID == sources[0] && len(f.Arguments) > 0 && (f.Custom || len(sources) > 1 || entry.TargetScope == corebattle.TargetScopeSingleAlly) {
				f.Arguments = f.Arguments[1:]
				break
			}
		}
	}
	return corebattle.CallbackDefinition{
		Validate: func(s *corebattle.LifetimeState, f *corebattle.CallbackContext) error {
			_, _, err := bind(s, f, random.Default())
			return err
		},
		Project: func(s *corebattle.LifetimeState, f *corebattle.CallbackContext) (bool, error) {
			sources, target, err := bind(s, f, random.Default())
			if err != nil {
				return false, err
			}
			known, err := s.ProjectDefenseGems(sources, f.TargetContext, target)
			if known {
				consume(f, sources)
			}
			return known, err
		},
		Execute: func(s *corebattle.LifetimeState, f *corebattle.CallbackContext, rng random.Generator) error {
			sources, target, err := bind(s, f, rng)
			if err != nil {
				return err
			}
			_, err = s.ApplyDefenseGems(sources, f.TargetContext, target, rng)
			if err == nil {
				consume(f, sources)
			}
			return err
		},
	}
}
