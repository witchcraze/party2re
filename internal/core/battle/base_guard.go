package battle

import "github.com/witchcraze/party2re/internal/core/random"

// BaseGuardContext holds intrinsic operation facts; rebuild ports after reload.
type BaseGuardContext struct {
	DamageContext
	Status    *StatusContext
	Arguments []string
}

// BaseGuardEntries retains source order and the separately owned revival3.6.
func BaseGuardEntries(job int) []CallbackEntry {
	switch job {
	case 0:
		return []CallbackEntry{
			{ID: "job-0-entry-0", Name: "normal-attack", RequiredSP: 8},
			{ID: "job-0-entry-1", Name: "normal-attack", RequiredSP: 9},
			{ID: "job-0-entry-2", Name: "normal-attack", RequiredSP: 10},
			{ID: "job-0-entry-3", Name: "self-tension", RequiredSP: 20},
			{ID: "job-0-entry-4", Name: "defense", RequiredSP: 30},
		}
	case 1:
		return []CallbackEntry{
			{ID: "job-1-entry-0", Name: "shield-strike", Extended: true},
			{ID: "job-1-entry-1", Name: "armor-strike", RequiredSP: 5, MPCost: 5},
			{ID: "job-1-entry-2", Name: "cover", RequiredSP: 8},
			{ID: "job-1-entry-3", Name: "gather-strength", RequiredSP: 25, MPCost: 8},
			{ID: "job-1-entry-4", Name: "stone-wave", RequiredSP: 50, MPCost: 5},
			{ID: "job-1-entry-5", Name: "variable-strike", RequiredSP: 80, MPCost: 10},
			{ID: "job-1-entry-6", Name: "greater-variable-strike", RequiredSP: 80, MPCost: 20, Extended: true},
		}
	case 2:
		return []CallbackEntry{
			{ID: "job-2-entry-0", Name: "sweeping-strike", Extended: true},
			{ID: "job-2-entry-1", Name: "agile-strike", RequiredSP: 5, MPCost: 3},
			{ID: "job-2-entry-2", Name: "binding-strike", RequiredSP: 10, MPCost: 4},
			{ID: "job-2-entry-3", Name: "deflection", RequiredSP: 20, MPCost: 5},
			{ID: "job-2-entry-4", Name: "cover", RequiredSP: 30},
			{ID: "job-2-entry-5", Name: "neutral-strike", RequiredSP: 50, MPCost: 6},
			{ID: "job-2-entry-6", Name: "twin-agile-strikes", RequiredSP: 80, MPCost: 12},
			{ID: "job-2-entry-7", Name: "sword-wave", RequiredSP: 100, MPCost: 21},
			{ID: "job-2-entry-8", Name: "ratio-strike", RequiredSP: 100, MPCost: 49, Extended: true},
		}
	case 3:
		return []CallbackEntry{
			{ID: "job-3-entry-0", Name: "shield-strike", Extended: true},
			{ID: "job-3-entry-1", Name: "cover", RequiredSP: 1},
			{ID: "job-3-entry-2", Name: "fortify-defense", RequiredSP: 5, MPCost: 2},
			{ID: "job-3-entry-3", Name: "reckless-strike", RequiredSP: 15, MPCost: 5},
			{ID: "job-3-entry-4", Name: "major-defense", RequiredSP: 25, MPCost: 3},
			{ID: "job-3-entry-5", Name: "party-defense", RequiredSP: 40, MPCost: 7},
			{ID: "job-3-entry-6", Name: "献命回復", RequiredSP: 60, MPCost: 1},
			{ID: "job-3-entry-7", Name: "shield-wave", RequiredSP: 80, MPCost: 18},
		}
	}
	return nil
}

func (c *BaseGuardContext) shift() string {
	if len(c.Arguments) == 0 {
		return ""
	}
	id := c.Arguments[0]
	c.Arguments = c.Arguments[1:]
	return id
}

func baseGuardTemporary(id string) TemporaryEffect {
	switch id {
	case "job-0-entry-4":
		return TemporaryDefense
	case "job-2-entry-3":
		return TemporaryDeflect
	case "job-3-entry-4":
		return TemporaryStrongDefense
	}
	return ""
}

func baseGuardCover(id string) bool {
	return id == "job-1-entry-2" || id == "job-2-entry-4" || id == "job-3-entry-1"
}

