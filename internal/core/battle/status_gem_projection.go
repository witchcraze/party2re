package battle

import (
	"math"

	"github.com/witchcraze/party2re/internal/core/random"
)

type StatusContinuation struct {
	State   LifetimeState
	Context StatusContext
}

// validateDeathGem enumerates gem71's per-enemy death outcomes so each later
// target is checked after earlier absorption, revival and defeat facts.
func (s *LifetimeState) validateDeathGem(c *StatusContext) error {
	if s == nil || c == nil {
		return ErrInvalidRequest
	}
	if err := s.validateTargetContext(c.TargetContext, random.Default()); err != nil {
		return err
	}
	e, chance := statusEntryEffect(71, "")
	branches := []StatusContinuation{{State: s.callbackCopy(), Context: *cloneCallbackContext(CallbackContext{Status: c}).Status}}
	// The shared budget also counts each distinct final outcome.
	results := statusContinuations{remaining: c.admissionBudget}
	if branches[0].State.actor(c.ActorID).categorySealed(e.Category) {
		if err := results.add(branches[0].State, branches[0].Context); err != nil {
			return err
		}
		branches[0].State.actor(c.ActorID).Status = ""
	}
	for index, target := range c.EnemyIDs {
		e.TargetID = target
		next := statusContinuations{remaining: c.admissionBudget}
		for _, branch := range branches {
			children, err := branch.State.projectGemStatus(branch.Context, e, chance, true, index+1 < len(c.EnemyIDs))
			if err != nil {
				return err
			}
			for _, child := range children {
				if err := next.add(child.State, child.Context); err != nil {
					return err
				}
			}
		}
		branches = next.branches
	}
	for _, branch := range branches {
		if err := results.add(branch.State, branch.Context); err != nil {
			return err
		}
	}
	return nil
}

func (s *LifetimeState) projectGemStatus(c StatusContext, e TargetEffect, chance float64, death, followed bool) ([]StatusContinuation, error) {
	target := c.TargetContext
	target.symbolicConversion = true
	targets, err := s.projectSymbolicTargets(target, e, true)
	if err != nil {
		return nil, err
	}
	results := statusContinuations{remaining: c.admissionBudget}
	for _, branch := range targets {
		child := c
		r, p := branch.Resolution, branch.State
		if !r.Resolved || r.Payload.Symbol == "" || p.actor(r.TargetID).HP <= 0 {
			if err := results.add(p, child); err != nil {
				return nil, err
			}
			continue
		}
		probability := TargetEffect{Category: r.Category, Payload: CallbackPayload{Number: chance}}
		power := p.finalEffectPower(&child.EffectContext, p.initialEffectPower(&child.EffectContext, probability, ModifierStatus))
		if !death && p.Fields.Current() == "den" {
			power = math.Trunc(power * 1.2)
		}
		t := p.actor(r.TargetID)
		if statusImmune(t, false, death, p.Fields.Current()) || (!death && power <= 0) {
			if err := results.add(p, child); err != nil {
				return nil, err
			}
			continue
		}
		if t.Gear == nil {
			return nil, ErrUnsupportedEffect
		}
		if power < 100 || t.Gear.WeaponID == "weapon-68" && t.Gear.ArmorID == "armor-53" {
			if err := results.add(p, child); err != nil {
				return nil, err
			}
		}
		if !death {
			if hasItem(t.ItemDefinitionIDs, "item-161") && t.Tension >= 5 {
				if err := results.add(p, child); err != nil {
					return nil, err
				}
				continue
			}
			t.Status, t.Tension = r.Payload.Symbol, 1
			if err := results.add(p, child); err != nil {
				return nil, err
			}
			continue
		}
		if child.Defeat == nil {
			return nil, ErrUnsupportedEffect
		}
		defeat := *child.Defeat
		defeat.TargetContext = child.TargetContext
		states, err := p.projectDefeatBranches(&defeat, t.ID, random.Default(), map[string]bool{}, followed)
		if err != nil {
			return nil, err
		}
		for _, state := range states {
			next := child
			next.Defeat = &state.Context
			if err := results.add(state.State, next); err != nil {
				return nil, err
			}
		}
	}
	return results.branches, nil
}
