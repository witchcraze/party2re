package battle

import (
	"slices"

	"github.com/witchcraze/party2re/internal/core/random"
)

// ValidateHighPower admits all reachable descendants without applying external
// ports, changing shared facts, consuming arguments, or drawing committed RNG.
func (s *LifetimeState) ValidateHighPower(c *HighPowerContext, id string, rng random.Generator) error {
	if s == nil || c == nil {
		return ErrInvalidRequest
	}
	if err := s.validateTargetContext(c.TargetContext, rng); err != nil {
		return err
	}
	known := false
	for _, job := range []int{47, 48, 49, 52} {
		for _, entry := range HighPowerEntries(job) {
			known = known || entry.ID == id
		}
	}
	if !known {
		return ErrUnsupportedEffect
	}
	p, child := s.callbackCopy(), *c
	child.Arguments = slices.Clone(c.Arguments)
	a := p.actor(c.ActorID)
	e := dualAgilityEffect("", float64(a.Attack)*1.2, CategoryPhysical, false)
	switch id {
	case "job-47-entry-0", "job-47-entry-1", "job-47-entry-2", "job-49-entry-2", "job-52-entry-0":
		e.TargetID = child.shift()
		if id == "job-47-entry-1" || id == "job-49-entry-2" {
			e.Payload.Number = float64(a.Attack) * 1.6
		}
		if id == "job-47-entry-2" {
			e.Payload.Number = float64(a.Attack) * 1.8
		}
		return p.ValidateDamage(&child.DamageContext, e, rng)
	case "job-47-entry-3", "job-48-entry-3", "job-52-entry-2":
		category := CategoryPhysical
		if id == "job-48-entry-3" {
			category = CategoryMagic
		}
		if a.categorySealed(category) {
			a.Status = ""
		}
		context := ControlClimaxContext{DamageContext: child.DamageContext}
		targets, err := p.controlTargets(&context, controlSymbol(child.shift(), "collapse", category), false)
		if err != nil {
			return err
		}
		for _, target := range targets {
			if target == "" {
				continue
			}
			q, nested := p.callbackCopy(), child
			t := q.actor(target)
			switch id {
			case "job-47-entry-3":
				if err := q.ValidateDamage(&nested.DamageContext, dualAgilityEffect(target, float64(min(400, t.MaxHP-t.HP+10)), CategoryPhysical, true), rng); err != nil {
					return err
				}
			case "job-52-entry-2":
				t.Temporary, t.Tension, t.Defense = "", 1, t.BaseStats.Defense
				if err := q.validateControlDown(&ControlClimaxContext{DamageContext: nested.DamageContext}, controlEffect(target, .1, CategoryPhysical), rng); err != nil {
					return err
				}
			}
		}
	case "job-47-entry-4":
		if a.HP <= 0 {
			return nil
		}
		e.TargetID, e.Payload.Number = "", float64(a.Attack)
		return p.validateRepeatedDamage(&child.DamageContext, e, 4, rng)
	case "job-47-entry-5":
		return nil
	case "job-48-entry-0", "job-48-entry-1":
		child.AdditionalEffect = true
		power, chance, status := 120.0, 40.0, StatusDeadlyPoison
		if id == "job-48-entry-1" {
			power, chance, status = 70, 30, StatusSleep
		}
		return p.validateAreaDamageStatus(&child.DamageContext, power, status, chance, rng)
	case "job-48-entry-2":
		child.AdditionalEffect = true
		e = dualAgilityEffect(child.shift(), 300, CategoryMagic, true)
		if err := p.admitDamageSequence(&child.DamageContext, []TargetEffect{e}, rng); err != nil {
			return err
		}
		// Stat-down has no external descendants, but its second category
		// conversion and resource contract still need the shared validation.
		for _, target := range child.EnemyIDs {
			if err := p.validateControlDown(&ControlClimaxContext{DamageContext: child.DamageContext}, controlEffect(target, .08, CategoryMagic), rng); err != nil {
				return err
			}
		}
	case "job-48-entry-4":
		return p.validateAreaDamage(&child.DamageContext, e, rng)
	case "job-48-entry-5":
		if c.EquipmentReset == nil {
			return ErrUnsupportedEffect
		}
		reset := *c.EquipmentReset
		reset.TargetContext = c.TargetContext
		for _, member := range p.Actors {
			if err := p.ValidateEquipmentReset(&reset, member.ID, rng); err != nil {
				return err
			}
		}
	case "job-49-entry-0":
		return p.validatePartyTarget(child.TargetContext, controlSymbol(child.shift(), "cure", CategoryUntyped), rng)
	case "job-49-entry-1":
		return p.ValidateStatUp(&child.EffectContext, controlEffect(child.ActorID, .6, CategoryPhysical), StatDefense, rng)
	case "job-52-entry-1":
		return p.ValidateStatUp(&child.EffectContext, controlEffect(child.ActorID, 1, CategoryPhysical), StatAttack, rng)
	case "job-52-entry-3":
		return p.ValidateControlClimax(&ControlClimaxContext{DamageContext: child.DamageContext}, "job-58-entry-4", rng)
	case "job-52-entry-4":
		return p.validateAreaDamage(&child.DamageContext, dualAgilityEffect("", 200, CategoryPhysical, true), rng)
	}
	return nil
}

func (s *LifetimeState) ProjectHighPower(c *HighPowerContext, id string) (bool, error) {
	if err := s.ValidateHighPower(c, id, random.Default()); err != nil {
		return false, err
	}
	switch id {
	case "job-47-entry-5":
		s.highPowerWeapon(c)
		return true, nil
	case "job-48-entry-5":
		// Equipment callbacks can draw or change the package target cursor.
		// Without their exact transition, keep the existing unknown-effect gate.
		for _, a := range s.Actors {
			if a.Gear.AccessoryID != "" || a.Gear.WeaponID == "weapon-71" {
				return false, nil
			}
		}
		p, child := s.callbackCopy(), *c
		reset := *c.EquipmentReset
		child.EquipmentReset = &reset
		if err := p.highPowerReset(&child, resetBoundary(0)); err != nil {
			return false, err
		}
		*s = p
		return true, nil
	}
	return false, nil
}