// Ratio power inspects the supplied identity before helper fallback/conversion.
func (s *LifetimeState) baseGuardDamage(c *BaseGuardContext, id, target string) (TargetEffect, bool) {
	a := s.actor(c.ActorID)
	e := dualAgilityEffect(target, float64(a.Attack), CategoryPhysical, false)
	area := false
	switch id {
	case "job-1-entry-0", "job-3-entry-0":
		e.Payload.Number = float64(a.Defense) * 1.4
	case "job-1-entry-1":
		e.Payload.Number *= .9
	case "job-1-entry-4":
		e.Payload.Number, e.Arguments, area = 90, []string{"1"}, true
	case "job-2-entry-0":
		e.Payload.Number, area = e.Payload.Number*.75, true
	case "job-2-entry-1":
		e.Payload.Number = float64(a.Agility) * 1.5
	case "job-2-entry-5":
		e.Payload.Number, e.Category, e.Arguments = e.Payload.Number*.4, CategoryUntyped, []string{"1"}
	case "job-2-entry-6":
		e.Payload.Number = float64(a.Agility) * 2.2
	case "job-2-entry-7":
		area = true
	case "job-2-entry-8":
		e.Arguments = []string{"1"}
		if t := s.actor(target); t != nil && t.TeamID != a.TeamID {
			coefficient := 1.0
			for _, band := range []struct{ threshold, power float64 }{
				{float64(a.MaxHP) * 15, 1.5}, {float64(a.HP) * 10, 1.4}, {float64(a.HP) * 8, 1.3}, {float64(a.HP) * 5, 1.2}, {float64(a.HP) * 3, 1.1},
			} {
				if float64(t.HP) > band.threshold {
					coefficient = band.power
					break
				}
			}
			e.Payload.Number *= coefficient
		} else {
			e.Payload.Number, area = e.Payload.Number*.8, true
		}
	case "job-3-entry-3":
		e.Payload.Number *= 2
	case "job-3-entry-7":
		e.Payload.Number, area = float64(a.Defense)*1.5, true
	}
	return e, area
}

// ApplyBaseGuard executes bodies only; the invocation/consumer owns outer costs.
func (s *LifetimeState) ApplyBaseGuard(c *BaseGuardContext, id string, rng random.Generator) error {
	if err := s.ValidateBaseGuard(c, id, rng); err != nil {
		return err
	}
	a := s.actor(c.ActorID)
	if tmp := baseGuardTemporary(id); tmp != "" {
		a.Temporary = tmp
		return nil
	}
	previousDamage, previousEffect := c.targetResources, c.EffectContext.targetResources
	resources := func(r TargetResolution) {
		a := s.actor(c.ActorID)
		a.MP = max(0, a.MP-r.MPDebit)
		applyStatusResources(s, r)
	}
	c.targetResources, c.EffectContext.targetResources = resources, resources
	defer func() { c.targetResources, c.EffectContext.targetResources = previousDamage, previousEffect }()
	if baseGuardCover(id) {
		r, err := s.ResolvePartyTarget(c.TargetContext, controlSymbol(c.shift(), "cover", CategoryPhysical), rng)
		if err != nil {
			return err
		}
		resources(r)
		if r.Resolved && r.TargetID != c.ActorID {
			a.Temporary, s.actor(r.TargetID).Temporary = TemporaryCovering, TemporaryCover
		}
		return nil
	}
	switch id {
	case "job-0-entry-0", "job-0-entry-1", "job-0-entry-2":
		_, err := s.applyNormalStrike(&c.DamageContext, c.shift(), rng)
		return err
	case "job-0-entry-3":
		return s.statTension(&c.EffectContext, a, false, rng)
	case "job-1-entry-3", "job-3-entry-2":
		stat, power := StatAttack, 1.0
		if id == "job-3-entry-2" {
			stat, power = StatDefense, .4
		}
		_, err := s.ApplyStatUp(&c.EffectContext, controlEffect(c.ActorID, power, CategoryPhysical), stat, rng)
		return err
	case "job-2-entry-2":
		child := *c.Status
		child.EffectContext = c.EffectContext
		_, err := s.ApplyStatus(&child, controlSymbol(c.shift(), StatusDofuu, CategoryPhysical), 80, false, false, rng)
		child.targetResources = c.Status.targetResources
		*c.Status = child
		c.EffectContext = child.EffectContext
		return err
	case "job-3-entry-5":
		_, err := s.ApplyPartyStat(&c.EffectContext, .25, CategoryMagic, StatDefense, false, rng)
		return err
	}
	target := ""
	switch id {
	case "job-1-entry-4", "job-2-entry-0", "job-2-entry-7", "job-3-entry-7":
	default:
		target = c.shift()
	}
	count := 1
	if id == "job-2-entry-6" {
		count = 2
	}
	if id == "job-1-entry-1" {
		c.AdditionalEffect = true
	}
	for range count {
		if id == "job-2-entry-6" && s.actor(c.ActorID).HP <= 0 {
			break
		}
		e, area := s.baseGuardDamage(c, id, target)
		if id == "job-1-entry-5" || id == "job-1-entry-6" {
			chance, low := .5, 20.0
			if id == "job-1-entry-6" {
				chance, low = 2.0/3, float64(s.actor(c.ActorID).Attack)*2
			}
			e.Payload.Number = low
			if rng.Float64() < chance {
				e.Payload.Number = float64(s.actor(c.ActorID).Attack) * 3
			}
		}
		if area {
			_, err := s.ApplyAreaDamage(&c.DamageContext, e.Payload.Number, e.Category, len(e.Arguments) > 0, rng)
			return err
		}
		r, err := s.ApplyDamage(&c.DamageContext, e, rng)
		if err != nil {
			return err
		}
		if id == "job-1-entry-1" && r.Returned && r.TargetID != "" && s.actor(r.TargetID).HP > 0 {
			if _, err := s.ApplyStatDown(&c.EffectContext, controlEffect(r.TargetID, .15, CategoryPhysical), StatDefense, rng); err != nil {
				return err
			}
		}
		if id == "job-3-entry-3" {
			s.actor(c.ActorID).Temporary = TemporaryDouble
		}
	}
	return nil
}
