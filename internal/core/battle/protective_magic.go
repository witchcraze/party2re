package battle

import "github.com/witchcraze/party2re/internal/core/random"

// ProtectiveMagicContext retains operation facts; consumers rebuild ports after reload.
type ProtectiveMagicContext struct {
	DamageContext
	Arguments []string
	CureCalls int
}

// ProtectiveMagicEntries includes the separately owned revival32.4/33.6 bodies.
func ProtectiveMagicEntries(job int) []CallbackEntry {
	switch job {
	case 32:
		return []CallbackEntry{
			{ID: "job-32-entry-0", Name: "variable-magic-strike", RequiredSP: 5, MPCost: 5},
			{ID: "job-32-entry-1", Name: "party-heal-50", RequiredSP: 25, MPCost: 10},
			{ID: "job-32-entry-2", Name: "party-physical-barrier", RequiredSP: 50, MPCost: 20},
			{ID: "job-32-entry-3", Name: "party-magic-reflection", RequiredSP: 70, MPCost: 30},
			{ID: "job-32-entry-4", Name: "転生の炎", RequiredSP: 100, MPCost: 40},
			{ID: "job-32-entry-5", Name: "magic-wave-220", RequiredSP: 150, MPCost: 50},
		}
	case 33:
		return []CallbackEntry{
			{ID: "job-33-entry-0", Name: "party-magic-barrier", RequiredSP: 5, MPCost: 6},
			{ID: "job-33-entry-1", Name: "magic-wave-70", RequiredSP: 15, MPCost: 12},
			{ID: "job-33-entry-2", Name: "party-breath-barrier", RequiredSP: 30, MPCost: 7},
			{ID: "job-33-entry-3", Name: "party-heal-120", RequiredSP: 70, MPCost: 25},
			{ID: "job-33-entry-4", Name: "attack-doubling", RequiredSP: 100, MPCost: 16},
			{ID: "job-33-entry-5", Name: "magic-wave-160", RequiredSP: 130, MPCost: 34},
			{ID: "job-33-entry-6", Name: "完全蘇生", RequiredSP: 160, MPCost: 80},
			{ID: "job-33-entry-7", Name: "additional-submission", RequiredSP: 160, MPCost: 33, Extended: true},
		}
	case 34:
		return []CallbackEntry{
			{ID: "job-34-entry-0", Name: "party-cover", RequiredSP: 10},
			{ID: "job-34-entry-1", Name: "magic-110", RequiredSP: 30, MPCost: 15},
			{ID: "job-34-entry-2", Name: "meditation-300", RequiredSP: 60, MPCost: 25},
			{ID: "job-34-entry-3", Name: "magic-wave-180", RequiredSP: 90, MPCost: 40},
			{ID: "job-34-entry-4", Name: "magic-immunity", RequiredSP: 120, MPCost: 6},
			{ID: "job-34-entry-5", Name: "party-heal-999", RequiredSP: 150, MPCost: 80},
			{ID: "job-34-entry-6", Name: "group-magic-strike", RequiredSP: 180, MPCost: 30},
		}
	}
	return nil
}

func (c *ProtectiveMagicContext) shift() string {
	if len(c.Arguments) == 0 {
		return ""
	}
	id := c.Arguments[0]
	c.Arguments = c.Arguments[1:]
	return id
}

func protectiveMagicTemporary(id string) TemporaryEffect {
	switch id {
	case "job-32-entry-2":
		return TemporaryReduceAttack
	case "job-32-entry-3":
		return TemporaryReflectMagic
	case "job-33-entry-0":
		return TemporaryReduceMagic
	case "job-33-entry-2":
		return TemporaryReduceBreath
	case "job-34-entry-4":
		return TemporaryNullMagic
	}
	return ""
}

func protectiveMagicDamage(id string) (float64, bool) {
	switch id {
	case "job-32-entry-5":
		return 220, true
	case "job-33-entry-1":
		return 70, true
	case "job-33-entry-5":
		return 160, true
	case "job-34-entry-1":
		return 110, false
	case "job-34-entry-3":
		return 180, true
	}
	return 0, false
}

