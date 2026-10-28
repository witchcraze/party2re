package battle

import (
	"slices"

	"github.com/witchcraze/party2re/internal/core/random"
)

// ValidateOrdinaryTool is the discovery and execution gate. It never calls
// Apply ports, consumes arguments, mutates results or draws committed RNG.
func (s *LifetimeState) ValidateOrdinaryTool(c *OrdinaryToolContext, id string, rng random.Generator) error {
	if s == nil || c == nil {
		return ErrInvalidRequest
	}
	if err := s.validateTargetContext(c.TargetContext, rng); err != nil {
		return err
	}
	if inspectionEntry(id) {
		if id == "job-9-entry-6" || id == "job-16-entry-1" {
			return nil
		}
		return s.validateInspection(c)
	}
	found := false
	for _, entry := range OrdinaryToolEntries() {
		found = found || entry.ID == id
	}
	if !found {
		return ErrUnsupportedEffect
	}
	p, child := s.callbackCopy(), *c
	child.Arguments = slices.Clone(c.Arguments)
	if power := ordinaryToolMP(id); power != 0 {
		return p.validatePartyTarget(c.TargetContext, controlEffect(child.toolTarget(), power, CategoryTool), rng)
	}
	if power := ordinaryToolHealing(id); power != 0 {
		if id == "tool-4" || id == "tool-5" {
			for _, target := range c.PartyIDs {
				if err := p.validatePartyTarget(c.TargetContext, controlEffect(target, power, CategoryTool), rng); err != nil {
					return err
				}
			}
			return nil
		}
		return p.validatePartyTarget(c.TargetContext, controlEffect(child.toolTarget(), power, CategoryTool), rng)
	}
	if id == "tool-41" || id == "tool-42" || id == "tool-75" || id == "tool-77" || id == "tool-82" || id == "tool-213" {
		if len(c.EnemyIDs) == 0 {
			return ErrNoBattleTarget
		}
	}
	if tmp := ordinaryToolTemporary(id); tmp != "" {
		e := controlSymbol(child.toolTarget(), string(tmp), CategoryTool)
		if id == "tool-82" {
			_, err := p.controlTargets(&ControlClimaxContext{DamageContext: child.DamageContext}, e, false)
			return err
		}
		return p.validatePartyTarget(c.TargetContext, e, rng)
	}
	switch id {
	case "tool-7", "tool-8", "tool-9", "tool-10":
		e := controlSymbol(child.toolTarget(), StatusDeadlyPoison, CategoryTool)
		if id == "tool-8" {
			e.Payload.Symbol = StatusParalyze
		} else if id == "tool-9" {
			e.Payload.Symbol = StatusConfusion
		} else if id == "tool-10" {
			// Current-state cure resolves twice; the first hook can restore MP
			// but cannot create a seal for the tool-category second helper.
			e.Payload.Symbol = "cure"
		}
		return p.validatePartyTarget(c.TargetContext, e, rng)
	case "tool-41":
		return p.ValidateDamage(&child.DamageContext, dualAgilityEffect(child.toolTarget(), 80, CategoryTool, true), rng)
	case "tool-42":
		return p.validateAreaDamage(&child.DamageContext, dualAgilityEffect("", 60, CategoryTool, true), rng)
	case "tool-76":
		return p.ValidateStatUp(&child.EffectContext, controlEffect(child.toolTarget(), 1, CategoryTool), StatAttack, rng)
	case "tool-79":
		return p.ValidateStatUp(&child.EffectContext, controlEffect(child.toolTarget(), .3, CategoryTool), StatDefense, rng)
	case "tool-75", "tool-77", "tool-86":
		if err := p.validateToolCursor(c.Cursor); err != nil {
			return err
		}
		e := controlSymbol(c.Cursor.ActorID, "tension", CategoryTool)
		var targets []string
		if id == "tool-86" {
			if err := p.validatePartyTarget(c.TargetContext, e, rng); err != nil {
				return err
			}
			for _, candidate := range p.partyTargetCandidates(c.TargetContext, e, nil) {
				targets = append(targets, candidate.TargetID)
			}
		} else {
			var err error
			targets, err = p.controlTargets(&ControlClimaxContext{DamageContext: child.DamageContext}, e, false)
			if err != nil {
				return err
			}
		}
		if id != "tool-75" {
			for _, target := range targets {
				t := p.actor(target)
				if target == "" {
					// tenshon checks binding on the absent record before self.
					actor := *p.actor(c.ActorID)
					actor.Status = ""
					t = &actor
				}
				if _, err := p.gemTensionCeiling(&child.EffectContext, t, 1); err != nil {
					return err
				}
			}
		}
		return nil
	case "tool-87":
		if c.EquipmentReset == nil {
			return ErrUnsupportedEffect
		}
		reset := *c.EquipmentReset
		reset.TargetContext = c.TargetContext
		for i := range p.Actors {
			a := &p.Actors[i]
			if err := p.ValidateEquipmentReset(&reset, a.ID, rng); err != nil {
				return err
			}
			// item188's pre-reset revival can expose living-only reset hooks.
			a.HP = max(1, a.HP)
			if err := p.ValidateEquipmentReset(&reset, a.ID, rng); err != nil {
				return err
			}
		}
		return nil
	case "tool-130":
		return nil
	case "tool-151":
		for _, target := range c.EnemyIDs[:min(10, len(c.EnemyIDs))] {
			if err := c.validateInspectionContent(target); err != nil {
				return err
			}
		}
		return nil
	case "tool-213":
		if c.RequesterMaxHP == nil || *c.RequesterMaxHP < 0 {
			return ErrUnsupportedEffect
		}
		_, err := p.controlTargets(&ControlClimaxContext{DamageContext: child.DamageContext}, controlSymbol(child.toolTarget(), "capture", CategoryTool), false)
		return err
	}
	return ErrUnsupportedEffect
}

func (s *LifetimeState) validateToolCursor(cursor TargetCursor) error {
	switch cursor.Source {
	case "":
		if cursor.ActorID == "" {
			return nil
		}
	case "item150", "item159":
		if s.actor(cursor.ActorID) != nil {
			return nil
		}
	}
	return ErrUnsupportedEffect
}
