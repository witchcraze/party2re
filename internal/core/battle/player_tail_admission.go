package battle

import (
	"math"
	"slices"

	"github.com/witchcraze/party2re/internal/core/random"
)

func (s *LifetimeState) validateReachedPlayerTail(c *PlayerTailContext, id string, rng random.Generator) error {
	a := s.actor(c.ActorID)
	switch id {
	case "item-210-revival-tail":
		seen := make(map[string]bool)
		for _, target := range c.Item210Fallen {
			if !slices.Contains(c.PartyIDs, target) || seen[target] {
				return ErrInvalidParticipant
			}
			seen[target] = true
		}
	case "item-202-combat-tail", "item-198-combat-tail":
		// Reuse the complete tool tuple gate on an isolated state and a private
		// boundary generator. These helpers invoke no external hooks/counters.
		p := s.callbackCopy()
		power := float64(a.MaxHP) * .04
		if id == "item-198-combat-tail" {
			power = float64(a.MaxMP) * .02
		}
		_, err := p.ApplyToolHealing(c.TargetContext, a.ID, power, true, resetBoundary(0))
		return err
	case "weapon-69-armor-54-tail", "weapon-70-armor-55-tail":
		if a.Tension < 2 {
			_, err := s.gemTensionCeiling(&c.EffectContext, a, 1)
			return err
		}
	case "cmp-recharge-tail":
		if a.CMPCharge >= a.CMPNeedCharge && (c.RequesterLevel == nil || *c.RequesterLevel < 0) {
			return ErrUnsupportedEffect
		}
	case "item-194-combat-tail":
		return s.validateResetTail(c, rng)
	case "frenzy-tail":
		return s.validateFrenzyTail(c, rng)
	}
	return nil
}

func (s *LifetimeState) validateResetTail(c *PlayerTailContext, rng random.Generator) error {
	if c.DefeatContext == nil {
		return ErrUnsupportedEffect
	}
	reset := c.DefeatContext.EquipmentResetContext
	reset.TargetContext, reset.Cursor = c.TargetContext, c.Cursor
	states, cursor, err := s.projectEquipmentReset(reset, c.ActorID, rng)
	if err != nil {
		return err
	}
	a := s.actor(c.ActorID)
	for _, p := range states {
		t := p.actor(a.ID)
		difference := t.Attack + t.Defense + t.Agility - a.Attack - a.Defense - a.Agility
		if difference > 0 {
			damage, defeat := c.DamageContext, *c.DefeatContext
			defeat.EquipmentResetContext = reset
			defeat.Cursor = cursor
			e := dualAgilityEffect("", float64(difference)*.5, CategoryUntyped, true)
			if _, err := p.projectCallbackDamage(&damage, &defeat, e, e.Payload.Number, true, rng); err != nil {
				return err
			}
		} else if difference < 0 {
			if err := p.validatePartyTarget(c.TargetContext, controlEffect(a.ID, float64(-difference)*.5, CategoryUntyped), rng); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *LifetimeState) validateFrenzyTail(c *PlayerTailContext, rng random.Generator) error {
	a := s.actor(c.ActorID)
	power := float64(min(a.BaseStats.Attack*5, a.Attack+int(float64(a.BaseStats.Attack)*.6)))
	if err := s.validateRecovery(&c.EffectContext, controlEffect("", power, CategoryUntyped), rng); err != nil {
		return err
	}
	// The dedicated tool hit chooses from ALL ordered members, including fallen
	// ones and self. Each alternative gets its own defeat projection.
	for _, target := range s.Actors {
		if target.Temporary == TemporaryOneHitDefense || (power == 0 && target.HP > 0) {
			continue
		}
		if err := c.ToolCategory.validate(); err != nil {
			return err
		}
		if target.Gear == nil {
			return ErrUnsupportedEffect
		}
		maximum := rollDamage(power, toolDefense(&target), false, target.BaseStats.Defense, resetBoundary(math.Nextafter(1, 0)))
		if (power*.5-toolDefense(&target))*.9 < 1 {
			maximum = max(2, maximum)
		}
		if target.HP <= maximum {
			if c.DefeatContext == nil {
				return ErrUnsupportedEffect
			}
			p, child := s.callbackCopy(), *c.DefeatContext
			p.actor(a.ID).Attack = int(power)
			p.actor(target.ID).HP = 0
			child.TargetContext, child.Cursor = c.TargetContext, c.Cursor
			if err := p.ValidateDefeat(&child, target.ID, rng); err != nil {
				return err
			}
		}
	}
	return nil
}