func (s *LifetimeState) protectiveGroupPower(c *ProtectiveMagicContext) float64 {
	power := 100.0
	for _, id := range c.PartyIDs {
		if a := s.actor(id); id != c.ActorID && a.MP >= 15 {
			a.MP -= 15
			power += 85
		}
	}
	return power
}

// ApplyProtectiveMagic composes intrinsic bodies with the existing effect owners.
// Ordinary costs and random/tool MP restoration remain with the invoking owner.
func (s *LifetimeState) ApplyProtectiveMagic(c *ProtectiveMagicContext, id string, rng random.Generator) error {
	if err := s.ValidateProtectiveMagic(c, id, rng); err != nil {
		return err
	}
	previousDamage, previousEffect := c.targetResources, c.EffectContext.targetResources
	resources := func(r TargetResolution) {
		a := s.actor(c.ActorID)
		a.MP = max(0, a.MP-r.MPDebit)
		applyStatusResources(s, r)
	}
	c.targetResources, c.EffectContext.targetResources = resources, resources
	defer func() { c.targetResources, c.EffectContext.targetResources = previousDamage, previousEffect }()
	a := s.actor(c.ActorID)
	if tmp := protectiveMagicTemporary(id); tmp != "" {
		allowed, err := a.CheckCategorySeal(CategoryMagic, rng)
		if err != nil || !allowed {
			return err
		}
		if id == "job-34-entry-4" {
			a.Temporary = tmp
		} else {
			for _, target := range c.PartyIDs {
				if t := s.actor(target); t.HP > 0 {
					t.Temporary = tmp
				}
			}
		}
		return nil
	}
	if power, area := protectiveMagicDamage(id); power != 0 {
		if area {
			_, err := s.ApplyAreaDamage(&c.DamageContext, power, CategoryMagic, true, rng)
			return err
		}
		_, err := s.ApplyDamage(&c.DamageContext, dualAgilityEffect(c.shift(), power, CategoryMagic, true), rng)
		return err
	}
	switch id {
	case "job-32-entry-0":
		power := 30.0
		if rng.Float64() < .25 {
			power = 100
		}
		_, err := s.ApplyDamage(&c.DamageContext, dualAgilityEffect(c.shift(), power, CategoryMagic, true), rng)
		return err
	case "job-32-entry-1", "job-33-entry-3", "job-34-entry-5":
		power := 50.0
		if id == "job-33-entry-3" {
			power = 120
		} else if id == "job-34-entry-5" {
			power = 999
		}
		_, err := s.ApplyPartyHealing(&c.EffectContext, power, CategoryMagic, rng)
		return err
	case "job-33-entry-4":
		_, err := s.ApplyStatUp(&c.EffectContext, controlEffect(c.shift(), 1, CategoryMagic), StatAttack, rng)
		return err
	case "job-33-entry-7":
		child := StatusContext{EffectContext: c.EffectContext, CureCalls: c.CureCalls}
		_, err := s.ApplyCure(&child, controlSymbol(c.ActorID, a.Status, CategoryUntyped), rng)
		c.EffectContext, c.CureCalls = child.EffectContext, child.CureCalls
		if err != nil {
			return err
		}
		a.Extended, a.ExtendedTurns = ExtendedAdditional, 5
	case "job-34-entry-0":
		r, err := s.ResolvePartyTarget(c.TargetContext, controlSymbol(c.shift(), "cover", CategoryPhysical), rng)
		if err != nil {
			return err
		}
		resources(r)
		if r.Resolved && r.TargetID != c.ActorID {
			a.Temporary, s.actor(r.TargetID).Temporary = TemporaryCovering, TemporaryCover
		}
	case "job-34-entry-2":
		_, err := s.ApplyHealing(&c.EffectContext, controlEffect(c.ActorID, 300, CategoryUntyped), rng)
		return err
	case "job-34-entry-6":
		r, err := s.ResolveEnemyTarget(c.TargetContext, controlSymbol(c.shift(), "group", CategoryMagic), rng)
		if err != nil {
			return err
		}
		resources(r)
		if r.Resolved {
			power := s.protectiveGroupPower(c)
			_, err = s.ApplyDamage(&c.DamageContext, dualAgilityEffect(r.TargetID, power, CategoryMagic, true), rng)
		}
		return err
	}
	return nil
}
